package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (s *chatGPTImageSession) uploadInputs(ctx context.Context, c *gin.Context, parsed *OpenAIImagesRequest) ([]chatgptimages.Attachment, error) {
	proxy := ""
	if s.account.ProxyID != nil && s.account.Proxy != nil {
		proxy = s.account.Proxy.URL()
	}
	ctx = withEgressAccountRouteContext(ctx, accountEgressRoute(s.account, proxy), s.account.ID, s.gateway.cfg)
	uploads := append([]OpenAIImagesUpload(nil), parsed.Uploads...)
	for _, raw := range parsed.InputImageURLs {
		data, kind, name, err := s.gateway.resolveOpenAIResponsesImageInput(ctx, c, s.account, openAIResponsesImageInput{ImageURL: raw})
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, OpenAIImagesUpload{Data: data, ContentType: kind, FileName: name})
	}
	if len(uploads) > 16 {
		return nil, fmt.Errorf("too many ChatGPT image inputs")
	}
	var attachments []chatgptimages.Attachment
	for _, upload := range uploads {
		if len(upload.Data) == 0 || len(upload.Data) > openAIImageMaxUploadPartSize {
			return nil, fmt.Errorf("invalid reference image size")
		}
		kind := http.DetectContentType(upload.Data)
		if !strings.HasPrefix(kind, "image/") {
			return nil, fmt.Errorf("reference input is not an image")
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(upload.Data))
		if err != nil {
			width, height, ok := detectOpenAIWebPDimensions(upload.Data)
			if !ok {
				return nil, fmt.Errorf("reference image dimensions unavailable")
			}
			cfg.Width, cfg.Height = width, height
		}
		name := upload.FileName
		if name == "" {
			name = "reference.png"
		}
		entry, err := s.json(ctx, http.MethodPost, "/files", map[string]any{
			"file_name": name, "file_size": len(upload.Data), "mime_type": kind, "client_resolved_mime_type": kind,
			"entry_surface": "image_gen_upload_input", "upload_source": "reference_image", "use_case": "image_tag", "store_in_library": false,
		})
		if err != nil {
			return nil, err
		}
		id := gjson.GetBytes(entry, "file_id").String()
		if !validChatGPTImageFileID(id) {
			return nil, fmt.Errorf("invalid ChatGPT upload file identity")
		}
		address := gjson.GetBytes(entry, "upload_url").String()
		headers := make(http.Header)
		headers.Set("Content-Type", kind)
		// App direct-Azure uploads need these defaults even when /files omits
		// upload_headers. Upstream-provided storage headers override them below.
		if parsed, parseErr := url.Parse(address); parseErr == nil {
			host := strings.ToLower(parsed.Hostname())
			if strings.HasSuffix(host, ".blob.core.windows.net") || strings.HasSuffix(host, ".oaiusercontent.com") {
				headers.Set("x-ms-blob-type", "BlockBlob")
				headers.Set("x-ms-version", "2020-04-08")
				headers.Set("x-ms-blob-content-type", kind)
			}
		}
		gjson.GetBytes(entry, "upload_headers").ForEach(func(key, value gjson.Result) bool {
			lower := strings.ToLower(key.String())
			if lower == "content-type" || lower == "content-md5" || strings.HasPrefix(lower, "x-ms-") {
				headers.Set(key.String(), value.String())
			}
			return true
		})
		if _, err = s.publicBytes(ctx, http.MethodPut, address, upload.Data, headers, 1<<20); err != nil {
			return nil, err
		}
		body, err := s.requestJSONBody(map[string]any{"file_id": id, "file_name": name, "mime_type": kind, "entry_surface": "image_gen_upload_input", "upload_source": "reference_image", "use_case": "image_tag", "index_for_retrieval": false, "metadata": map[string]any{"store_in_library": false}})
		if err != nil {
			return nil, err
		}
		response, err := s.request(ctx, http.MethodPost, "/files/process_upload_stream", body, true)
		if err != nil {
			return nil, err
		}
		ready, readErr := readChatGPTImageBody(response.Body, 4<<20)
		_ = response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if !chatGPTImageUploadReady(ready, id) {
			return nil, fmt.Errorf("ChatGPT reference image is not ready")
		}
		attachments = append(attachments, chatgptimages.Attachment{ID: id, Width: cfg.Width, Height: cfg.Height, Bytes: len(upload.Data)})
	}
	return attachments, nil
}

