//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiFileImageCountsAndRejectsInvalidParts(t *testing.T) {
	for _, tc := range []struct {
		part  string
		count int
	}{
		{`{"fileData":{"mimeType":"image/png","fileUri":"https://fixture.invalid/a"}}`, 1},
		{`{"file_data":{"mime_type":"image/webp","file_uri":"gs://fixture/b"}}`, 1},
		{`{"fileData":{"mimeType":"image/png","fileUri":42}}`, 0},
		{`{"fileData":{"mimeType":"image/png","fileUri":" "}}`, 0},
		{`{"fileData":{"mimeType":"audio/wav","fileUri":"gs://fixture/b"}}`, 0},
		{`{"text":"normal"}`, 0},
		{`{"inlineData":{"mimeType":"image/png","data":"eA=="},"fileData":{"mimeType":"image/png","fileUri":"gs://fixture/b"}}`, 1},
	} {
		require.Equal(t, tc.count, countGeminiImageOutputs([]byte(geminiImageResponse(tc.part))), tc.part)
	}
}

func TestGeminiFileImagesThroughNativeResponseAndRepeatedFrames(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	body := geminiImageResponse(`{"fileData":{"mimeType":"image/png","fileUri":"gs://fixture/a"}},{"file_data":{"mime_type":"image/png","file_uri":"gs://fixture/b"}}`)
	usage, err := (&GeminiMessagesCompatService{}).handleNativeNonStreamingResponse(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, false)
	require.NoError(t, err)
	require.NotNil(t, usage)
	for range 3 {
		observeGeminiImageOutputs(c, []byte(body))
	}
	require.Equal(t, 2, resolveGeminiImageCount(c, "custom-image", "custom-image"))
	beginGeminiImageOutputObservation(c)
	require.Zero(t, observedGeminiImageOutputs(c))
}
