package middleware

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestIsMultimodalVideoTaskRead(t *testing.T) {
	for _, path := range []string{"/v1/videos/task_123", "/v1/videos/task_123/content", "/videos/task_123", "/videos/task_123/content"} {
		require.True(t, isMultimodalVideoTaskRead(http.MethodGet, path), path)
		require.False(t, isMultimodalVideoTaskRead(http.MethodPost, path), path)
	}
	for _, path := range []string{
		"/v1/videos", "/v1/videos/", "/v1/videos/task_123/", "/v1/videos/task_123/content/extra",
		"/v1/videos/task_123/metadata", "/v1/videos/generations", "/v1/videos/edits", "/v1/videos/extensions",
		"/v1/images/tasks/task_123",
	} {
		require.False(t, isMultimodalVideoTaskRead(http.MethodGet, path), path)
	}
}
