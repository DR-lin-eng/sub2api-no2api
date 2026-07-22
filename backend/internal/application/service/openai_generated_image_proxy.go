package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const (
	generatedImageURLTTL              = 30 * time.Minute
	generatedImageDownloadConcurrency = 4
)

var generatedImageDownloadSlots = make(chan struct{}, generatedImageDownloadConcurrency)

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
	if store == nil || localBaseURL == "" || !containsGeneratedImageURLField(body) {
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

func containsGeneratedImageURLField(body []byte) bool {
	return bytes.Contains(body, []byte(`"url"`)) ||
		bytes.Contains(body, []byte(`"URL"`)) ||
		bytes.Contains(body, []byte(`"Url"`))
}

// rewriteOpenAIImagesResponseURLsStrict rewrites every upstream image URL to
// the local /generated/<sha256>.<ext> proxy form. It is used for an explicit
// response_format=url request, where falling back to the upstream URL would
// leak the origin and create a multi-instance inconsistency.
func (s *OpenAIGatewayService) rewriteOpenAIImagesResponseURLsStrict(c *gin.Context, body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode response_format=url image response: %w", err)
	}
	store := s.generatedImageURLStore()
	localBaseURL := generatedImageLocalBaseURL(c)
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	found, changed, err := rewriteGeneratedImageURLFieldsStrict(document, func(rawURL string) (string, error) {
		if store == nil || localBaseURL == "" {
			return "", ErrGeneratedImageUnavailable
		}
		validated, parsed, err := s.validateGeneratedImageURL(rawURL)
		if err != nil {
			return "", fmt.Errorf("validate generated image URL: %w", err)
		}
		hashBytes := sha256.Sum256([]byte(validated))
		hash := hex.EncodeToString(hashBytes[:])
		if err := store.SetGeneratedImageURL(ctx, hash, validated, generatedImageURLTTL); err != nil {
			return "", fmt.Errorf("%w: cache generated image URL: %v", ErrGeneratedImageUnavailable, err)
		}
		return localBaseURL + "/generated/" + hash + generatedImageExtension(parsed.Path), nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		if root, ok := document.(map[string]any); ok {
			if data, ok := root["data"].([]any); ok && len(data) > 0 {
				return nil, fmt.Errorf("upstream did not honor response_format=url")
			}
		}
		return body, nil
	}
	if !changed {
		return body, nil
	}
	rewritten, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode response_format=url image response: %w", err)
	}
	return rewritten, nil
}

func rewriteGeneratedImageURLFieldsStrict(value any, rewrite func(string) (string, error)) (found bool, changed bool, err error) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "url") {
				rawURL, ok := child.(string)
				if !ok || strings.TrimSpace(rawURL) == "" {
					continue
				}
				found = true
				rewritten, rewriteErr := rewrite(rawURL)
				if rewriteErr != nil {
					return true, changed, rewriteErr
				}
				if rewritten != rawURL {
					typed[key] = rewritten
					changed = true
				}
				continue
			}
			childFound, childChanged, childErr := rewriteGeneratedImageURLFieldsStrict(child, rewrite)
			found = found || childFound
			changed = changed || childChanged
			if childErr != nil {
				return found, changed, childErr
			}
		}
	case []any:
		for _, child := range typed {
			childFound, childChanged, childErr := rewriteGeneratedImageURLFieldsStrict(child, rewrite)
			found = found || childFound
			changed = changed || childChanged
			if childErr != nil {
				return found, changed, childErr
			}
		}
	}
	return found, changed, nil
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
		return []byte("data: {}" + lineEnding)
	}
	data, ok := extractOpenAISSEDataLine(trimmed)
	if !ok || strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return line
	}
	if isOpenAIImagesEmptySSEData(data) {
		return []byte("data: {}" + lineEnding)
	}
	rewritten := s.rewriteOpenAIImagesResponseURLs(c, []byte(data))
	if bytes.Equal(rewritten, []byte(data)) {
		return line
	}
	return []byte("data: " + string(rewritten) + lineEnding)
}

