package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestCompressOpenAIOAuthCodexRequestBodyScope(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-codex","input":"compress me"}`)

	for _, account := range []*Account{
		nil,
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{Platform: PlatformOpenAI, Type: AccountTypeUpstream},
	} {
		encoded, compressed, err := compressOpenAIOAuthCodexRequestBody(account, body)
		require.NoError(t, err)
		require.False(t, compressed)
		require.Equal(t, body, encoded)
	}

	encoded, compressed, err := compressOpenAIOAuthCodexRequestBody(
		&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body,
	)
	require.NoError(t, err)
	require.True(t, compressed)
	require.NotEqual(t, body, encoded)
	require.Equal(t, body, decodeZstdBody(t, encoded))

	empty, compressed, err := compressOpenAIOAuthCodexRequestBody(
		&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, nil,
	)
	require.NoError(t, err)
	require.False(t, compressed)
	require.Empty(t, empty)
}

func TestOpenAIHTTPBuildersCompressOnlyOAuthBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-codex","input":"` + strings.Repeat("compressible prompt ", 64) + `"}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	oauthAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "test-account",
		},
	}
	apiKeyAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "test-api-key",
			"base_url": "https://example.com/v1",
		},
	}

	tests := []struct {
		name       string
		compressed bool
		build      func(*gin.Context) (*http.Request, error)
	}{
		{
			name:       "ordinary OAuth",
			compressed: true,
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(context.Background(), c, oauthAccount, body, "oauth-token", false, "", true)
			},
		},
		{
			name:       "OAuth passthrough",
			compressed: true,
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, oauthAccount, body, "oauth-token")
			},
		},
		{
			name: "ordinary custom API key",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(context.Background(), c, apiKeyAccount, body, "api-key-token", false, "", true)
			},
		},
		{
			name: "custom API key passthrough",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, apiKeyAccount, body, "api-key-token")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

			request, err := test.build(c)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, request.Body.Close()) })
			wireBody, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Equal(t, int64(len(wireBody)), request.ContentLength)

			if !test.compressed {
				require.Empty(t, request.Header.Get("Content-Encoding"))
				require.Equal(t, body, wireBody)
				return
			}
			require.Equal(t, "zstd", request.Header.Get("Content-Encoding"))
			require.Equal(t, body, decodeZstdBody(t, wireBody))
		})
	}
}

func decodeZstdBody(t testing.TB, body []byte) []byte {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	t.Cleanup(decoder.Close)
	decoded, err := decoder.DecodeAll(body, nil)
	require.NoError(t, err)
	return decoded
}

var benchmarkOpenAIOAuthCompressedBody []byte

func BenchmarkOpenAIOAuthCodexBodyCompression(b *testing.B) {
	body := bytes.Repeat([]byte(`{"role":"user","content":"realistic repeated Codex request content"},`), 256)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	_, _, err := compressOpenAIOAuthCodexRequestBody(account, body)
	require.NoError(b, err)

	b.Run("shared_encoder", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for range b.N {
			encoded, _, encodeErr := compressOpenAIOAuthCodexRequestBody(account, body)
			if encodeErr != nil {
				b.Fatal(encodeErr)
			}
			benchmarkOpenAIOAuthCompressedBody = encoded
		}
	})

	b.Run("upstream_new_encoder_per_request", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for range b.N {
			encoder, newErr := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(openAIOAuthCodexZstdLevel)))
			if newErr != nil {
				b.Fatal(newErr)
			}
			benchmarkOpenAIOAuthCompressedBody = encoder.EncodeAll(body, nil)
			if closeErr := encoder.Close(); closeErr != nil {
				b.Fatal(closeErr)
			}
		}
	})
}
