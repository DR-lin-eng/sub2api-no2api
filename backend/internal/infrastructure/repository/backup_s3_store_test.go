package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func TestS3BackupStoreUploadFileRangeSendsExactRangeAndContentLength(t *testing.T) {
	t.Parallel()

	type uploadRequest struct {
		path          string
		contentType   string
		contentLength int64
		body          []byte
		readErr       error
	}
	requests := make(chan uploadRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		requests <- uploadRequest{
			path:          r.URL.Path,
			contentType:   r.Header.Get("Content-Type"),
			contentLength: r.ContentLength,
			body:          body,
			readErr:       err,
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	filePath := filepath.Join(t.TempDir(), "backup.sql.gz")
	require.NoError(t, os.WriteFile(filePath, []byte("0123456789abcdef"), 0o600))

	store, err := NewS3BackupStoreFactory()(context.Background(), &service.BackupS3Config{
		Endpoint:        server.URL,
		Region:          "test-region",
		Bucket:          "backup-bucket",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		ForcePathStyle:  true,
	})
	require.NoError(t, err)
	require.NoError(t, store.UploadFileRange(context.Background(), "daily/part-0002", filePath, 4, 6, "application/octet-stream"))

	request := <-requests
	require.NoError(t, request.readErr)
	require.Equal(t, "/backup-bucket/daily/part-0002", request.path)
	require.Equal(t, "application/octet-stream", request.contentType)
	require.Equal(t, int64(6), request.contentLength)
	require.Equal(t, []byte("456789"), request.body)
}
