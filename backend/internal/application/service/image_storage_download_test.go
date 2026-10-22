package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageResultUploaderDownloadContentTypes(t *testing.T) {
	for _, tt := range []struct {
		name, contentType, wantType string
		body                        []byte
	}{
		{name: "declared png", contentType: "image/png", body: pngBytes, wantType: "image/png"},
		{name: "sniff png", contentType: "application/octet-stream", body: pngBytes, wantType: "image/png"},
		{name: "missing header", body: pngBytes, wantType: "image/png"},
		{name: "declared avif", contentType: "Image/AVIF; charset=binary", body: []byte("\x00\x00\x00\x1cftypavif-not-sniffable"), wantType: "image/avif"},
		{name: "json", contentType: "application/json", body: []byte(`{"ok":true}`)},
		{name: "html", contentType: "text/html", body: []byte("<html>internal</html>")},
		{name: "empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{tt.contentType}}, Body: io.NopCloser(bytes.NewReader(tt.body)), Request: req}, nil
			})}
			storage := &fakeImageStorage{}
			uploader := NewImageResultUploader(storage, "images/", 0, imageStorageTestUpstream{client})
			_, err := uploader.Rewrite(context.Background(), "fixture", json.RawMessage(`{"data":[{"url":"https://cdn.example.test/image"}]}`))
			if tt.wantType == "" {
				require.ErrorContains(t, err, "content is not an image")
				require.Empty(t, storage.saved)
			} else {
				require.NoError(t, err)
				require.Len(t, storage.saved, 1)
				require.Equal(t, tt.wantType, storage.saved[0].contentType)
				require.Equal(t, tt.body, storage.saved[0].data)
			}
		})
	}
}
