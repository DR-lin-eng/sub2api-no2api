package service

import "io"

// Flush buffered downstream output before blocking for another upstream read.
// Scanner may consume many already available lines between Read calls.
type flushBeforeUpstreamRead struct {
	r     io.Reader
	flush func()
}

func (f *flushBeforeUpstreamRead) Read(p []byte) (int, error) {
	f.flush()
	return f.r.Read(p)
}
