package service

import (
	"io"
	"sync"
	"time"
)

// anthropicCompatStreamReader arms the idle bound only while Body.Read can block.
// Buffered SSE lines and slow downstream writes require no timer resets or line channels.
type anthropicCompatStreamReader struct {
	stream *anthropicCompatStream
	body   io.Reader
}

func (r anthropicCompatStreamReader) Read(p []byte) (int, error) {
	watchdog := r.stream.watchdog
	if watchdog == nil {
		return r.body.Read(p)
	}
	watchdog.beginRead()
	n, err := r.body.Read(p)
	watchdog.endRead()
	return n, err
}

// State is shared only with context/timer callbacks. Closing Body interrupts a
// blocked Read without a permanent reader goroutine or an extra SSE queue.
type anthropicCompatReadWatchdog struct {
	mu       sync.Mutex
	body     io.Closer
	timer    *time.Timer
	interval time.Duration
	deadline time.Time
	reading  bool
	stopped  bool
	timedOut bool
}

func (w *anthropicCompatReadWatchdog) beginRead() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.timedOut {
		return
	}
	w.reading = true
	w.armLocked()
}
func (w *anthropicCompatReadWatchdog) armLocked() {
	if w.interval <= 0 {
		return
	}
	w.deadline = time.Now().Add(w.interval)
	if w.timer == nil {
		w.timer = time.AfterFunc(w.interval, w.expire)
	} else {
		w.timer.Reset(w.interval)
	}
}
func (w *anthropicCompatReadWatchdog) endRead() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reading = false
	if w.timer != nil {
		w.timer.Stop()
	}
}
func (w *anthropicCompatReadWatchdog) enableInterval(interval time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.timedOut || w.interval > 0 || interval <= 0 {
		return
	}
	w.interval = interval
	if w.reading {
		w.armLocked()
	}
}
func (w *anthropicCompatReadWatchdog) expire() {
	w.mu.Lock()
	// A stopped callback can already be runnable when a later read rearms the
	// timer. Its old deadline must not close the body during the new read.
	if w.stopped || !w.reading || w.timedOut || time.Now().Before(w.deadline) {
		w.mu.Unlock()
		return
	}
	w.timedOut = true
	w.mu.Unlock()
	_ = w.body.Close()
}
func (w *anthropicCompatReadWatchdog) didTimeout() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timedOut
}
func (w *anthropicCompatReadWatchdog) readInterval() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.interval
}
func (w *anthropicCompatReadWatchdog) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
}
