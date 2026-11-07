package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/httputil"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

func requestBodyReadFailure(log *zap.Logger, req *http.Request, err error) (int, string) {
	failure := httputil.DescribeRequestBodyReadFailure(req, err)
	if failure.Kind == httputil.RequestBodyReadErrorMaxBytes {
		return failure.Status, buildBodyTooLargeMessage(failure.MaxBytesLimit)
	}
	if log != nil {
		log.Warn("read request body failed", failure.LogFields()...)
	}
	return failure.Status, failure.Message
}

func handleRequestBodyReadError(c *gin.Context, err error) bool {
	failure := httputil.DescribeRequestBodyReadFailure(c.Request, err)
	if failure.Kind != httputil.RequestBodyReadErrorClientDisconnect {
		return false
	}
	requestLogger(c, "handler.request_body").Warn("read request body failed", failure.LogFields()...)
	c.AbortWithStatus(failure.Status)
	return true
}
