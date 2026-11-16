package service

import "github.com/gin-gonic/gin"

func observeAnthropicCompatibilityPayload(c *gin.Context, payload []byte) {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observer.ObserveAnthropic(payload)
}
