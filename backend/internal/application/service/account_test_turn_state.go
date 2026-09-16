package service

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/net/http/httpguts"
)

type accountTestTurnStateInputContextKey struct{}
type accountTestTurnStateCaptureContextKey struct{}

type accountTestTurnStateCapture struct {
	mu    sync.Mutex
	state string
}

func withAccountTestTurnState(ctx context.Context, state string) context.Context {
	state = boundedAccountTestTurnState(state)
	if state == "" {
		return ctx
	}
	return context.WithValue(ctx, accountTestTurnStateInputContextKey{}, state)
}

func accountTestTurnState(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	state, _ := ctx.Value(accountTestTurnStateInputContextKey{}).(string)
	return boundedAccountTestTurnState(state)
}

func withAccountTestTurnStateCapture(ctx context.Context) (context.Context, *accountTestTurnStateCapture) {
	capture := &accountTestTurnStateCapture{}
	return context.WithValue(ctx, accountTestTurnStateCaptureContextKey{}, capture), capture
}

func captureAccountTestTurnState(ctx context.Context, headers http.Header) {
	if ctx == nil || headers == nil {
		return
	}
	state := boundedAccountTestTurnState(headers.Get(openAICodexTurnStateHeader))
	if state == "" {
		return
	}
	capture, _ := ctx.Value(accountTestTurnStateCaptureContextKey{}).(*accountTestTurnStateCapture)
	if capture == nil {
		return
	}
	capture.mu.Lock()
	capture.state = state
	capture.mu.Unlock()
}

func (c *accountTestTurnStateCapture) value() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func boundedAccountTestTurnState(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > codexTurnStateMaxValueBytes || !httpguts.ValidHeaderFieldValue(value) {
		return ""
	}
	return value
}
