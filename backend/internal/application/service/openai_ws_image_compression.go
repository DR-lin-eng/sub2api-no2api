package service

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"mime"
	"strings"
	"sync"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/tidwall/gjson"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type openAIWSImageCompressionResult struct {
	OriginalBytes    int
	PreparedBytes    int
	Changed          bool
	CompressedImages int
	SkippedImages    int
	CandidateBytes   int
	TimedOut         bool
	ShadowOnly       bool
	SkippedReason    string
	decodedPixels    int64
}

type openAIWSImageSpan struct {
	start      int
	end        int
	rawURL     string
	nestedJSON string
}

type openAIWSImageProfile struct {
	maxEdge int
	quality int
}

type openAIWSImageEncoded struct {
	url        string
	compressed bool
}

var openAIWSJPEGBufferPool = sync.Pool{New: func() any {
	return new(bytes.Buffer)
}}

func (s *OpenAIGatewayService) prepareOpenAIWSImagePayload(payload []byte) ([]byte, openAIWSImageCompressionResult) {
	return s.prepareOpenAIWSImagePayloadForAccount(nil, payload)
}

func (s *OpenAIGatewayService) prepareOpenAIWSImagePayloadForAccount(account *Account, payload []byte) ([]byte, openAIWSImageCompressionResult) {
	cfg := s.openAIWSImageCompressionConfig()
	if account != nil && account.IsOpenAIApiKey() && !cfg.APIKeyEnabled {
		cfg.Enabled = false
	}
	result := openAIWSImageCompressionResult{OriginalBytes: len(payload), PreparedBytes: len(payload)}
	if !cfg.Enabled || len(payload) < int(cfg.MinRequestBytes) {
		return payload, result
	}
	prepared, result := prepareOpenAIWSImagePayload(payload, cfg, true, 0)
	if cfg.Mode == "shadow" {
		result.ShadowOnly = true
		return payload, result
	}
	return prepared, result
}

func (s *OpenAIGatewayService) openAIWSImageCompressionConfig() config.OpenAIWSImageCompressionConfig {
	if s == nil || s.cfg == nil {
		return config.OpenAIWSImageCompressionConfig{}
	}
	cfg := s.cfg.Gateway.OpenAIWS.ImageCompression
	if cfg.Mode == "" {
		cfg.Mode = "adaptive"
	}
	if cfg.TargetRequestBytes <= 0 {
		cfg.TargetRequestBytes = 10 * 1024 * 1024
	}
	if cfg.MinRequestBytes <= 0 {
		cfg.MinRequestBytes = 1 * 1024 * 1024
	}
	if cfg.MinImageBytes <= 0 {
		cfg.MinImageBytes = 256 * 1024
	}
	if cfg.MaxEdgePixels <= 0 {
		cfg.MaxEdgePixels = 2048
	}
	if cfg.MaxDecodedPixelsPerImage <= 0 {
		cfg.MaxDecodedPixelsPerImage = 16 * 1024 * 1024
	}
	if cfg.MaxTotalDecodedPixels <= 0 {
		cfg.MaxTotalDecodedPixels = 64 * 1024 * 1024
	}
	if cfg.MaxImagesPerRequest <= 0 {
		cfg.MaxImagesPerRequest = 16
	}
	if cfg.MaxTotalEncodeMS <= 0 {
		cfg.MaxTotalEncodeMS = 750
	}
	if len(cfg.QualityLadder) == 0 {
		cfg.QualityLadder = []int{85, 78, 70}
	}
	return cfg
}

