package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// These headers are useful after a slow pre-output stream, but cannot be
// mutated as ordinary response headers once a keepalive has committed 200.
// Declare them up front and populate them as HTTP trailers when necessary.
var openAIStreamResponseMetadataTrailerNames = []string{
	"X-Codex-Turn-State",
	"X-Request-Id",
	"X-Codex-Primary-Used-Percent",
	"X-Codex-Primary-Reset-After-Seconds",
	"X-Codex-Primary-Window-Minutes",
	"X-Codex-Secondary-Used-Percent",
	"X-Codex-Secondary-Reset-After-Seconds",
	"X-Codex-Secondary-Window-Minutes",
	"X-Codex-Primary-Over-Secondary-Limit-Percent",
	"X-Ratelimit-Limit-Requests",
	"X-Ratelimit-Limit-Tokens",
	"X-Ratelimit-Remaining-Requests",
	"X-Ratelimit-Remaining-Tokens",
	"X-Ratelimit-Reset-Requests",
	"X-Ratelimit-Reset-Tokens",
}

func openAIStreamResponseMetadataTrailersActive(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if _, ok := c.Get(openAICompactSSEKeepaliveKey); ok {
		return true
	}
	if value, ok := c.Get(openAIStreamKeepaliveBytesKey); ok {
		bytes, _ := value.(int)
		return bytes > 0
	}
	return false
}

func declareOpenAIStreamResponseMetadataTrailers(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	var declared func(http.Header)
	declared = func(header http.Header) {
		if header == nil {
			return
		}
		existing := make(map[string]struct{})
		for _, raw := range strings.Split(header.Get("Trailer"), ",") {
			if name := strings.ToLower(strings.TrimSpace(raw)); name != "" {
				existing[name] = struct{}{}
			}
		}
		ordered := make([]string, 0, len(existing)+len(openAIStreamResponseMetadataTrailerNames))
		for _, raw := range strings.Split(header.Get("Trailer"), ",") {
			if name := strings.TrimSpace(raw); name != "" {
				ordered = append(ordered, name)
			}
		}
		for _, name := range openAIStreamResponseMetadataTrailerNames {
			lower := strings.ToLower(name)
			if _, ok := existing[lower]; ok {
				continue
			}
			existing[lower] = struct{}{}
			ordered = append(ordered, name)
		}
		header.Set("Trailer", strings.Join(ordered, ", "))
	}
	// Reading Header through the compact keepalive wrapper intentionally stops
	// the heartbeat. Declaration is metadata-only, so bypass that wrapper while
	// holding its mutex and let the actual response write stop it later.
	if writer, ok := c.Writer.(*openAICompactKeepaliveWriter); ok && writer.k != nil {
		writer.k.mu.Lock()
		if writer.ResponseWriter != nil {
			declared(writer.ResponseWriter.Header())
		}
		writer.k.mu.Unlock()
		return
	}
	declared(c.Writer.Header())
}

func setOpenAIStreamResponseMetadataTrailers(c *gin.Context, staged http.Header) {
	if c == nil || c.Writer == nil || staged == nil {
		return
	}
	for _, name := range openAIStreamResponseMetadataTrailerNames {
		values := headerValuesCaseInsensitive(staged, name)
		if len(values) == 0 {
			continue
		}
		key := http.CanonicalHeaderKey(name)
		c.Writer.Header().Del(key)
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
}

func headerValuesCaseInsensitive(headers http.Header, want string) []string {
	if headers == nil {
		return nil
	}
	for key, values := range headers {
		if strings.EqualFold(key, want) {
			return values
		}
	}
	return nil
}
