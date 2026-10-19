package service

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"
)

// An internal upstream cancellation alone does not prove the client disconnected.
func geminiStreamClientGone(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil
}

// Retain the last cumulative usage after client cancellation. Do not drain the
// provider stream or convert a live client's upstream failure into success.
func geminiStreamAbortedWithBillableOutput(c *gin.Context, err error, usage *ClaudeUsage) bool {
	return err != nil && !errors.Is(err, io.EOF) && geminiStreamClientGone(c) &&
		(usage.hasObservedTokens() || observedGeminiImageOutputs(c) > 0)
}

// Remember the first failed downstream write without changing the SSE encoder.
// This is request-local and stops further writes to the broken connection.
type geminiStreamWriteTracker struct {
	io.Writer
	err error
}

func (w *geminiStreamWriteTracker) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

// Preserve Gin/net/http's StringWriter path for large native SSE image lines.
// Falling back to Write here would allocate a full []byte copy of each line.
func (w *geminiStreamWriteTracker) WriteString(s string) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := io.WriteString(w.Writer, s)
	if n < len(s) && err == nil {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
