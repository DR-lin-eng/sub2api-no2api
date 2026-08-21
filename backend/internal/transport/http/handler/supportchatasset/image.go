package supportchatasset

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"
)

const (
	multipartOverheadAllowance = 128 << 10
	maxImageDimension          = 4096
	maxImagePixels             = 16_000_000
	maxConcurrentImageDecodes  = 2
	legacyAssetDir             = "data/support-chat-assets"
)

var errNormalizedImageTooLarge = errors.New("normalized image is too large")
var imageDecodeSlots = make(chan struct{}, maxConcurrentImageDecodes)
var safeLegacyAssetName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func ParseUpload(c *gin.Context) (chat.AssetUpload, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, chat.MaxAssetBytes+multipartOverheadAllowance)
	defer func() {
		if form := c.Request.MultipartForm; form != nil {
			_ = form.RemoveAll()
		}
	}()
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Image file is too large")
		} else {
			response.BadRequest(c, "Image file is required")
		}
		return chat.AssetUpload{}, false
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, chat.MaxAssetBytes+1))
	if err != nil || len(data) == 0 {
		response.BadRequest(c, "Invalid image file")
		return chat.AssetUpload{}, false
	}
	if len(data) > chat.MaxAssetBytes {
		response.RequestEntityTooLarge(c, "Image file is too large")
		return chat.AssetUpload{}, false
	}

	release, acquired := acquireImageDecodeSlot()
	if !acquired {
		c.Header("Retry-After", "1")
		response.Error(c, http.StatusServiceUnavailable, "Image processor is busy; retry shortly")
		return chat.AssetUpload{}, false
	}
	defer release()

	normalized, mimeType, name, ok := normalizeImage(data)
	if !ok {
		response.BadRequest(c, "Unsupported or oversized image")
		return chat.AssetUpload{}, false
	}

	return chat.AssetUpload{
		Name:       chat.NormalizeAssetName(c.PostForm("name"), name),
		MIMEType:   mimeType,
		Data:       normalized,
		Collection: chat.NormalizeAssetCollection(c.PostForm("collection")),
	}, true
}

func acquireImageDecodeSlot() (func(), bool) {
	select {
	case imageDecodeSlots <- struct{}{}:
		return func() { <-imageDecodeSlots }, true
	default:
		return func() {}, false
	}
}

func ParseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid asset ID")
		return 0, false
	}
	return id, true
}

// IsLegacyAssetName identifies the filename URLs emitted by the pre-database
// support-chat uploader. Numeric references remain owned by the current asset
// repository and its conversation-level authorization checks.
func IsLegacyAssetName(value string) bool {
	if value == "" || !safeLegacyAssetName.MatchString(value) || filepath.Base(value) != value {
		return false
	}
	switch strings.ToLower(filepath.Ext(value)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

// WriteLegacyAsset serves files created by the PR #7 filesystem uploader.
// The caller is responsible for applying user/admin authentication first.
func WriteLegacyAsset(c *gin.Context, name string) {
	if !IsLegacyAssetName(name) {
		response.NotFound(c, "Chat asset not found")
		return
	}

	path := filepath.Join(legacyAssetDir, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > chat.MaxAssetBytes {
		response.NotFound(c, "Chat asset not found")
		return
	}

	file, err := os.Open(path)
	if err != nil {
		response.NotFound(c, "Chat asset not found")
		return
	}
	defer func() { _ = file.Close() }()

	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		response.NotFound(c, "Chat asset not found")
		return
	}
	mimeType := http.DetectContentType(header[:n])
	if !legacyMIMETypeAllowed(mimeType) {
		response.NotFound(c, "Chat asset not found")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		response.NotFound(c, "Chat asset not found")
		return
	}

	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	c.DataFromReader(http.StatusOK, info.Size(), mimeType, file, nil)
}

func legacyMIMETypeAllowed(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func WriteAsset(c *gin.Context, asset *chat.Asset) {
	if asset == nil || len(asset.Data) == 0 || len(asset.Data) > chat.MaxAssetBytes || asset.Size != len(asset.Data) {
		response.NotFound(c, "Chat asset not found")
		return
	}
	name := "image.png"
	if asset.MIMEType == "image/jpeg" {
		name = "image.jpg"
	} else if asset.MIMEType != "image/png" {
		response.NotFound(c, "Chat asset not found")
		return
	}
	// Authenticated media must not survive a logout/user switch in a shared
	// browser cache. The frontend creates a short-lived Blob URL for display.
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	c.Data(http.StatusOK, asset.MIMEType, asset.Data)
}

func normalizeImage(data []byte) ([]byte, string, string, bool) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !allowedDecodeFormat(format) || !validImageDimensions(config.Width, config.Height) {
		return nil, "", "", false
	}
	detected := http.DetectContentType(data)
	if !contentTypeMatchesFormat(detected, format) {
		return nil, "", "", false
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return nil, "", "", false
	}
	bounds := decoded.Bounds()
	if !validImageDimensions(bounds.Dx(), bounds.Dy()) || bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return nil, "", "", false
	}

	output := &boundedImageBuffer{max: chat.MaxAssetBytes}
	if format == "jpeg" {
		err = jpeg.Encode(output, decoded, &jpeg.Options{Quality: 90})
		if err == nil {
			return output.Bytes(), "image/jpeg", "image.jpg", true
		}
	} else {
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		err = encoder.Encode(output, decoded)
		if err == nil {
			return output.Bytes(), "image/png", "image.png", true
		}
	}
	return nil, "", "", false
}

func allowedDecodeFormat(format string) bool {
	return format == "png" || format == "jpeg" || format == "gif" || format == "webp"
}

func contentTypeMatchesFormat(contentType, format string) bool {
	want := "image/" + format
	if format == "jpeg" {
		want = "image/jpeg"
	}
	return contentType == want
}

func validImageDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= maxImageDimension && height <= maxImageDimension &&
		int64(width)*int64(height) <= maxImagePixels
}

type boundedImageBuffer struct {
	bytes.Buffer
	max int
}

func (w *boundedImageBuffer) Write(p []byte) (int, error) {
	if len(p) > w.max-w.Len() {
		return 0, errNormalizedImageTooLarge
	}
	return w.Buffer.Write(p)
}
