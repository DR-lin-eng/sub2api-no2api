package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Keep only the latest output for an explicit image identity. A shared PNG/JPEG
// base64 prefix is not an identity: unrelated images routinely share it.
type multimodalImageSSECollector struct {
	created  json.RawMessage
	usage    OpenAIUsage
	usageRaw json.RawMessage
	images   []json.RawMessage
	indices  map[string]int
}

func newMultimodalImageSSECollector() *multimodalImageSSECollector {
	return &multimodalImageSSECollector{indices: make(map[string]int)}
}

func (a *multimodalImageSSECollector) addFrame(frame []byte) error {
	var parseErr error
	forEachOpenAISSEDataPayload(string(frame), func(payload []byte) {
		if parseErr != nil {
			return
		}
		if !gjson.ValidBytes(payload) {
			parseErr = fmt.Errorf("invalid multimodal image SSE payload")
			return
		}
		root := gjson.ParseBytes(payload)
		errorValue := root.Get("error")
		eventType := root.Get("type").String()
		if (errorValue.Exists() && errorValue.Type != gjson.Null) || eventType == "error" || strings.HasSuffix(eventType, ".failed") {
			parseErr = fmt.Errorf("multimodal upstream image generation failed")
			return
		}
		if created := root.Get("created"); created.Exists() {
			a.created = json.RawMessage(created.Raw)
		}
		if usage := root.Get("usage"); usage.IsObject() {
			a.usageRaw = json.RawMessage(usage.Raw)
		}
		mergeOpenAIUsage(&a.usage, payload)
		if data := root.Get("data"); data.IsArray() {
			items := data.Array()
			occurrences := make(map[string]int)
			for index, item := range items {
				a.addImage(item, root, index, len(items), occurrences)
			}
			return
		}
		if item := root.Get("item"); item.IsObject() {
			a.addImage(item, root, 0, 1, nil)
			return
		}
		a.addImage(root, root, 0, 1, nil)
	})
	return parseErr
}

func multimodalImageIdentity(item gjson.Result) string {
	for _, name := range []string{"id", "image_id", "output_index", "image_index", "index"} {
		if value := item.Get(name); value.Exists() && (value.Type == gjson.String || value.Type == gjson.Number) {
			if name == "id" || name == "image_id" {
				return "id:" + value.String()
			}
			return "index:" + value.String()
		}
	}
	return ""
}

func (a *multimodalImageSSECollector) addImage(item, root gjson.Result, index, length int, occurrences map[string]int) {
	if !item.IsObject() {
		return
	}
	content := item.Get("b64_json").String()
	if content == "" {
		content = item.Get("url").String()
	}
	if strings.TrimSpace(content) == "" {
		return
	}
	key := multimodalImageIdentity(item)
	if key == "" {
		rootKey := ""
		for _, name := range []string{"image_id", "output_index", "image_index", "index"} {
			if value := root.Get(name); value.Exists() {
				rootKey = "index:" + value.String()
				if name == "image_id" {
					rootKey = "id:" + value.String()
				}
				break
			}
		}
		if rootKey != "" {
			key = rootKey
			if length > 1 {
				key += ":" + strconv.Itoa(index)
			}
		} else {
			key = "content:" + hashOpenAIImageOutputResult(content)
			ordinal := 0
			if occurrences != nil {
				ordinal = occurrences[key]
				occurrences[key]++
			}
			// Preserve repeated identical outputs in one result array, while
			// deduplicating retransmissions of that same array. Array position
			// alone cannot identify images sent in separate incremental batches.
			key += ":" + strconv.Itoa(ordinal)
		}
	}
	entry := json.RawMessage(item.Raw)
	if existing, ok := a.indices[key]; ok {
		a.images[existing] = entry
		return
	}
	a.indices[key] = len(a.images)
	a.images = append(a.images, entry)
}

func (a *multimodalImageSSECollector) metadata(request GrokMediaRequestInfo) multimodalMediaUsageMetadata {
	meta := multimodalMediaUsageMetadata{Usage: a.usage, ImageCount: len(a.images), ImageInputSize: request.Size}
	if tier, ok := ClassifyImageBillingTier(request.Size); ok {
		meta.ImageSize = tier
		meta.ImageBillingSizeLocked = true
	}
	for _, image := range a.images {
		if size := gjson.GetBytes(image, "size").String(); strings.TrimSpace(size) != "" {
			meta.ImageOutputSizes = append(meta.ImageOutputSizes, size)
		}
	}
	return meta
}

