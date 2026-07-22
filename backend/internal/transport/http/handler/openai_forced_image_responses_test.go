package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIForcedImageChildWriterCapturesStreamingOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	events := make([]map[string]any, 0, 3)
	writer := newOpenAIForcedImageChildWriter(context.Writer, true, func(event map[string]any) {
		events = append(events, event)
	})

	_, err := writer.WriteString("event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"in_progress\"}}\n\n")
	require.NoError(t, err)
	_, err = writer.WriteString(": keepalive\n\n")
	require.NoError(t, err)
	_, err = writer.WriteString("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"completed\",\"result\":\"aW1hZ2U=\"}],\"usage\":{\"input_tokens\":2,\"output_tokens\":3}}}\n\n")
	require.NoError(t, err)

	output, usage, err := writer.response()
	require.NoError(t, err)
	require.Equal(t, "aW1hZ2U=", output["result"])
	require.Equal(t, json.Number("2"), usage["input_tokens"])
	require.Len(t, events, 2)
}

func TestMergeOpenAIForcedImageUsageSumsNestedTokenDetails(t *testing.T) {
	target := map[string]any{
		"input_tokens": json.Number("2"),
		"input_tokens_details": map[string]any{
			"image_tokens": json.Number("5"),
		},
	}
	mergeOpenAIForcedImageUsage(target, map[string]any{
		"input_tokens": json.Number("3"),
		"input_tokens_details": map[string]any{
			"image_tokens": json.Number("7"),
		},
	})

	require.Equal(t, json.Number("5"), target["input_tokens"])
	details, ok := target["input_tokens_details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, json.Number("12"), details["image_tokens"])
}

func TestRunOpenAIForcedImageChildInGlobalSlotLimitsProcessConcurrency(t *testing.T) {
	const children = 12
	entered := make(chan int, children)
	release := make(chan struct{})
	results := make(chan openAIForcedImageChildResult, children)
	var workers sync.WaitGroup
	workers.Add(children)
	for index := 0; index < children; index++ {
		index := index
		go func() {
			defer workers.Done()
			results <- runOpenAIForcedImageChildInGlobalSlot(context.Background(), index, func() openAIForcedImageChildResult {
				entered <- index
				<-release
				return openAIForcedImageChildResult{index: index}
			})
		}()
	}

	for i := 0; i < openAIForcedImageChildConcurrency; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("expected four image children to enter the global slot")
		}
	}
	select {
	case index := <-entered:
		t.Fatalf("image child %d exceeded the process-wide concurrency limit", index)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	workers.Wait()
	close(results)
	for result := range results {
		require.NoError(t, result.err)
	}
	require.Empty(t, openAIForcedImageGlobalChildSlots)
}

func TestRunOpenAIForcedImageChildInGlobalSlotReleasesSlotAfterPanic(t *testing.T) {
	result := runOpenAIForcedImageChildInGlobalSlot(context.Background(), 7, func() openAIForcedImageChildResult {
		panic("forced test panic")
	})

	require.Equal(t, 7, result.index)
	require.Error(t, result.err)
	require.True(t, strings.Contains(result.err.Error(), "forced test panic"))
	require.Empty(t, openAIForcedImageGlobalChildSlots)
}