func prepareOpenAIWSImagePayload(payload []byte, cfg config.OpenAIWSImageCompressionConfig, rootInput bool, depth int) ([]byte, openAIWSImageCompressionResult) {
	result := openAIWSImageCompressionResult{OriginalBytes: len(payload), PreparedBytes: len(payload)}
	if len(payload) == 0 || depth > 8 {
		return payload, result
	}
	root := gjson.ParseBytes(payload)
	if !root.Exists() {
		return payload, result
	}
	spans := make([]openAIWSImageSpan, 0, 4)
	if rootInput {
		collectOpenAIWSImageSpans(root.Get("input"), payload, &spans, depth)
	} else {
		collectOpenAIWSImageSpans(root, payload, &spans, depth)
	}
	if len(spans) == 0 {
		return payload, result
	}

	start := time.Now()
	fixedReplacements := make(map[int]string, len(spans))
	direct := make([]openAIWSImageSpan, 0, len(spans))
	for _, span := range spans {
		if span.nestedJSON == "" {
			direct = append(direct, span)
			continue
		}
		nested, nestedResult := prepareOpenAIWSImagePayload([]byte(span.nestedJSON), cfg, false, depth+1)
		if !nestedResult.Changed || len(nested) >= len(span.nestedJSON) {
			result.SkippedImages++
			continue
		}
		encoded, err := json.Marshal(string(nested))
		if err != nil {
			result.SkippedImages++
			continue
		}
		if !validOpenAIWSImageSpan(payload, span) {
			result.SkippedImages++
			continue
		}
		fixedReplacements[span.start] = string(encoded)
		result.Changed = true
		result.CompressedImages += nestedResult.CompressedImages
	}

	if len(direct) == 0 {
		if len(fixedReplacements) == 0 {
			return payload, result
		}
		prepared := applyOpenAIWSImageReplacements(payload, spans, fixedReplacements)
		result.PreparedBytes = len(prepared)
		result.CandidateBytes = len(prepared)
		return prepared, result
	}

	profiles := openAIWSImageProfiles(cfg)
	profileStart := openAIWSImageProfileStart(len(payload), cfg.TargetRequestBytes, len(profiles))
	if profileStart >= len(profiles) {
		profileStart = len(profiles) - 1
	}
	if profileStart < 0 {
		profileStart = 0
	}

	var bestBody []byte
	bestBytes := len(payload)
	nestedCompressedImages := result.CompressedImages
	nestedSkippedImages := result.SkippedImages
	bestCompressedImages := 0
	bestSkippedImages := 0
	for profileIndex := profileStart; profileIndex < len(profiles); profileIndex++ {
		if time.Since(start) >= time.Duration(cfg.MaxTotalEncodeMS)*time.Millisecond {
			result.TimedOut = true
			break
		}
		profile := profiles[profileIndex]
		result.decodedPixels = 0
		replacements := make(map[int]string, len(fixedReplacements)+len(direct))
		encodedCache := make(map[string]openAIWSImageEncoded, len(direct))
		failedCache := make(map[string]struct{}, len(direct))
		profileCompressedImages := 0
		profileSkippedImages := 0
		for index, replacement := range fixedReplacements {
			replacements[index] = replacement
		}
		currentBytes := len(payload)
		for index, span := range direct {
			if index >= cfg.MaxImagesPerRequest {
				profileSkippedImages++
				continue
			}
			if time.Since(start) >= time.Duration(cfg.MaxTotalEncodeMS)*time.Millisecond {
				result.TimedOut = true
				break
			}
			if !validOpenAIWSImageSpan(payload, span) {
				profileSkippedImages++
				continue
			}
			encoded, ok := encodedCache[span.rawURL]
			if !ok {
				if _, failed := failedCache[span.rawURL]; failed {
					profileSkippedImages++
					continue
				}
				encoded, ok = encodeOpenAIWSImage(span.rawURL, profile, cfg, &result)
				if ok {
					encodedCache[span.rawURL] = encoded
				} else {
					failedCache[span.rawURL] = struct{}{}
				}
			}
			if !ok || encoded.url == "" || len(encoded.url) >= span.end-span.start {
				profileSkippedImages++
				continue
			}
			replacements[span.start] = encodedJSONString(encoded.url)
			currentBytes += len(replacements[span.start]) - (span.end - span.start)
			if encoded.compressed {
				profileCompressedImages++
			}
		}
		if currentBytes < bestBytes {
			bestBytes = currentBytes
			bestBody = applyOpenAIWSImageReplacements(payload, spans, replacements)
			bestCompressedImages = profileCompressedImages
			bestSkippedImages = profileSkippedImages
		}
		if int64(currentBytes) <= cfg.TargetRequestBytes {
			break
		}
	}

	if len(bestBody) == 0 || len(bestBody) >= len(payload) {
		result.PreparedBytes = len(payload)
		result.CandidateBytes = bestBytes
		result.CompressedImages = nestedCompressedImages + bestCompressedImages
		result.SkippedImages = nestedSkippedImages + bestSkippedImages
		if result.TimedOut {
			result.SkippedReason = "encode_timeout"
		}
		return payload, result
	}
	result.PreparedBytes = len(bestBody)
	result.CandidateBytes = bestBytes
	result.CompressedImages = nestedCompressedImages + bestCompressedImages
	result.SkippedImages = nestedSkippedImages + bestSkippedImages
	result.Changed = true
	return bestBody, result
}

