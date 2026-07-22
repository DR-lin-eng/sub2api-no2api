package service

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const openAIImagesJSONKeepaliveKey = "openai_images_json_keepalive"

// openAIImagesJSONKeepalive keeps non-streaming Images API requests alive while
// an OAuth upstream is producing SSE internally. JSON permits leading
// whitespace, so each heartbeat remains compatible with clients expecting one
// final JSON document.
//
// Once the first heartbeat is sent, the HTTP status is committed as 200. Late
// upstream errors are still returned as an OpenAI-compatible JSON error body,
// matching the status tradeoff used by the compact SSE keepalive path.
type openAIImagesJSONKeepalive struct {
	mu          sync.Mutex
	writer      gin.ResponseWriter
	contentType string
	payload     []byte
	started     bool
	stopped     bool
	bytes       int
	stop        chan struct{}
}

// StartOpenAIImagesJSONKeepalive starts whitespace heartbeats for a
// non-streaming Images request. A non-positive interval disables the feature.
func StartOpenAIImagesJSONKeepalive(c *gin.Context, interval time.Duration) func() {
	return startOpenAIImagesKeepalive(c, interval, "application/json; charset=utf-8", []byte("\n"), false)
}

// StartOpenAIImagesSSEKeepalive starts pseudo-stream SSE heartbeats while an
// Images stream request is waiting for the upstream response headers.
func StartOpenAIImagesSSEKeepalive(c *gin.Context, interval time.Duration) func() {
	return startOpenAIImagesKeepalive(c, interval, "text/event-stream", []byte("data: {}\n\n"), true)
}

// StartOpenAIResponsesImageSSEKeepalive keeps a forced Images-API bridge alive
// without injecting non-standard data events into the Responses stream. SSE
// comments are ignored by conforming clients and therefore do not disturb the
// Responses event state machine.
func StartOpenAIResponsesImageSSEKeepalive(c *gin.Context, interval time.Duration) func() {
	return startOpenAIImagesKeepalive(c, interval, "text/event-stream", []byte(": keepalive\n\n"), false)
}

func startOpenAIImagesKeepalive(c *gin.Context, interval time.Duration, contentType string, payload []byte, beatImmediately bool) func() {
	if c == nil || c.Writer == nil || interval <= 0 {
		return func() {}
	}
	carriedHeartbeatBytes := 0
	if previous := openAIImagesJSONKeepaliveFromContext(c); previous != nil {
		previous.mu.Lock()
		carriedHeartbeatBytes = previous.bytes
		previous.mu.Unlock()
	}
	originalWriter := c.Writer
	k := &openAIImagesJSONKeepalive{
		writer:      originalWriter,
		contentType: contentType,
		payload:     append([]byte(nil), payload...),
		bytes:       carriedHeartbeatBytes,
		stop:        make(chan struct{}),
	}
	header := originalWriter.Header()
	header.Set("Content-Type", contentType)
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	c.Set(openAIImagesJSONKeepaliveKey, k)
	wrappedWriter := &openAIImagesJSONKeepaliveWriter{ResponseWriter: originalWriter, k: k}
	c.Writer = wrappedWriter
	stop := func() {
		k.Stop()
		if current, ok := c.Writer.(*openAIImagesJSONKeepaliveWriter); ok && current == wrappedWriter {
			c.Writer = originalWriter
		}
	}
	if beatImmediately && !k.beat() {
		return stop
	}

	var reqDone <-chan struct{}
	if c.Request != nil {
		reqDone = c.Request.Context().Done()
	}
	go func() {
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-k.stop:
				return
			case <-reqDone:
				return
			case <-timer.C:
			}
			if !k.beat() {
				return
			}
			timer.Reset(interval)
		}
	}()

	return stop
}

func (k *openAIImagesJSONKeepalive) beat() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stopped {
		return false
	}
	if !k.started {
		header := k.writer.Header()
		header.Set("Content-Type", k.contentType)
		header.Set("Cache-Control", "no-cache")
		header.Set("X-Accel-Buffering", "no")
		k.writer.WriteHeader(http.StatusOK)
		k.started = true
	}
	n, err := k.writer.Write(k.payload)
	k.bytes += n
	if err != nil {
		k.stopped = true
		return false
	}
	k.writer.Flush()
	return true
}

