package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

// GeneratedImage proxies a short-lived image URL previously emitted by the
// Images API. The filename is a SHA-256 key; the upstream URL remains in Redis.
func (h *OpenAIGatewayHandler) GeneratedImage(c *gin.Context) {
	if h == nil || h.gatewayService == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	resp, err := h.gatewayService.OpenGeneratedImage(c.Request.Context(), c.Param("filename"), c.Request.Header)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGeneratedImageNotFound):
			c.Status(http.StatusNotFound)
		case errors.Is(err, service.ErrGeneratedImageUnavailable):
			c.Status(http.StatusServiceUnavailable)
		default:
			c.Status(http.StatusBadGateway)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for _, header := range []string{
		"Accept-Ranges",
		"Content-Disposition",
		"Content-Range",
		"ETag",
		"Expires",
		"Last-Modified",
	} {
		if value := resp.Header.Get(header); value != "" {
			c.Header(header, value)
		}
	}
	c.Header("Cache-Control", "public, max-age=1800")
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	if resp.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
