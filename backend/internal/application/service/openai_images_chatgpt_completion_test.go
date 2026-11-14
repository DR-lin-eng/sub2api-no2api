//go:build unit

package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatGPTAppImagesMessageCompletionMarkerAcceptsEOF(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 3, 3)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"assistant\"},\"output\":[{\"type\":\"image_generation_call\",\"status\":\"completed\",\"result\":%q}]}}\n\ndata: {\"type\":\"message_stream_complete\",\"conversation_id\":\"conv-fixture\"}\n\n", encoded)
	})
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Len(t, upstream.imageCalls(), 1)
}
