package liveattestation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type codexCallbackFixture struct {
	codexFixtureProvider
	callback func(context.Context) (string, error)
}

func (p codexCallbackFixture) GenerateCodexToken(ctx context.Context) (string, error) {
	return p.callback(ctx)
}

func TestCodexCallbackPreservesOpaqueToken(t *testing.T) {
	for _, token := range []string{"", "opaque.payload+/=", "quotes\"\r\n雪", "v1.payload"} {
		p := codexCallbackFixture{callback: func(context.Context) (string, error) { return token, nil }}
		header := CodexHeader(context.Background(), p)
		var envelope struct {
			Version int    `json:"v"`
			Status  int    `json:"s"`
			Token   string `json:"t"`
		}
		if err := json.Unmarshal([]byte(header), &envelope); err != nil || envelope.Version != 1 || envelope.Status != 0 || envelope.Token != token || strings.ContainsAny(header, "\r\n") {
			t.Fatal("opaque callback string must survive JSON encoding without introducing header lines")
		}
	}
	p := codexCallbackFixture{callback: func(context.Context) (string, error) {
		return "sensitive callback token", errors.New("sensitive callback message")
	}}
	if got := CodexHeader(context.Background(), p); got != `{"v":1,"s":2}` {
		t.Fatal("failed callback must not expose its token or error message")
	}
}

func TestCodexCallbackCancellationReleasesWorker(t *testing.T) {
	for _, cancelBeforeCall := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		started, stopped := make(chan struct{}), make(chan struct{})
		p := codexCallbackFixture{callback: func(ctx context.Context) (string, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return "", ctx.Err()
		}}
		if cancelBeforeCall {
			cancel()
		} else {
			go func() { <-started; cancel() }()
		}
		header := CodexHeader(ctx, p)
		cancel()
		if header != `{"v":1,"s":3}` {
			t.Fatal("caller cancellation must report s=3")
		}
		if cancelBeforeCall {
			select {
			case <-started:
				t.Fatal("already canceled caller must not invoke provider")
			default:
			}
		} else {
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("provider did not receive cancellation")
			}
		}
	}
}

func TestCodexTimeoutBoundsUncooperativeProviders(t *testing.T) {
	slots := make(chan struct{}, 2)
	release, started := make(chan struct{}), make(chan struct{}, 2)
	var calls atomic.Int32
	var workers sync.WaitGroup
	p := codexCallbackFixture{callback: func(context.Context) (string, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release // Deliberately ignore context cancellation.
		return "late token", nil
	}}
	defer close(release)
	for range cap(slots) {
		workers.Go(func() {
			if got := codexHeader(context.Background(), p, slots); got != `{"v":1,"s":1}` {
				t.Error("uncooperative callback must time out")
			}
		})
	}
	for range cap(slots) {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("worker failed to start")
		}
	}
	workers.Wait()
	if got := codexHeader(context.Background(), p, slots); got != `{"v":1,"s":1}` || calls.Load() != int32(cap(slots)) {
		t.Fatal("saturated providers must not create additional workers")
	}
	// A late token is confined to its abandoned response channel, never reused.
	fresh := codexCallbackFixture{callback: func(context.Context) (string, error) { return "fresh", nil }}
	if got := CodexHeader(context.Background(), fresh); got != `{"v":1,"s":0,"t":"fresh"}` {
		t.Fatal("late response contaminated an independent request")
	}
}

func TestLiveValidationRemainsSeparateFromOpaqueCallback(t *testing.T) {
	for _, value := range []string{`{"v":1,"s":0,"t":""}`, `{"v":1,"s":0,"t":"opaque+/="}`, `{"v":1,"s":1}`} {
		if !ValidCodexHeader(value) || ValidLiveHeader(value) {
			t.Fatal("Live must retain its native token contract")
		}
	}
	if !ValidLiveHeader(`{"v":1,"s":0,"t":"v1.YWJj"}`) {
		t.Fatal("native Live header rejected")
	}
}
