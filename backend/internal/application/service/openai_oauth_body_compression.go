package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	openAIOAuthCodexZstdLevel          = 3
	openAIOAuthCodexZstdMaxConcurrency = 4
)

var openAIOAuthCodexBodyEncoder struct {
	sync.Once
	encoder *zstd.Encoder
	err     error
}

func newOpenAIHTTPUpstreamRequest(
	ctx context.Context,
	method string,
	targetURL string,
	account *Account,
	body []byte,
) (*http.Request, error) {
	upstreamBody, compressed, err := compressOpenAIOAuthCodexRequestBody(account, body)
	if err != nil {
		return nil, fmt.Errorf("compress OpenAI OAuth request body: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, err
	}
	if compressed {
		request.Header.Set("Content-Encoding", "zstd")
	}
	return request, nil
}

// compressOpenAIOAuthCodexRequestBody encodes requests sent to the official
// ChatGPT Codex backend. API-key requests may target arbitrary compatible
// upstreams, so their wire body must remain unchanged.
func compressOpenAIOAuthCodexRequestBody(account *Account, body []byte) ([]byte, bool, error) {
	if account == nil || account.Type != AccountTypeOAuth || len(body) == 0 {
		return body, false, nil
	}

	openAIOAuthCodexBodyEncoder.Do(func() {
		concurrency := min(runtime.GOMAXPROCS(0), openAIOAuthCodexZstdMaxConcurrency)
		openAIOAuthCodexBodyEncoder.encoder, openAIOAuthCodexBodyEncoder.err = zstd.NewWriter(
			nil,
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(openAIOAuthCodexZstdLevel)),
			zstd.WithEncoderConcurrency(concurrency),
		)
	})
	if openAIOAuthCodexBodyEncoder.err != nil {
		return nil, false, fmt.Errorf("create shared zstd encoder: %w", openAIOAuthCodexBodyEncoder.err)
	}

	// EncodeAll is documented as concurrency-safe. Reusing the process-lifetime
	// encoder avoids constructing a GOMAXPROCS-sized pool per request; the cap
	// also bounds retained encoder memory on high-core hosts.
	return openAIOAuthCodexBodyEncoder.encoder.EncodeAll(body, nil), true, nil
}
