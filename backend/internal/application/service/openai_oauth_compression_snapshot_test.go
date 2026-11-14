package service

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestOpenAIIdentityEncodingRequestSnapshot(t *testing.T) {
	plain := []byte(`{"model":"gpt-6.1-sol","input":"Unicode 漢字 🌈","metadata":{"counter":9007199254740993}}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	compress := func(body []byte) []byte {
		encoded, compressed, err := compressOpenAIOAuthCodexRequestBody(account, body)
		require.NoError(t, err)
		require.True(t, compressed)
		return encoded
	}
	valid := compress(plain)
	var stream bytes.Buffer
	encoder, err := zstd.NewWriter(&stream, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
	require.NoError(t, err)
	_, err = encoder.Write(plain)
	require.NoError(t, err)
	require.NoError(t, encoder.Close())

	for _, scenario := range []string{
		"complete", "unknown_frame_size", "chunked", "truncated", "trailing_garbage",
		"double_encoded", "plain_with_zstd_header", "concatenated_json", "invalid_utf8",
		"wrong_length", "wrong_length_header", "invalid_length_header",
	} {
		t.Run(scenario, func(t *testing.T) {
			wire := valid
			switch scenario {
			case "unknown_frame_size":
				wire = stream.Bytes()
			case "truncated":
				wire = valid[:len(valid)-2]
			case "trailing_garbage":
				wire = append(bytes.Clone(valid), []byte("garbage")...)
			case "double_encoded":
				wire = compress(valid)
			case "plain_with_zstd_header":
				wire = plain
			case "concatenated_json":
				wire = append(bytes.Clone(valid), valid...)
			case "invalid_utf8":
				wire = compress(append([]byte(`{"input":"`), 0xff, '"', '}'))
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.invalid/responses", bytes.NewReader(wire))
			require.NoError(t, err)
			req.Header.Set("Content-Encoding", "zstd")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer synthetic-token")
			switch scenario {
			case "chunked":
				req.ContentLength = -1
				req.TransferEncoding = []string{"chunked"}
			case "wrong_length":
				req.ContentLength++
			case "wrong_length_header":
				req.Header.Set("Content-Length", "1")
			case "invalid_length_header":
				req.Header.Set("Content-Length", "invalid")
			}
			retry, decoded, err := newOpenAIIdentityEncodingRequest(req)
			if scenario != "complete" && scenario != "unknown_frame_size" && scenario != "chunked" {
				require.Error(t, err)
				require.Nil(t, retry)
				return
			}
			require.NoError(t, err)
			require.Equal(t, plain, decoded)
			require.Equal(t, int64(len(plain)), retry.ContentLength)
			require.Empty(t, retry.Header.Get("Content-Encoding"))
			require.Empty(t, retry.Header.Get("Content-Length"))
			require.Empty(t, retry.TransferEncoding)
			require.Equal(t, req.Header.Get("Authorization"), retry.Header.Get("Authorization"))
			for range 2 {
				body, err := retry.GetBody()
				require.NoError(t, err)
				data, err := io.ReadAll(body)
				require.NoError(t, err)
				require.NoError(t, body.Close())
				require.Equal(t, plain, data)
			}
			require.Equal(t, "zstd", req.Header.Get("Content-Encoding"))
			require.NoError(t, retry.Body.Close())
		})
	}
}

func TestOpenAIHTTPCompressionFramesAreIndependent(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for i := range 16 {
		t.Run(strings.Repeat("x", i+1), func(t *testing.T) {
			t.Parallel()
			plain := []byte(`{"input":"` + strings.Repeat("unshared 漢字 ", i+1) + `"}`)
			encoded, compressed, err := compressOpenAIOAuthCodexRequestBody(account, plain)
			require.NoError(t, err)
			require.True(t, compressed)
			var header zstd.Header
			require.NoError(t, header.Decode(encoded))
			require.Zero(t, header.DictionaryID)
			require.False(t, header.Skippable)
			require.Equal(t, plain, decodeZstdBody(t, encoded))
		})
	}
}
