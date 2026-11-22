package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
)

// A synchronous upstream result is one Images-compatible data event followed
// by DONE. Compact JSON first so embedded formatting cannot split an SSE frame.
func (s *OpenAIGatewayService) writeMediaJSONAsSSE(c *gin.Context, resp *http.Response, body []byte) error {
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		return fmt.Errorf("invalid JSON in media response: %w", err)
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Writer.Header().Del("Content-Length")
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(resp.StatusCode)
	MarkResponseCommitted(c)
	if _, err := io.WriteString(c.Writer, "data: "); err != nil {
		return err
	}
	if _, err := compact.WriteTo(c.Writer); err != nil {
		return err
	}
	if _, err := io.WriteString(c.Writer, "\n\ndata: [DONE]\n\n"); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// Keep the periodic writer active between complete upstream SSE frames. Gin's
// normal Header/Write methods stop the heartbeat when a terminal response takes
// over. Both the timer and this frame use the same lock and underlying writer.
func (s *OpenAIGatewayService) writeMultimodalImageFrame(c *gin.Context, resp *http.Response, frame []byte, control, done bool) error {
	w := c.Writer
	k := openAIImagesJSONKeepaliveFromContext(c)
	if k != nil {
		k.mu.Lock()
		defer k.mu.Unlock()
		w = k.writer
		if done {
			k.markStoppedLocked()
		}
	}
	if !w.Written() {
		writeOpenAIPassthroughResponseHeaders(w.Header(), resp.Header, s.responseHeaderFilter)
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(resp.StatusCode)
	}
	if !control {
		MarkResponseCommitted(c)
	}
	n, err := w.Write(frame)
	if k != nil && control {
		k.bytes += n
	}
	if err != nil {
		if k != nil {
			k.markStoppedLocked()
		}
		return err
	}
	w.Flush()
	return nil
}