func (s *OpenAIGatewayService) rewriteOpenAIImagesSSELineStrict(c *gin.Context, line []byte) ([]byte, error) {
	if len(line) == 0 {
		return line, nil
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
		return []byte("data: {}" + lineEnding), nil
	}
	data, ok := extractOpenAISSEDataLine(trimmed)
	if !ok || strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" || isOpenAIImagesEmptySSEData(data) {
		return line, nil
	}
	rewritten, err := s.rewriteOpenAIImagesResponseURLsStrictPayload(c, []byte(data))
	if err != nil {
		return nil, err
	}
	if bytes.Equal(rewritten, []byte(data)) {
		return line, nil
	}
	return []byte("data: " + string(rewritten) + lineEnding), nil
}

func (s *OpenAIGatewayService) rewriteOpenAIImagesResponseURLsStrictPayload(c *gin.Context, body []byte) ([]byte, error) {
	if !containsGeneratedImageURLField(body) {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode image URL event: %w", err)
	}
	store := s.generatedImageURLStore()
	localBaseURL := generatedImageLocalBaseURL(c)
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	_, changed, err := rewriteGeneratedImageURLFieldsStrict(document, func(rawURL string) (string, error) {
		if store == nil || localBaseURL == "" {
			return "", ErrGeneratedImageUnavailable
		}
		validated, parsed, err := s.validateGeneratedImageURL(rawURL)
		if err != nil {
			return "", err
		}
		hashBytes := sha256.Sum256([]byte(validated))
		hash := hex.EncodeToString(hashBytes[:])
		if err := store.SetGeneratedImageURL(ctx, hash, validated, generatedImageURLTTL); err != nil {
			return "", fmt.Errorf("%w: %v", ErrGeneratedImageUnavailable, err)
		}
		return localBaseURL + "/generated/" + hash + generatedImageExtension(parsed.Path), nil
	})
	if err != nil {
		return nil, err
	}
	if !changed {
		return body, nil
	}
	return json.Marshal(document)
}

// rewriteOpenAIImagesResponseURLsAsBase64 handles upstreams that return URLs
// even though the client requested the default b64_json form. Downloads are
// bounded and concurrent, then URL fields are replaced with b64_json without
// exposing the upstream origin.
func (s *OpenAIGatewayService) rewriteOpenAIImagesResponseURLsAsBase64(c *gin.Context, body []byte) ([]byte, error) {
	if !containsGeneratedImageURLField(body) {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode image URL response: %w", err)
	}

	urls := make([]string, 0, 1)
	seen := make(map[string]struct{})
	collectGeneratedImageURLs(document, &urls, seen)
	if len(urls) == 0 {
		return body, nil
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	encoded := make([]string, len(urls))
	g, downloadCtx := errgroup.WithContext(ctx)
	g.SetLimit(generatedImageDownloadConcurrency)
	for i := range urls {
		i := i
		g.Go(func() error {
			select {
			case generatedImageDownloadSlots <- struct{}{}:
				defer func() { <-generatedImageDownloadSlots }()
			case <-downloadCtx.Done():
				return downloadCtx.Err()
			}
			value, err := s.downloadGeneratedImageBase64(downloadCtx, urls[i])
			if err != nil {
				return fmt.Errorf("download generated image: %w", err)
			}
			encoded[i] = value
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	byURL := make(map[string]string, len(urls))
	for i, rawURL := range urls {
		byURL[rawURL] = encoded[i]
	}
	rewriteGeneratedImageURLsAsBase64(document, byURL)
	rewritten, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode base64 image response: %w", err)
	}
	return rewritten, nil
}

func collectGeneratedImageURLs(value any, urls *[]string, seen map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "url") {
				if rawURL, ok := child.(string); ok {
					rawURL = strings.TrimSpace(rawURL)
					if rawURL != "" {
						if _, exists := seen[rawURL]; !exists {
							seen[rawURL] = struct{}{}
							*urls = append(*urls, rawURL)
						}
					}
				}
				continue
			}
			collectGeneratedImageURLs(child, urls, seen)
		}
	case []any:
		for _, child := range typed {
			collectGeneratedImageURLs(child, urls, seen)
		}
	}
}