func (k *openAIImagesJSONKeepalive) Stop() {
	k.mu.Lock()
	k.markStoppedLocked()
	k.mu.Unlock()
}

func (k *openAIImagesJSONKeepalive) markStoppedLocked() {
	if k.stopped {
		return
	}
	k.stopped = true
	close(k.stop)
}

// StopOpenAIImagesJSONKeepaliveCommitted stops heartbeats and reports whether
// they already committed a 200 response.
func StopOpenAIImagesJSONKeepaliveCommitted(c *gin.Context) bool {
	k := openAIImagesJSONKeepaliveFromContext(c)
	if k == nil {
		return false
	}
	k.mu.Lock()
	k.markStoppedLocked()
	committed := k.started
	k.mu.Unlock()
	return committed
}

// OpenAIImagesJSONKeepalivePresent reports whether the response writer belongs
// to an Images JSON request, including fast responses before the first beat.
func OpenAIImagesJSONKeepalivePresent(c *gin.Context) bool {
	return openAIImagesJSONKeepaliveFromContext(c) != nil
}

// OpenAIImagesSSEKeepalivePresent reports whether the Images response is using
// the stream=true pseudo-stream keepalive mode.
func OpenAIImagesSSEKeepalivePresent(c *gin.Context) bool {
	k := openAIImagesJSONKeepaliveFromContext(c)
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(k.contentType)), "text/event-stream")
}

// OpenAIImagesJSONKeepaliveAdjustedWrittenSize excludes heartbeat whitespace
// from response-size checks so account retry and failover remain available.
func OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	if c == nil || c.Writer == nil {
		return -1
	}
	k := openAIImagesJSONKeepaliveFromContext(c)
	if k == nil {
		return c.Writer.Size()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	size := k.writer.Size()
	if size < 0 {
		return size
	}
	if real := size - k.bytes; real > 0 {
		return real
	}
	return -1
}

func openAIImagesJSONKeepaliveFromContext(c *gin.Context) *openAIImagesJSONKeepalive {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIImagesJSONKeepaliveKey)
	if !ok {
		return nil
	}
	k, _ := value.(*openAIImagesJSONKeepalive)
	return k
}

type openAIImagesJSONKeepaliveWriter struct {
	gin.ResponseWriter
	k *openAIImagesJSONKeepalive
}

func (w *openAIImagesJSONKeepaliveWriter) suspend() {
	if w.k != nil {
		w.k.Stop()
	}
}

func (w *openAIImagesJSONKeepaliveWriter) Header() http.Header {
	w.suspend()
	if w.ResponseWriter == nil {
		return http.Header{}
	}
	return w.ResponseWriter.Header()
}

func (w *openAIImagesJSONKeepaliveWriter) Write(data []byte) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *openAIImagesJSONKeepaliveWriter) WriteString(s string) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
	}
	return w.ResponseWriter.WriteString(s)
}

func (w *openAIImagesJSONKeepaliveWriter) WriteHeader(code int) {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *openAIImagesJSONKeepaliveWriter) WriteHeaderNow() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeaderNow()
	}
}

func (w *openAIImagesJSONKeepaliveWriter) Flush() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.Flush()
	}
}

func (w *openAIImagesJSONKeepaliveWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.ResponseWriter == nil {
		return nil, nil, errors.New("response writer released")
	}
	return w.ResponseWriter.Hijack()
}

func (w *openAIImagesJSONKeepaliveWriter) CloseNotify() <-chan bool {
	if w.ResponseWriter == nil {
		ch := make(chan bool)
		close(ch)
		return ch
	}
	return w.ResponseWriter.CloseNotify()
}

func (w *openAIImagesJSONKeepaliveWriter) Pusher() http.Pusher {
	if w.ResponseWriter == nil {
		return nil
	}
	return w.ResponseWriter.Pusher()
}

func (w *openAIImagesJSONKeepaliveWriter) Status() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Status()
}

func (w *openAIImagesJSONKeepaliveWriter) Size() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Size()
}

func (w *openAIImagesJSONKeepaliveWriter) Written() bool {
	if w.k == nil || w.ResponseWriter == nil {
		return false
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Written()
}
