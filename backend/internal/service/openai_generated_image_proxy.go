package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
)

const generatedImageURLTTL = 30 * time.Minute

// ErrGeneratedImageNotFound indicates that a local hash has no live Redis mapping.
var ErrGeneratedImageNotFound = errors.New("generated image mapping not found")

// ErrGeneratedImageUnavailable indicates that the mapping store or HTTP proxy is unavailable.
var ErrGeneratedImageUnavailable = errors.New("generated image proxy is unavailable")

// GeneratedImageURLStore is an optional GatewayCache capability used to share
// short-lived generated-image URL mappings across application instances.
type GeneratedImageURLStore interface {
	SetGeneratedImageURL(ctx context.Context, hash, rawURL string, ttl time.Duration) error
	GetGeneratedImageURL(ctx context.Context, hash string) (string, bool, error)
}

func (s *OpenAIGatewayService) generatedImageURLStore() GeneratedImageURLStore {
	if s == nil || s.cache == nil {
		return nil
	}
	store, _ := s.cache.(GeneratedImageURLStore)
	return store
}

func (s *OpenAIGatewayService) rewriteOpenAIImagesResponseURLs(c *gin.Context, body []byte) []byte {
	store := s.generatedImageURLStore()
	localBaseURL := generatedImageLocalBaseURL(c)
	if store == nil || localBaseURL == "" || len(body) == 0 {
		return body
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return body
	}

	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	changed := rewriteGeneratedImageURLFields(document, func(rawURL string) string {
		return s.mapGeneratedImageURL(ctx, store, localBaseURL, rawURL)
	})
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(document)
	if err != nil {
		return body
	}
	return rewritten
}

func rewriteGeneratedImageURLFields(value any, rewrite func(string) string) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "url") {
				if rawURL, ok := child.(string); ok {
					if rewritten := rewrite(rawURL); rewritten != rawURL {
						typed[key] = rewritten
						changed = true
					}
					continue
				}
			}
			if rewriteGeneratedImageURLFields(child, rewrite) {
				changed = true
			}
		}
	case []any:
		for _, child := range typed {
			if rewriteGeneratedImageURLFields(child, rewrite) {
				changed = true
			}
		}
	}
	return changed
}

func (s *OpenAIGatewayService) mapGeneratedImageURL(
	ctx context.Context,
	store GeneratedImageURLStore,
	localBaseURL string,
	rawURL string,
) string {
	validated, parsed, err := s.validateGeneratedImageURL(rawURL)
	if err != nil {
		return rawURL
	}
	hashBytes := sha256.Sum256([]byte(validated))
	hash := hex.EncodeToString(hashBytes[:])
	if err := store.SetGeneratedImageURL(ctx, hash, validated, generatedImageURLTTL); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Failed to cache generated image URL: %v", err)
		return rawURL
	}
	return localBaseURL + "/generated/" + hash + generatedImageExtension(parsed.Path)
}

func (s *OpenAIGatewayService) validateGeneratedImageURL(rawURL string) (string, *url.URL, error) {
	allowInsecureHTTP := false
	allowPrivate := false
	if s != nil && s.cfg != nil {
		allowInsecureHTTP = s.cfg.Security.URLAllowlist.AllowInsecureHTTP
		allowPrivate = s.cfg.Security.URLAllowlist.AllowPrivateHosts
	}
	validated, err := urlvalidator.ValidateHTTPURL(rawURL, allowInsecureHTTP, urlvalidator.ValidationOptions{
		AllowPrivate: allowPrivate,
	})
	if err != nil {
		return "", nil, err
	}
	parsed, err := url.Parse(validated)
	if err != nil {
		return "", nil, err
	}
	return validated, parsed, nil
}

func generatedImageExtension(rawPath string) string {
	ext := strings.ToLower(path.Ext(rawPath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif":
		return ext
	default:
		return ".png"
	}
}

func generatedImageLocalBaseURL(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	scheme := firstForwardedHeaderValue(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	host := firstForwardedHeaderValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" || strings.ContainsAny(host, "/\\ \t\r\n") {
		return ""
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}

func firstForwardedHeaderValue(value string) string {
	if before, _, ok := strings.Cut(value, ","); ok {
		value = before
	}
	return strings.TrimSpace(value)
}

func (s *OpenAIGatewayService) rewriteOpenAIImagesSSELine(c *gin.Context, line []byte) []byte {
	if len(line) == 0 {
		return line
	}
	lineEnding := ""
	switch {
	case bytes.HasSuffix(line, []byte("\r\n")):
		lineEnding = "\r\n"
	case bytes.HasSuffix(line, []byte("\n")):
		lineEnding = "\n"
	}
	trimmed := strings.TrimRight(string(line), "\r\n")
	if strings.HasPrefix(trimmed, ":") {
		return []byte(lineEnding)
	}
	data, ok := extractOpenAISSEDataLine(trimmed)
	if !ok || strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return line
	}
	rewritten := s.rewriteOpenAIImagesResponseURLs(c, []byte(data))
	if bytes.Equal(rewritten, []byte(data)) {
		return line
	}
	return []byte("data: " + string(rewritten) + lineEnding)
}

func generatedImageHashFromFilename(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || path.Base(filename) != filename {
		return "", ErrGeneratedImageNotFound
	}
	hash, _, _ := strings.Cut(filename, ".")
	hash = strings.ToLower(hash)
	if len(hash) != sha256.Size*2 {
		return "", ErrGeneratedImageNotFound
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", ErrGeneratedImageNotFound
	}
	return hash, nil
}

// OpenGeneratedImage resolves a hash-only local filename and opens its mapped
// upstream URL. Client authorization headers are intentionally not forwarded.
func (s *OpenAIGatewayService) OpenGeneratedImage(
	ctx context.Context,
	filename string,
	clientHeaders http.Header,
) (*http.Response, error) {
	hash, err := generatedImageHashFromFilename(filename)
	if err != nil {
		return nil, err
	}
	store := s.generatedImageURLStore()
	if store == nil {
		return nil, ErrGeneratedImageUnavailable
	}
	rawURL, found, err := store.GetGeneratedImageURL(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("get generated image mapping: %w", err)
	}
	if !found {
		return nil, ErrGeneratedImageNotFound
	}
	validated, _, err := s.validateGeneratedImageURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid generated image mapping: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validated, nil)
	if err != nil {
		return nil, err
	}
	for _, header := range []string{"Accept", "If-Modified-Since", "If-None-Match", "Range", "User-Agent"} {
		if value := clientHeaders.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "image/*,*/*;q=0.8")
	}
	if s == nil || s.httpUpstream == nil {
		return nil, ErrGeneratedImageUnavailable
	}
	return s.httpUpstream.Do(req, "", 0, 0)
}