func rewriteGeneratedImageURLsAsBase64(value any, encoded map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "url") {
				rawURL, ok := child.(string)
				if !ok {
					continue
				}
				if b64 := encoded[strings.TrimSpace(rawURL)]; b64 != "" {
					delete(typed, key)
					typed["b64_json"] = b64
				}
				continue
			}
			rewriteGeneratedImageURLsAsBase64(child, encoded)
		}
	case []any:
		for _, child := range typed {
			rewriteGeneratedImageURLsAsBase64(child, encoded)
		}
	}
}

func (s *OpenAIGatewayService) downloadGeneratedImageBase64(ctx context.Context, rawURL string) (string, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rawURL)), "data:") {
		if b64 := openAIResponsesBridgeBase64(rawURL); b64 != "" {
			return b64, nil
		}
		return "", fmt.Errorf("invalid base64 image data URL")
	}
	if s == nil || s.httpUpstream == nil {
		return "", ErrGeneratedImageUnavailable
	}
	validated, _, err := s.validateGeneratedImageURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("validate generated image URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validated, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	resp, err := s.httpUpstream.Do(req, "", 0, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected image status %d", resp.StatusCode)
	}
	if resp.ContentLength > openAIImageMaxDownloadBytes {
		return "", fmt.Errorf("generated image exceeds %d bytes", openAIImageMaxDownloadBytes)
	}
	limited := io.LimitReader(resp.Body, openAIImageMaxDownloadBytes+1)
	prefix, err := io.ReadAll(io.LimitReader(limited, 512))
	if err != nil {
		return "", err
	}
	if len(prefix) == 0 {
		return "", fmt.Errorf("generated image response is empty")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	detectedType := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(prefix), ";")[0]))
	if !strings.HasPrefix(contentType, "image/") && !strings.HasPrefix(detectedType, "image/") {
		return "", fmt.Errorf("generated image URL returned non-image content")
	}
	var encoded strings.Builder
	if resp.ContentLength > 0 {
		encoded.Grow(base64.StdEncoding.EncodedLen(int(resp.ContentLength)))
	}
	encoder := base64.NewEncoder(base64.StdEncoding, &encoded)
	if _, err := encoder.Write(prefix); err != nil {
		return "", err
	}
	copied, copyErr := io.Copy(encoder, limited)
	closeErr := encoder.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if int64(len(prefix))+copied > openAIImageMaxDownloadBytes {
		return "", fmt.Errorf("generated image exceeds %d bytes", openAIImageMaxDownloadBytes)
	}
	return encoded.String(), nil
}

func (s *OpenAIGatewayService) rewriteOpenAIImagesSSELineAsBase64(c *gin.Context, line []byte) ([]byte, error) {
	if len(line) == 0 {
		return line, nil
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
		return []byte("data: {}" + lineEnding), nil
	}
	data, ok := extractOpenAISSEDataLine(trimmed)
	if !ok || strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" || isOpenAIImagesEmptySSEData(data) {
		return line, nil
	}
	if !containsGeneratedImageURLField([]byte(data)) {
		return line, nil
	}
	stopKeepalive := StartOpenAIImagesSSEKeepalive(c, s.openAIImageStreamKeepaliveInterval())
	defer stopKeepalive()
	rewritten, err := s.rewriteOpenAIImagesResponseURLsAsBase64(c, []byte(data))
	if err != nil {
		return nil, err
	}
	if bytes.Equal(rewritten, []byte(data)) {
		return line, nil
	}
	return []byte("data: " + string(rewritten) + lineEnding), nil
}

func stripOpenAIImagesEmptySSEKeepalives(body []byte) []byte {
	if len(body) == 0 || !bytes.Contains(body, []byte("data:")) {
		return body
	}
	lines := bytes.SplitAfter(body, []byte("\n"))
	filtered := make([]byte, 0, len(body))
	changed := false
	for _, line := range lines {
		trimmed := strings.TrimRight(string(line), "\r\n")
		data, ok := extractOpenAISSEDataLine(trimmed)
		if ok && isOpenAIImagesEmptySSEData(data) {
			changed = true
			continue
		}
		filtered = append(filtered, line...)
	}
	if !changed {
		return body
	}
	return filtered
}

func isOpenAIImagesEmptySSEData(data string) bool {
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, "{") {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal([]byte(data), &object) == nil && len(object) == 0
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
