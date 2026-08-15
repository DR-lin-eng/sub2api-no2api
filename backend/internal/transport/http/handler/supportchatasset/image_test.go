package supportchatasset

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNormalizeImageReencodesAndStripsTrailingPayload(t *testing.T) {
	input := image.NewRGBA(image.Rect(0, 0, 2, 2))
	input.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, input))
	_, err := encoded.WriteString("<script>alert(1)</script>")
	require.NoError(t, err)

	got, mimeType, name, ok := normalizeImage(encoded.Bytes())
	require.True(t, ok)
	require.Equal(t, "image/png", mimeType)
	require.Equal(t, "image.png", name)
	require.NotContains(t, string(got), "<script>")
	_, format, err := image.Decode(bytes.NewReader(got))
	require.NoError(t, err)
	require.Equal(t, "png", format)
}

func TestNormalizeImageRejectsActiveAndOversizedFormats(t *testing.T) {
	_, _, _, ok := normalizeImage([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	require.False(t, ok)

	oversized := image.NewRGBA(image.Rect(0, 0, maxImageDimension+1, 1))
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, oversized))
	_, _, _, ok = normalizeImage(encoded.Bytes())
	require.False(t, ok)
}

func TestWriteAssetUsesFixedSafeResponseHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/asset", nil)

	WriteAsset(ctx, &chat.Asset{
		Name:     "payload.html",
		MIMEType: "image/png",
		Size:     len("normalized"),
		Data:     []byte("normalized"),
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "same-origin", recorder.Header().Get("Cross-Origin-Resource-Policy"))
	require.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
	require.Contains(t, recorder.Header().Get("Content-Disposition"), "image.png")
	require.NotContains(t, recorder.Header().Get("Content-Disposition"), "payload.html")
}

func TestWriteAssetRejectsCorruptStoredLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/asset", nil)

	WriteAsset(ctx, &chat.Asset{
		Name:     "image.png",
		MIMEType: "image/png",
		Size:     999,
		Data:     []byte("normalized"),
	})

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestImageDecodeConcurrencyIsBounded(t *testing.T) {
	releases := make([]func(), 0, maxConcurrentImageDecodes)
	for range maxConcurrentImageDecodes {
		release, acquired := acquireImageDecodeSlot()
		require.True(t, acquired)
		releases = append(releases, release)
	}
	_, acquired := acquireImageDecodeSlot()
	require.False(t, acquired, "uploads must fail fast instead of accumulating image-decode memory")
	for _, release := range releases {
		release()
	}
	_, acquired = acquireImageDecodeSlot()
	require.True(t, acquired)
	<-imageDecodeSlots
}
