package supportchatasset

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

const (
	maxAssetBytes = 5 << 20
	assetDir      = "data/support-chat-assets"
	assetURLBase  = "/api/v1/chat/assets"
	libraryPath   = "data/support-chat-assets/library.json"
	stickerPath   = "data/support-chat-assets/stickers.json"
)

var safeAssetName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var libraryMu sync.Mutex

type uploadResponse struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	Size     int    `json:"size"`
	MIMEType string `json:"mime_type"`
}

type LibraryItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	URL       string `json:"url"`
	Size      int    `json:"size"`
	MIMEType  string `json:"mime_type"`
	CreatedAt string `json:"created_at"`
}

type StickerItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	URL       string `json:"url"`
	Size      int    `json:"size"`
	MIMEType  string `json:"mime_type"`
	CreatedAt string `json:"created_at"`
}

func Upload(c *gin.Context) {
	asset, ok := saveUploadedAsset(c)
	if !ok {
		return
	}
	response.Success(c, asset)
}

func ListLibrary(c *gin.Context) {
	items, err := readLibrary()
	if err != nil {
		response.InternalError(c, "Failed to load image library")
		return
	}
	response.Success(c, items)
}

func CreateLibraryItem(c *gin.Context) {
	asset, ok := saveUploadedAsset(c)
	if !ok {
		return
	}

	item := LibraryItem{
		ID:        strings.TrimSuffix(filepath.Base(asset.URL), filepath.Ext(asset.URL)),
		Name:      firstNonEmpty(strings.TrimSpace(c.PostForm("name")), asset.Name, "Image"),
		Category:  firstNonEmpty(strings.TrimSpace(c.PostForm("category")), "默认"),
		URL:       asset.URL,
		Size:      asset.Size,
		MIMEType:  asset.MIMEType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := appendLibraryItem(item); err != nil {
		response.InternalError(c, "Failed to save image library item")
		return
	}
	response.Success(c, item)
}

func DeleteLibraryItem(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "Invalid image ID")
		return
	}
	if err := deleteLibraryItem(id); err != nil {
		response.InternalError(c, "Failed to delete image library item")
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func ListStickers(c *gin.Context) {
	items, err := readStickers()
	if err != nil {
		response.InternalError(c, "Failed to load stickers")
		return
	}
	response.Success(c, items)
}

func CreateSticker(c *gin.Context) {
	asset, ok := saveUploadedAsset(c)
	if !ok {
		return
	}

	item := StickerItem{
		ID:        strings.TrimSuffix(filepath.Base(asset.URL), filepath.Ext(asset.URL)),
		Name:      firstNonEmpty(strings.TrimSpace(c.PostForm("name")), asset.Name, "Sticker"),
		Group:     firstNonEmpty(strings.TrimSpace(c.PostForm("group")), "默认"),
		URL:       asset.URL,
		Size:      asset.Size,
		MIMEType:  asset.MIMEType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := appendSticker(item); err != nil {
		response.InternalError(c, "Failed to save sticker")
		return
	}
	response.Success(c, item)
}

func DeleteSticker(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "Invalid sticker ID")
		return
	}
	if err := deleteSticker(id); err != nil {
		response.InternalError(c, "Failed to delete sticker")
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func saveUploadedAsset(c *gin.Context) (uploadResponse, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAssetBytes+1024)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Image file is required")
		return uploadResponse{}, false
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxAssetBytes+1))
	if err != nil {
		response.BadRequest(c, "Invalid image file")
		return uploadResponse{}, false
	}
	if len(data) == 0 {
		response.BadRequest(c, "Image file is empty")
		return uploadResponse{}, false
	}
	if len(data) > maxAssetBytes {
		response.RequestEntityTooLarge(c, "Image file is too large")
		return uploadResponse{}, false
	}

	mimeType, ext, ok := detectImage(data, header.Header.Get("Content-Type"), header.Filename)
	if !ok {
		response.BadRequest(c, "Only PNG, JPEG, GIF, and WebP images are supported")
		return uploadResponse{}, false
	}

	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		response.InternalError(c, "Failed to prepare image storage")
		return uploadResponse{}, false
	}

	name, err := newAssetName(ext)
	if err != nil {
		response.InternalError(c, "Failed to allocate image name")
		return uploadResponse{}, false
	}

	path := filepath.Join(assetDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		response.InternalError(c, "Failed to save image")
		return uploadResponse{}, false
	}

	return uploadResponse{
		URL:      assetURLBase + "/" + name,
		Name:     sanitizeOriginalName(header.Filename),
		Size:     len(data),
		MIMEType: mimeType,
	}, true
}

