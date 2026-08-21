package supportchatasset

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestNormalizeImageAcceptsJPEG(t *testing.T) {
	input := image.NewRGBA(image.Rect(0, 0, 2, 2))
	input.Set(0, 0, color.RGBA{R: 255, G: 128, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, jpeg.Encode(&encoded, input, &jpeg.Options{Quality: 95}))

	got, mimeType, name, ok := normalizeImage(encoded.Bytes())
	require.True(t, ok)
	require.Equal(t, "image/jpeg", mimeType)
	require.Equal(t, "image.jpg", name)
	_, format, err := image.Decode(bytes.NewReader(got))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
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

func TestIsLegacyAssetNameAcceptsOnlyFlatImageNames(t *testing.T) {
	require.True(t, IsLegacyAssetName("muxue_coin.png"))
	require.True(t, IsLegacyAssetName("1700000000-a_B-9.webp"))
	require.False(t, IsLegacyAssetName("123"))
	require.False(t, IsLegacyAssetName("../image.png"))
	require.False(t, IsLegacyAssetName("folder/image.png"))
	require.False(t, IsLegacyAssetName("library.json"))
}

func TestWriteLegacyAssetServesOnlyValidatedImageFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, os.MkdirAll(legacyAssetDir, 0o755))
	name := "legacy-handler-test.png"
	path := filepath.Join(legacyAssetDir, name)
	t.Cleanup(func() { _ = os.Remove(path) })

	input := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, input))
	require.NoError(t, os.WriteFile(path, encoded.Bytes(), 0o600))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/asset", nil)
	WriteLegacyAsset(ctx, name)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, encoded.Bytes(), recorder.Body.Bytes())
}

func TestWriteLegacyAssetRejectsNonImageContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, os.MkdirAll(legacyAssetDir, 0o755))
	name := "legacy-handler-test.jpg"
	path := filepath.Join(legacyAssetDir, name)
	t.Cleanup(func() { _ = os.Remove(path) })
	require.NoError(t, os.WriteFile(path, []byte("not an image"), 0o600))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/asset", nil)
	WriteLegacyAsset(ctx, name)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}
