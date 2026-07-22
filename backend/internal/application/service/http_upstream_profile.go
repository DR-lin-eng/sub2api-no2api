package service

import (
	"context"
	"errors"
	"strings"
)

// HTTPUpstreamProfile marks HTTP upstream requests that need provider-specific
// transport policy.
type HTTPUpstreamProfile string

const (
	HTTPUpstreamProfileDefault      HTTPUpstreamProfile = ""
	HTTPUpstreamProfileOpenAI       HTTPUpstreamProfile = "openai"
	HTTPUpstreamProfileOpenAIStream HTTPUpstreamProfile = "openai_stream"
)

type httpUpstreamProfileContextKey struct{}
type httpUpstreamDisableRedirectsContextKey struct{}
type openAIStreamSchedulingContextKey struct{}

// WithHTTPUpstreamProfile injects an upstream transport profile into ctx.
func WithHTTPUpstreamProfile(ctx context.Context, profile HTTPUpstreamProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if profile == HTTPUpstreamProfileDefault {
		return ctx
	}
	return context.WithValue(ctx, httpUpstreamProfileContextKey{}, profile)
}

// HTTPUpstreamProfileFromContext resolves the upstream transport profile from ctx.
func HTTPUpstreamProfileFromContext(ctx context.Context) HTTPUpstreamProfile {
	if ctx == nil {
		return HTTPUpstreamProfileDefault
	}
	profile, ok := ctx.Value(httpUpstreamProfileContextKey{}).(HTTPUpstreamProfile)
	if !ok {
		return HTTPUpstreamProfileDefault
	}
	switch profile {
	case HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileOpenAIStream:
		return profile
	default:
		return HTTPUpstreamProfileDefault
	}
}

// WithHTTPUpstreamRedirectsDisabled prevents credential-bearing probes from
// following redirects through the shared upstream client.
func WithHTTPUpstreamRedirectsDisabled(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, httpUpstreamDisableRedirectsContextKey{}, true)
}

func HTTPUpstreamRedirectsDisabled(ctx context.Context) bool {
	return ctx != nil && ctx.Value(httpUpstreamDisableRedirectsContextKey{}) == true
}

// WithOpenAIStreamScheduling marks scheduler work that will open a streaming
// OpenAI-compatible upstream request. Stream degradation only affects these
// requests; non-stream scheduling keeps its existing behavior.
func WithOpenAIStreamScheduling(ctx context.Context, stream bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !stream {
		return ctx
	}
	return context.WithValue(ctx, openAIStreamSchedulingContextKey{}, true)
}

func isOpenAIStreamScheduling(ctx context.Context) bool {
	return ctx != nil && ctx.Value(openAIStreamSchedulingContextKey{}) == true
}

// OpenAIStreamResponseHeaderTimeoutError distinguishes the stream response-
// header deadline from dial, TLS, caller-context, and non-stream timeouts.
type OpenAIStreamResponseHeaderTimeoutError struct {
	err error
}

func (e *OpenAIStreamResponseHeaderTimeoutError) Error() string {
	if e == nil || e.err == nil {
		return "openai stream response header timeout"
	}
	return e.err.Error()
}

func (e *OpenAIStreamResponseHeaderTimeoutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *OpenAIStreamResponseHeaderTimeoutError) Timeout() bool   { return true }
func (e *OpenAIStreamResponseHeaderTimeoutError) Temporary() bool { return true }

// WrapOpenAIStreamResponseHeaderTimeout wraps only the stable errors emitted by
// net/http and x/net/http2 when ResponseHeaderTimeout expires.
func WrapOpenAIStreamResponseHeaderTimeout(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "timeout awaiting response headers") {
		return err
	}
	return &OpenAIStreamResponseHeaderTimeoutError{err: err}
}

func IsOpenAIStreamResponseHeaderTimeout(err error) bool {
	var target *OpenAIStreamResponseHeaderTimeoutError
	return errors.As(err, &target)
}

func openAIHTTPUpstreamProfile(ctx context.Context, _ *Account, stream bool) HTTPUpstreamProfile {
	if stream && !OpenAIImageGenerationIntentFromContext(ctx) {
		return HTTPUpstreamProfileOpenAIStream
	}
	return HTTPUpstreamProfileOpenAI
}
