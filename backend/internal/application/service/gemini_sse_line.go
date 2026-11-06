package service

import (
	"bufio"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"io"
	"unsafe"
)

type boundedSSELineReader struct {
	reader      *bufio.Reader
	maxLineSize int
}

func newBoundedSSELineReader(reader io.Reader, cfg *config.Config) *boundedSSELineReader {
	limit := defaultMaxLineSize
	if cfg != nil && cfg.Gateway.MaxLineSize > 0 {
		limit = cfg.Gateway.MaxLineSize
	}
	return &boundedSSELineReader{reader: bufio.NewReader(reader), maxLineSize: limit}
}
func (r *boundedSSELineReader) ReadLineString() (string, error) {
	var line []byte
	for {
		part, err := r.reader.ReadSlice('\n')
		if len(part) > r.maxLineSize-len(line) {
			return "", fmt.Errorf("upstream SSE line exceeds %d bytes: %w", r.maxLineSize, bufio.ErrTooLong)
		}
		line = append(line, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return unsafe.String(unsafe.SliceData(line), len(line)), err
	}
}