func validChatGPTImageFileID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for _, r := range id {
		if r != '_' && r != '-' && r != '#' && r != '*' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func chatGPTImageUploadReady(body []byte, id string) bool {
	ready := false
	check := func(raw []byte) {
		if !gjson.ValidBytes(raw) {
			return
		}
		p := gjson.ParseBytes(raw)
		if p.Get("error").Exists() {
			return
		}
		file := p.Get("file_id").String()
		if file != "" && file != id {
			return
		}
		status := p.Get("status").String()
		event := p.Get("event").String()
		if event == "file.processing.file_ready" || event == "file.processing.completed" || status == "ready" || status == "success" || status == "completed" || p.Get("ready").Bool() {
			ready = true
		}
		if strings.HasSuffix(event, ".failed") || strings.HasSuffix(event, ".cancelled") || strings.HasSuffix(event, ".error") && event != "file.indexing.error" {
			ready = false
		}
	}
	check(body)
	for _, line := range bytes.Split(body, []byte("\n")) {
		check(bytes.TrimSpace(line))
	}
	forEachOpenAISSEDataPayload(string(body), func(raw []byte) { check(raw) })
	return ready
}

// Signed upload/download URLs receive only the resolver's storage headers.
// Public-destination validation, DNS pinning and redirect rejection apply even
// when the account uses a private trusted model relay.
func (s *chatGPTImageSession) publicBytes(ctx context.Context, method, address string, body []byte, headers http.Header, limit int64) ([]byte, error) {
	address, err := urlvalidator.ValidatePublicHTTPURL(address, false)
	if err != nil {
		return nil, fmt.Errorf("invalid ChatGPT image storage destination")
	}
	ctx = WithHTTPUpstreamPublicDestination(WithHTTPUpstreamRedirectsDisabled(ctx))
	if method == http.MethodPut {
		ctx = WithHTTPUpstreamPublicUpload(ctx)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers.Clone()
	proxy := ""
	if s.account.ProxyID != nil && s.account.Proxy != nil {
		proxy = s.account.Proxy.URL()
	}
	// Storage receives only its upload/download headers. Preserve account egress
	// without the first-party OAuth proof, workspace, or device-header adapters.
	resp, err := doAccountHTTPUpstream(s.gateway.httpUpstream, req, proxy, s.account)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT image storage request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ChatGPT image storage returned HTTP %d", resp.StatusCode)
	}
	return readChatGPTImageBody(resp.Body, limit)
}

func (s *chatGPTImageSession) materialize(ctx context.Context, d *chatgptimages.Decoder) ([]openAIResponsesImageResult, error) {
	var results []openAIResponsesImageResult
	for _, asset := range d.Images {
		value := strings.TrimSpace(asset.Value)
		var data []byte
		var err error
		if strings.HasPrefix(value, "file-service://") || strings.HasPrefix(value, "sediment://") {
			id := strings.TrimPrefix(strings.TrimPrefix(value, "file-service://"), "sediment://")
			if !validChatGPTImageFileID(id) {
				return results, fmt.Errorf("invalid generated image file identity")
			}
			data, err = s.download(ctx, d.ConversationID, id)
		} else {
			b64 := normalizeOpenAIImageBase64(value)
			if b64 == "" || base64.StdEncoding.DecodedLen(len(b64)) > openAIImageMaxDownloadBytes {
				return results, fmt.Errorf("invalid generated image data")
			}
			data, err = base64.StdEncoding.DecodeString(b64)
		}
		if err != nil {
			return results, err
		}
		kind := http.DetectContentType(data)
		if !strings.HasPrefix(kind, "image/") {
			return results, fmt.Errorf("ChatGPT returned non-image content")
		}
		if s.outputBytes+int64(len(data)) > 96<<20 {
			return results, fmt.Errorf("ChatGPT image output exceeds request budget")
		}
		s.outputBytes += int64(len(data))
		format := strings.TrimPrefix(strings.Split(kind, ";")[0], "image/")
		b64 := base64.StdEncoding.EncodeToString(data)
		size := detectOpenAIImageResultSize(b64)
		if size == "" {
			return results, fmt.Errorf("ChatGPT returned an incomplete or unsupported image")
		}
		results = append(results, openAIResponsesImageResult{Result: b64, OutputFormat: format, RevisedPrompt: asset.Prompt, Size: size})
	}
	return results, nil
}

func (s *chatGPTImageSession) download(ctx context.Context, conversationID, id string) ([]byte, error) {
	started := time.Now()
	delay := 500 * time.Millisecond
	for attempt := 0; attempt < 10; attempt++ {
		query := url.Values{"inline": []string{"true"}}
		if conversationID != "" {
			query.Set("conversation_id", conversationID)
		}
		body, err := s.json(ctx, http.MethodGet, "/files/download/"+url.PathEscape(strings.ReplaceAll(id, "#", "*"))+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if gjson.GetBytes(body, "status").String() != "retry" {
			address := gjson.GetBytes(body, "download_url").String()
			parsed, err := url.Parse(address)
			if err != nil {
				return nil, fmt.Errorf("invalid image download URL")
			}
			if parsed.Scheme == s.base.Scheme && parsed.Host == s.base.Host {
				resp, err := s.request(ctx, http.MethodGet, address, nil, false)
				if err != nil {
					return nil, err
				}
				defer func() { _ = resp.Body.Close() }()
				return readChatGPTImageBody(resp.Body, openAIImageMaxDownloadBytes)
			}
			return s.publicBytes(ctx, http.MethodGet, address, nil, http.Header{}, openAIImageMaxDownloadBytes)
		}
		remaining := 15*time.Second - time.Since(started)
		if remaining <= 0 {
			break
		}
		if err := chatGPTImageWait(ctx, min(delay, remaining)); err != nil {
			return nil, err
		}
		delay = min(delay*16/10, 4*time.Second)
	}
	return nil, fmt.Errorf("ChatGPT image preview is not ready")
}