func Serve(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !safeAssetName.MatchString(name) || filepath.Base(name) != name {
		c.Status(http.StatusNotFound)
		return
	}

	path := filepath.Join(assetDir, name)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}
	_ = file.Close()

	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.File(path)
}

func detectImage(data []byte, declaredType string, filename string) (string, string, bool) {
	detected := http.DetectContentType(data)
	switch detected {
	case "image/png":
		return "image/png", ".png", true
	case "image/jpeg":
		return "image/jpeg", ".jpg", true
	case "image/gif":
		return "image/gif", ".gif", true
	}

	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp", ".webp", true
	}

	declaredType = strings.ToLower(strings.TrimSpace(strings.Split(declaredType, ";")[0]))
	if declaredType == "image/webp" && strings.EqualFold(filepath.Ext(filename), ".webp") {
		return "image/webp", ".webp", true
	}

	return "", "", false
}

func newAssetName(ext string) (string, error) {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), hex.EncodeToString(randomBytes[:]), ext), nil
}

func sanitizeOriginalName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	if len([]rune(name)) > 120 {
		return string([]rune(name)[:120])
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readLibrary() ([]LibraryItem, error) {
	libraryMu.Lock()
	defer libraryMu.Unlock()
	return readLibraryLocked()
}

func readLibraryLocked() ([]LibraryItem, error) {
	data, err := os.ReadFile(libraryPath)
	if errors.Is(err, os.ErrNotExist) {
		return []LibraryItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []LibraryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func writeLibraryLocked(items []LibraryItem) error {
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(libraryPath, data, 0o644)
}

func appendLibraryItem(item LibraryItem) error {
	libraryMu.Lock()
	defer libraryMu.Unlock()
	items, err := readLibraryLocked()
	if err != nil {
		return err
	}
	items = append([]LibraryItem{item}, items...)
	return writeLibraryLocked(items)
}

func deleteLibraryItem(id string) error {
	libraryMu.Lock()
	defer libraryMu.Unlock()
	items, err := readLibraryLocked()
	if err != nil {
		return err
	}
	next := items[:0]
	for _, item := range items {
		if item.ID != id {
			next = append(next, item)
		}
	}
	return writeLibraryLocked(next)
}

func readStickers() ([]StickerItem, error) {
	libraryMu.Lock()
	defer libraryMu.Unlock()
	return readStickersLocked()
}

func readStickersLocked() ([]StickerItem, error) {
	data, err := os.ReadFile(stickerPath)
	if errors.Is(err, os.ErrNotExist) {
		return []StickerItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []StickerItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Group != items[j].Group {
			return items[i].Group < items[j].Group
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func writeStickersLocked(items []StickerItem) error {
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stickerPath, data, 0o644)
}

func appendSticker(item StickerItem) error {
	libraryMu.Lock()
	defer libraryMu.Unlock()
	items, err := readStickersLocked()
	if err != nil {
		return err
	}
	items = append([]StickerItem{item}, items...)
	return writeStickersLocked(items)
}

func deleteSticker(id string) error {
	libraryMu.Lock()
	defer libraryMu.Unlock()
	items, err := readStickersLocked()
	if err != nil {
		return err
	}
	next := items[:0]
	for _, item := range items {
		if item.ID != id {
			next = append(next, item)
		}
	}
	return writeStickersLocked(next)
}