func (a *multimodalImageSSECollector) response() ([]byte, error) {
	if len(a.images) == 0 {
		return nil, fmt.Errorf("multimodal upstream returned no image output")
	}
	return json.Marshal(struct {
		Created json.RawMessage   `json:"created,omitempty"`
		Data    []json.RawMessage `json:"data"`
		Usage   json.RawMessage   `json:"usage,omitempty"`
	}{a.created, a.images, a.usageRaw})
}

// Bound the complete response and each frame with the existing response limit.
// This accepts multiline SSE and images larger than bufio.Scanner's old 16 MiB
// token limit, without retaining a second copy of the entire wire response.
func readMultimodalSSEFrames(reader io.Reader, limit int64, consume func([]byte) error) error {
	if reader == nil {
		return fmt.Errorf("multimodal image response body is nil")
	}
	if limit <= 0 {
		limit = defaultUpstreamResponseReadMaxBytes
	}
	buffered := bufio.NewReader(io.LimitReader(reader, limit+1))
	var frame bytes.Buffer
	var total int64
	for {
		line, readErr := buffered.ReadBytes('\n')
		total += int64(len(line))
		if total > limit {
			return fmt.Errorf("%w: limit=%d", ErrUpstreamResponseBodyTooLarge, limit)
		}
		_, _ = frame.Write(line)
		if len(line) > 0 && len(bytes.TrimSpace(line)) == 0 {
			if err := consume(frame.Bytes()); err != nil {
				return err
			}
			frame.Reset()
		}
		if readErr != nil {
			if frame.Len() > 0 {
				if err := consume(frame.Bytes()); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func (s *OpenAIGatewayService) readMultimodalImageSSE(ctx context.Context, c *gin.Context, resp *http.Response, request GrokMediaRequestInfo, stream bool, startTime time.Time) ([]byte, multimodalMediaUsageMetadata, *int, error) {
	collector := newMultimodalImageSSECollector()
	var approved multimodalMediaUsageMetadata
	var firstTokenMs *int
	clientDisconnected := false
	err := readMultimodalSSEFrames(resp.Body, resolveUpstreamResponseReadLimit(s.cfg), func(frame []byte) error {
		if err := collector.addFrame(frame); err != nil {
			return err
		}
		if !stream {
			return nil
		}
		meta := collector.metadata(request)
		if err := s.validateMultimodalImageSettlement(ctx, request.Model, getAPIKeyFromContext(c), meta); err != nil {
			return err
		}
		approved = meta
		if c == nil || clientDisconnected {
			return nil
		}
		if !IsResponseCommitted(c) {
			writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
			c.Writer.Header().Del("Content-Length")
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("X-Accel-Buffering", "no")
			c.Status(resp.StatusCode)
		}
		if firstTokenMs == nil {
			elapsed := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &elapsed
		}
		MarkResponseCommitted(c)
		if _, err := c.Writer.Write(frame); err != nil {
			// Continue reading accepted upstream work so a disconnected browser
			// cannot discard its generated-image accounting.
			clientDisconnected = true
			return nil
		}
		c.Writer.Flush()
		return nil
	})
	if err == nil && len(collector.images) == 0 {
		err = fmt.Errorf("multimodal upstream returned no image output")
	}
	if err != nil {
		setOpsUpstreamError(c, http.StatusBadGateway, "multimodal image stream failed", "")
		if stream && c != nil && IsResponseCommitted(c) {
			if !clientDisconnected {
				_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", buildOpenAIImagesStreamErrorBody("Upstream image stream failed"))
				c.Writer.Flush()
			}
			return nil, approved, firstTokenMs, err
		}
		return nil, multimodalMediaUsageMetadata{}, firstTokenMs, err
	}
	meta := collector.metadata(request)
	if stream {
		return nil, meta, firstTokenMs, nil
	}
	body, err := collector.response()
	return body, meta, firstTokenMs, err
}
