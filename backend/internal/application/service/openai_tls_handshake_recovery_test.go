package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// Keep the gateway path under test while exercising the real Go TLS timeout
// and httptrace callbacks against a local server.
type tlsHandshakeRecoveryUpstream struct {
	HTTPUpstream
	client *http.Client
	target string
}

func (u *tlsHandshakeRecoveryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	forwarded := req.Clone(req.Context())
	target, err := http.NewRequest(req.Method, u.target, nil)
	if err != nil {
		return nil, err
	}
	forwarded.URL = target.URL
	forwarded.Host = target.URL.Host
	return u.client.Do(forwarded)
}

func (u *tlsHandshakeRecoveryUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func TestOpenAIGatewayService_TLSHandshakeRecoveryRealTransport(t *testing.T) {
	var handshakes, requests atomic.Int32
	releaseFirstHandshake := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		_, _ = io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp-after-tls-retry","model":"gpt-5.2","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		if handshakes.Add(1) == 1 {
			<-releaseFirstHandshake
		}
		return nil, nil
	}}
	server.StartTLS()
	defer server.Close()
	defer close(releaseFirstHandshake)
	client := server.Client()
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	transport.TLSHandshakeTimeout = 200 * time.Millisecond
	svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, &httpUpstreamRecorder{})
	account.Extra["openai_passthrough"] = false
	svc.httpUpstream = &tlsHandshakeRecoveryUpstream{client: client, target: server.URL}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp-after-tls-retry", result.ResponseID)
	require.Equal(t, int32(2), handshakes.Load())
	require.Equal(t, int32(1), requests.Load(), "failed TLS handshake must never execute the model request")
}

func TestOpenAIGatewayService_TLSHandshakeTimeoutRetriesSameRequest(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "non-stream"
		if stream {
			name = "stream-after-heartbeat"
		}
		t.Run(name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{
				errs: []error{errors.New("net/http: TLS handshake timeout")},
				resp: openAIPassthroughTransportRetrySuccessResponse(),
			}
			svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
			account.Extra["openai_passthrough"] = false
			if stream {
				body = bytes.Replace(body, []byte(`"stream":false`), []byte(`"stream":true`), 1)
				c.Header("Content-Type", "text/event-stream")
				n, err := c.Writer.Write([]byte(":\n\n"))
				require.NoError(t, err)
				recordOpenAIStreamKeepaliveBytes(c, n)
				c.Writer.Flush()
			}

			result, err := svc.Forward(context.Background(), c, account, body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "resp_transport_retry", result.ResponseID)
			require.Equal(t, []int64{account.ID, account.ID}, upstream.accountIDs)
			require.Equal(t, upstream.bodies[0], upstream.bodies[1])
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

func TestOpenAIGatewayService_TLSHandshakeRecoveryBoundaries(t *testing.T) {
	for _, scenario := range []string{"exhausted", "request-sent", "distillation", "canceled", "response-header-timeout", "certificate-error", "semantic-output"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("net/http: TLS handshake timeout")}
			svc, c, _, account, body := newOpenAIPassthroughTransportRetryFixture(t, upstream)
			account.Extra["openai_passthrough"] = false
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			attempts := 1
			switch scenario {
			case "exhausted":
				attempts = 2
			case "request-sent":
				upstream.onDo = func() { httptrace.ContextClientTrace(upstream.lastReq.Context()).WroteHeaders() }
			case "distillation":
				c.Set("api_key", &APIKey{Group: &Group{ID: 981001, IsDistillationGroup: true}})
			case "canceled":
				upstream.onDo = cancel
			case "response-header-timeout":
				upstream.err = errors.New("net/http: timeout awaiting response headers")
			case "certificate-error":
				upstream.err = errors.New("tls: failed to verify certificate: x509: certificate has expired")
			case "semantic-output":
				_, err := c.Writer.Write([]byte("data: output\n\n"))
				require.NoError(t, err)
			}

			_, err := svc.Forward(ctx, c, account, body)

			require.Error(t, err)
			require.Len(t, upstream.requests, attempts)
		})
	}
}
