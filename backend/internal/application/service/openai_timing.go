package service

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/openaitiming"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAITimingContextKey = "openai_turn_timing"

func beginOpenAITimingObservation(c *gin.Context) {
	if c != nil {
		c.Set(openAITimingContextKey, &openaitiming.Collector{})
	}
}

func openAITimingCollector(c *gin.Context) *openaitiming.Collector {
	if c == nil {
		return nil
	}
	value, exists := c.Get(openAITimingContextKey)
	if !exists {
		beginOpenAITimingObservation(c)
		value, _ = c.Get(openAITimingContextKey)
	}
	collector, _ := value.(*openaitiming.Collector)
	return collector
}

func observeOpenAITiming(c *gin.Context, payload []byte, eventType string) {
	switch eventType {
	case openaitiming.EventType, "response.created", "response.in_progress", "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		openAITimingCollector(c).Observe(payload, eventType)
	}
}

func observeOpenAITimingBody(c *gin.Context, body []byte) {
	if bodyHasSSEFraming(body) {
		forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
			observeOpenAITiming(c, payload, gjson.GetBytes(payload, "type").String())
		})
		return
	}
	observeOpenAITiming(c, body, gjson.GetBytes(body, "type").String())
}

func observedOpenAITiming(c *gin.Context) *openaitiming.Metrics {
	return openAITimingCollector(c).Snapshot()
}