func openAIWSImageProfiles(cfg config.OpenAIWSImageCompressionConfig) []openAIWSImageProfile {
	quality := append([]int(nil), cfg.QualityLadder...)
	if len(quality) == 0 {
		quality = []int{85, 78, 70}
	}
	profiles := make([]openAIWSImageProfile, 0, len(quality)+1)
	for index, value := range quality {
		edge := cfg.MaxEdgePixels
		if index >= 2 {
			edge = max(768, cfg.MaxEdgePixels*3/4)
		}
		profiles = append(profiles, openAIWSImageProfile{maxEdge: edge, quality: value})
	}
	return profiles
}

func openAIWSImageProfileStart(bodyBytes int, target int64, profileCount int) int {
	if profileCount <= 1 || target <= 0 || int64(bodyBytes) <= target*4 {
		return 0
	}
	if profileCount > 2 && int64(bodyBytes) > target*8 {
		return 2
	}
	return 1
}

func collectOpenAIWSImageSpans(value gjson.Result, payload []byte, spans *[]openAIWSImageSpan, depth int) {
	if !value.Exists() || depth > 8 {
		return
	}
	if value.IsArray() {
		value.ForEach(func(_, item gjson.Result) bool {
			collectOpenAIWSImageSpans(item, payload, spans, depth+1)
			return true
		})
		return
	}
	if !value.IsObject() {
		return
	}
	typ := strings.TrimSpace(value.Get("type").String())
	if typ == "input_image" || typ == "image_url" {
		imageURL := value.Get("image_url")
		if imageURL.IsObject() {
			imageURL = imageURL.Get("url")
		}
		if imageURL.Type == gjson.String {
			addOpenAIWSImageSpan(imageURL, payload, spans)
		}
	}
	if content := value.Get("content"); content.Exists() {
		collectOpenAIWSImageSpans(content, payload, spans, depth+1)
	}
	if output := value.Get("output"); output.Exists() {
		if output.Type == gjson.String {
			nested := strings.TrimSpace(output.String())
			if (strings.HasPrefix(nested, "[") || strings.HasPrefix(nested, "{")) && strings.Contains(strings.ToLower(nested), "data:image/") {
				if validOpenAIWSImageSpan(payload, openAIWSImageSpan{start: output.Index, end: output.Index + len(output.Raw)}) {
					*spans = append(*spans, openAIWSImageSpan{start: output.Index, end: output.Index + len(output.Raw), nestedJSON: output.String()})
				}
			}
		} else {
			collectOpenAIWSImageSpans(output, payload, spans, depth+1)
		}
	}
}

func addOpenAIWSImageSpan(value gjson.Result, payload []byte, spans *[]openAIWSImageSpan) {
	span := openAIWSImageSpan{start: value.Index, end: value.Index + len(value.Raw), rawURL: value.String()}
	if validOpenAIWSImageSpan(payload, span) && strings.HasPrefix(strings.ToLower(strings.TrimSpace(span.rawURL)), "data:image/") {
		*spans = append(*spans, span)
	}
}

func validOpenAIWSImageSpan(payload []byte, span openAIWSImageSpan) bool {
	if span.start < 0 || span.end <= span.start || span.end > len(payload) {
		return false
	}
	raw := payload[span.start:span.end]
	return len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"'
}

func encodedJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func applyOpenAIWSImageReplacements(payload []byte, spans []openAIWSImageSpan, replacements map[int]string) []byte {
	selected := make([]openAIWSImageSpan, 0, len(replacements))
	for _, span := range spans {
		if replacements[span.start] != "" {
			selected = append(selected, span)
		}
	}
	for i := 0; i < len(selected); i++ {
		for j := i + 1; j < len(selected); j++ {
			if selected[j].start > selected[i].start {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}
	result := append([]byte(nil), payload...)
	for _, span := range selected {
		replacement := []byte(replacements[span.start])
		result = append(result[:span.start], append(replacement, result[span.end:]...)...)
	}
	return result
}

func encodeOpenAIWSImage(rawURL string, profile openAIWSImageProfile, cfg config.OpenAIWSImageCompressionConfig, result *openAIWSImageCompressionResult) (openAIWSImageEncoded, bool) {
	data, mediaType, ok := decodeOpenAIWSImageDataURL(rawURL)
	if !ok || int64(len(data)) < cfg.MinImageBytes {
		return openAIWSImageEncoded{}, false
	}
	if mediaType == "image/gif" || mediaType == "image/svg+xml" {
		return openAIWSImageEncoded{}, false
	}
	if mediaType == "image/jpeg" {
		orientation, known := openAIWSJPEGOrientation(data)
		if !known || orientation != 1 {
			return openAIWSImageEncoded{}, false
		}
	}
	configBounds, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || configBounds.Width <= 0 || configBounds.Height <= 0 {
		return openAIWSImageEncoded{}, false
	}
	pixels := int64(configBounds.Width) * int64(configBounds.Height)
	if pixels > cfg.MaxDecodedPixelsPerImage {
		return openAIWSImageEncoded{}, false
	}
	if result != nil && result.decodedPixels+pixels > cfg.MaxTotalDecodedPixels {
		return openAIWSImageEncoded{}, false
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format == "gif" {
		return openAIWSImageEncoded{}, false
	}
	if cfg.PreserveAlpha && openAIWSImageHasAlpha(src) {
		return openAIWSImageEncoded{}, false
	}
	if result != nil {
		result.decodedPixels += pixels
	}

	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if profile.maxEdge > 0 && (width > profile.maxEdge || height > profile.maxEdge) {
		scale := float64(profile.maxEdge) / float64(max(width, height))
		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, stddraw.Over, nil)

	buffer := openAIWSJPEGBufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	defer func() {
		buffer.Reset()
		openAIWSJPEGBufferPool.Put(buffer)
	}()
	if err := jpeg.Encode(buffer, dst, &jpeg.Options{Quality: profile.quality}); err != nil {
		return openAIWSImageEncoded{}, false
	}
	encoded := base64.StdEncoding.EncodeToString(buffer.Bytes())
	url := "data:image/jpeg;base64," + encoded
	return openAIWSImageEncoded{url: url, compressed: true}, true
}

func decodeOpenAIWSImageDataURL(rawURL string) ([]byte, string, bool) {
	value := strings.TrimSpace(rawURL)
	if len(value) < len("data:") || !strings.EqualFold(value[:len("data:")], "data:") {
		return nil, "", false
	}
	header, encoded, ok := strings.Cut(value[len("data:"):], ",")
	if !ok || !strings.Contains(strings.ToLower(header), ";base64") {
		return nil, "", false
	}
	mediaType, _, err := mime.ParseMediaType(strings.Split(header, ";")[0])
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(header, ";")[0])
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if !strings.HasPrefix(mediaType, "image/") {
		return nil, "", false
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	return data, mediaType, true
}

func openAIWSImageHasAlpha(src image.Image) bool {
	if src == nil {
		return false
	}
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			_, _, _, alpha := src.At(x, y).RGBA()
			if alpha != 0xffff {
				return true
			}
		}
	}
	return false
}

func openAIWSJPEGOrientation(data []byte) (int, bool) {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 1, true
	}
	for offset := 2; offset+4 <= len(data); {
		if data[offset] != 0xff {
			return 1, false
		}
		marker := data[offset+1]
		offset += 2
		if marker == 0xd9 || marker == 0xda {
			break
		}
		if offset+2 > len(data) {
			return 1, false
		}
		segmentLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if segmentLength < 2 || offset+segmentLength > len(data) {
			return 1, false
		}
		if marker == 0xe1 && segmentLength >= 8 && string(data[offset+2:offset+8]) == "Exif\x00" {
			return parseOpenAIWSExifOrientation(data[offset+8 : offset+segmentLength])
		}
		offset += segmentLength
	}
	return 1, true
}

func parseOpenAIWSExifOrientation(data []byte) (int, bool) {
	if len(data) < 8 {
		return 1, false
	}
	var order binary.ByteOrder = binary.BigEndian
	if string(data[:2]) == "II" {
		order = binary.LittleEndian
	} else if string(data[:2]) != "MM" {
		return 1, false
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1, false
	}
	ifdOffset := int(order.Uint32(data[4:8]))
	if ifdOffset < 0 || ifdOffset+2 > len(data) {
		return 1, false
	}
	count := int(order.Uint16(data[ifdOffset : ifdOffset+2]))
	for i := 0; i < count; i++ {
		entry := ifdOffset + 2 + i*12
		if entry+12 > len(data) {
			return 1, false
		}
		if order.Uint16(data[entry:entry+2]) != 0x0112 {
			continue
		}
		if order.Uint16(data[entry+2:entry+4]) != 3 {
			return 1, false
		}
		value := int(order.Uint16(data[entry+8 : entry+10]))
		if value < 1 || value > 8 {
			return 1, false
		}
		return value, true
	}
	return 1, true
}
