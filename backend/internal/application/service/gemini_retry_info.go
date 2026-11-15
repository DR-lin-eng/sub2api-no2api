package service

import (
	"math"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

func geminiRetryInfoResetTime(body []byte) *int64 {
	var reset *int64
	gjson.GetBytes(body, "error.details").ForEach(func(_, detail gjson.Result) bool {
		if !strings.HasSuffix(detail.Get("@type").String(), "/google.rpc.RetryInfo") {
			return true
		}
		delay, err := time.ParseDuration(detail.Get("retryDelay").String())
		if err != nil || delay <= 0 || delay > 24*time.Hour {
			return true
		}
		ts := time.Now().Unix() + int64(math.Ceil(delay.Seconds()))
		reset = &ts
		return false
	})
	return reset
}
