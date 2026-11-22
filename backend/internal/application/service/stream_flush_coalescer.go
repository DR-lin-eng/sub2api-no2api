package service

import (
	"context"

	"github.com/gin-gonic/gin"
)

// streamFlushCoalesceLimit bounds how many consecutive flushes one burst may
// skip, so a queue that never drains cannot hold output back.
const streamFlushCoalesceLimit = 16

// streamFlushCoalescer turns one flush per event into one flush per burst when
// stream performance mode is on. Upstream usually delivers several events per
// read; while more are already queued, the flush is left to the last of them.
// Nothing waits on a timer, so an event that arrives alone is still flushed
// immediately. With the mode off every event is flushed as before.
type streamFlushCoalescer struct {
	enabled bool
	flush   func()
	skipped int
}

// Flush flushes now unless more events are queued and the limit is not reached.
func (f *streamFlushCoalescer) Flush(moreQueued bool) {
	if f.enabled && moreQueued && f.skipped < streamFlushCoalesceLimit {
		f.skipped++
		return
	}
	f.skipped = 0
	f.flush()
}

// Drain flushes output left behind by skipped flushes. Callers run it whenever
// the queue is empty and before returning.
func (f *streamFlushCoalescer) Drain() {
	if f.skipped > 0 {
		f.skipped = 0
		f.flush()
	}
}

// streamPerformanceModeEnabled reads the admin switch for a gin request.
func streamPerformanceModeEnabled(settings *SettingService, c *gin.Context) bool {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return settings.IsStreamModePerformanceEnabled(ctx)
}
