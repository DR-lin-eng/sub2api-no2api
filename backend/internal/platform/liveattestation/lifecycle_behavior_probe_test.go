package liveattestation

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleBehaviorProvider struct {
	calls atomic.Int32
	delay time.Duration
}

func (p *lifecycleBehaviorProvider) Check(context.Context) error { return nil }
func (p *lifecycleBehaviorProvider) Generate(context.Context) (string, error) {
	p.calls.Add(1)
	time.Sleep(p.delay) // Intentionally ignores cancellation to exercise the caller's bound.
	return `{"v":1,"s":0,"t":"opaque.payload+/="}`, nil
}

// Identical source and inputs are used on baseline, modified and restored trees.
func TestCodexLifecycleBehaviorProbe(t *testing.T) {
	p := &lifecycleBehaviorProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := CodexHeader(ctx, p)
	noCalls := p.calls.Load() == 0
	opaque := CodexHeader(context.Background(), p) == `{"v":1,"s":0,"t":"opaque.payload+/="}`
	absent := CodexHeader(context.Background(), nil) == ""
	slow := &lifecycleBehaviorProvider{delay: 300 * time.Millisecond}
	started := time.Now()
	timeout := CodexHeader(context.Background(), slow) == `{"v":1,"s":1}`
	bounded := time.Since(started) < 250*time.Millisecond
	fmt.Printf("LIFECYCLE_BEHAVIOR precanceled_status_3=%t precanceled_provider_skipped=%t opaque_token_preserved=%t absent_provider_header_omitted=%t timeout_status_1=%t wait_bounded=%t\n", canceled == `{"v":1,"s":3}`, noCalls, opaque, absent, timeout, bounded)
	if canceled != `{"v":1,"s":3}` || !noCalls || !opaque || !absent || !timeout || !bounded {
		t.Fatal("attestation callback lifecycle differs from source contract")
	}
}
