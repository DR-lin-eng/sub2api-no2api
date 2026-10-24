//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMultimodalMediaUpstreamPath(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  MultimodalMediaEndpoint
		requestID string
		want      string
		wantErr   bool
	}{
		{name: "images generations", endpoint: MultimodalMediaEndpointImagesGenerations, want: "/v1/images/generations"},
		{name: "images edits", endpoint: MultimodalMediaEndpointImagesEdits, want: "/v1/images/edits"},
		{name: "videos generations", endpoint: MultimodalMediaEndpointVideosGenerations, want: "/v1/videos/generations"},
		// 上游媒体分组没有独立的 edits/extensions 路径，必须落到 generations。
		{name: "videos edits falls back to generations", endpoint: MultimodalMediaEndpointVideosEdits, want: "/v1/videos/generations"},
		{name: "videos extensions falls back to generations", endpoint: MultimodalMediaEndpointVideosExtensions, want: "/v1/videos/generations"},
		{name: "video status", endpoint: MultimodalMediaEndpointVideoStatus, requestID: "task_123", want: "/v1/videos/task_123"},
		{name: "video status trims slashes", endpoint: MultimodalMediaEndpointVideoStatus, requestID: "/task_123/", want: "/v1/videos/task_123"},
		{name: "video content", endpoint: MultimodalMediaEndpointVideoContent, requestID: "task_123", want: "/v1/videos/task_123/content"},
		{name: "unsupported endpoint", endpoint: MultimodalMediaEndpoint("unknown"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := multimodalMediaUpstreamPath(tt.endpoint, tt.requestID)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeMultimodalVideoReferenceBody(t *testing.T) {
	tests := []struct {
		name     string
		endpoint MultimodalMediaEndpoint
		body     string
		wantRefs []string
		wantBody string
	}{
		{
			name:     "generations endpoint is untouched",
			endpoint: MultimodalMediaEndpointVideosGenerations,
			body:     `{"model":"seedance","video":"https://cdn.example.com/a.mp4"}`,
			wantBody: `{"model":"seedance","video":"https://cdn.example.com/a.mp4"}`,
		},
		{
			name:     "single video string on edits",
			endpoint: MultimodalMediaEndpointVideosEdits,
			body:     `{"model":"seedance","prompt":"wave","video":"https://cdn.example.com/a.mp4"}`,
			wantRefs: []string{"https://cdn.example.com/a.mp4"},
		},
		{
			name:     "video_url alias on extensions",
			endpoint: MultimodalMediaEndpointVideosExtensions,
			body:     `{"model":"seedance","video_url":"https://cdn.example.com/b.mp4"}`,
			wantRefs: []string{"https://cdn.example.com/b.mp4"},
		},
		{
			name:     "videos array of strings",
			endpoint: MultimodalMediaEndpointVideosEdits,
			body:     `{"model":"seedance","videos":["https://cdn.example.com/a.mp4","https://cdn.example.com/b.mp4"]}`,
			wantRefs: []string{"https://cdn.example.com/a.mp4", "https://cdn.example.com/b.mp4"},
		},
		{
			name:     "videos array of objects",
			endpoint: MultimodalMediaEndpointVideosEdits,
			body:     `{"model":"seedance","videos":[{"url":"https://cdn.example.com/a.mp4"}]}`,
			wantRefs: []string{"https://cdn.example.com/a.mp4"},
		},
		{
			name:     "existing referenceVideos wins",
			endpoint: MultimodalMediaEndpointVideosEdits,
			body:     `{"model":"seedance","referenceVideos":["https://cdn.example.com/keep.mp4"],"video":"https://cdn.example.com/drop.mp4"}`,
			wantRefs: []string{"https://cdn.example.com/keep.mp4"},
		},
		{
			name:     "no reference field leaves body untouched",
			endpoint: MultimodalMediaEndpointVideosEdits,
			body:     `{"model":"seedance","prompt":"wave"}`,
			wantBody: `{"model":"seedance","prompt":"wave"}`,
		},
		{
			name:     "invalid json leaves body untouched",
			endpoint: MultimodalMediaEndpointVideosEdits,
			body:     `not-json`,
			wantBody: `not-json`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeMultimodalVideoReferenceBody(tt.endpoint, []byte(tt.body))
			require.NoError(t, err)
			if tt.wantBody != "" {
				require.JSONEq(t, tt.wantBody, string(got))
				return
			}
			refs := gjson.GetBytes(got, "referenceVideos")
			require.True(t, refs.IsArray(), "referenceVideos must be an array: %s", string(got))
			actual := make([]string, 0, len(refs.Array()))
			for _, item := range refs.Array() {
				actual = append(actual, item.String())
			}
			require.Equal(t, tt.wantRefs, actual)
			// 旧字段必须被删除，避免上游把未知字段当成新的输入。
			for _, legacy := range []string{"video", "video_url", "videos"} {
				require.False(t, gjson.GetBytes(got, legacy).Exists(), "%s must be removed: %s", legacy, string(got))
			}
		})
	}
}

func TestMultimodalVideoBillingDurationSeconds(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		requestInfo GrokMediaRequestInfo
		want        int
	}{
		{
			name:        "negative duration freezes max window",
			body:        `{"model":"seedance","duration":-1}`,
			requestInfo: GrokMediaRequestInfo{DurationSeconds: VideoBillingMinDurationSeconds},
			want:        VideoBillingMaxDurationSeconds,
		},
		{
			name:        "explicit duration is kept",
			body:        `{"model":"seedance","duration":4}`,
			requestInfo: GrokMediaRequestInfo{DurationSeconds: 4},
			want:        4,
		},
		{
			name:        "missing duration uses normalized default",
			body:        `{"model":"seedance"}`,
			requestInfo: GrokMediaRequestInfo{DurationSeconds: VideoBillingDefaultDurationSeconds},
			want:        VideoBillingDefaultDurationSeconds,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, multimodalVideoBillingDurationSeconds(tt.requestInfo, []byte(tt.body)))
		})
	}
}

func TestAggregateMultimodalImageSSE(t *testing.T) {
	t.Run("progressive base64 chunks collapse into one image", func(t *testing.T) {
		body := []byte(
			"data: {\"created\":1,\"data\":[{\"b64_json\":\"/9j/4AAQ\"}]}\n\n" +
				"data: {\"created\":1,\"data\":[{\"b64_json\":\"/9j/4AAQSkZJRg\"}]}\n\n" +
				"data: {\"created\":1,\"data\":[{\"b64_json\":\"/9j/4AAQSkZJRgABAQ==\"}]}\n\n" +
				"data: [DONE]\n\n")
		got, ok := aggregateMultimodalImageSSE(body)
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(got, "data").Array(), 1)
		require.Equal(t, "/9j/4AAQSkZJRgABAQ==", gjson.GetBytes(got, "data.0.b64_json").String())
		require.Equal(t, int64(1), gjson.GetBytes(got, "created").Int())
	})

	t.Run("distinct images are kept and usage is preserved", func(t *testing.T) {
		body := []byte(
			"data: {\"data\":[{\"url\":\"https://cdn.example.com/a.png\"}],\"usage\":{\"total_tokens\":7}}\n\n" +
				"data: {\"data\":[{\"url\":\"https://cdn.example.com/b.png\"}]}\n\n")
		got, ok := aggregateMultimodalImageSSE(body)
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(got, "data").Array(), 2)
		require.Equal(t, int64(7), gjson.GetBytes(got, "usage.total_tokens").Int())
	})

	t.Run("no image output reports failure", func(t *testing.T) {
		body := []byte("data: {\"data\":[]}\n\ndata: [DONE]\n\n")
		got, ok := aggregateMultimodalImageSSE(body)
		require.False(t, ok)
		require.Nil(t, got)
	})
}

func TestMultimodalMediaVideoRequestSessionHash(t *testing.T) {
	first := MultimodalMediaVideoRequestSessionHash("task_123", 1, 17)
	second := MultimodalMediaVideoRequestSessionHash("task_123", 1, 17)
	other := MultimodalMediaVideoRequestSessionHash("task_123", 1, 18)

	require.NotEmpty(t, first)
	require.Equal(t, first, second)
	require.NotEqual(t, first, other)
	require.Contains(t, first, multimodalVideoRequestSessionPrefix)

	require.Empty(t, MultimodalMediaVideoRequestSessionHash("", 1, 17))
	require.Empty(t, MultimodalMediaVideoRequestSessionHash("task_123", 0, 17))
	require.Empty(t, MultimodalMediaVideoRequestSessionHash("task_123", 1, 0))
}

func TestIsVideoUsageResult(t *testing.T) {
	require.False(t, isVideoUsageResult(nil))
	require.False(t, isVideoUsageResult(&OpenAIForwardResult{ImageCount: 1, ImageSize: "2K"}))
	require.True(t, isVideoUsageResult(&OpenAIForwardResult{VideoCount: 1, ImageCount: 1}))
}

func TestRenderMultimodalVideoResponse(t *testing.T) {
	t.Run("creation payload is normalized", func(t *testing.T) {
		upstream := []byte(`{
			"created_at": 1792861631,
			"id": "8887df63-f19d-4491-8119-f49da7cfa3a5",
			"job_id": "8887df63-f19d-4491-8119-f49da7cfa3a5",
			"quoted_price_usd": 0.152,
			"status": "processing",
			"task_id": "8887df63-f19d-4491-8119-f49da7cfa3a5",
			"usage": {"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
		}`)
		requestBody := []byte(`{"model":"seedance","prompt":"dog","resolution":"480p","duration":1,"aspect_ratio":"9:16"}`)
		got := renderMultimodalVideoResponse(
			MultimodalMediaEndpointVideosGenerations,
			GrokMediaRequestInfo{Model: "seedance", Resolution: "480p", DurationSeconds: 1},
			requestBody,
			upstream,
		)

		require.Equal(t, "8887df63-f19d-4491-8119-f49da7cfa3a5", gjson.GetBytes(got, "id").String())
		require.Equal(t, "8887df63-f19d-4491-8119-f49da7cfa3a5", gjson.GetBytes(got, "task_id").String())
		require.Equal(t, "video", gjson.GetBytes(got, "object").String())
		require.Equal(t, "processing", gjson.GetBytes(got, "status").String())
		require.Equal(t, "seedance", gjson.GetBytes(got, "model").String())
		require.Equal(t, "480p", gjson.GetBytes(got, "size").String())
		require.Equal(t, int64(1), gjson.GetBytes(got, "seconds").Int())
		require.Equal(t, int64(1792861631), gjson.GetBytes(got, "created_at").Int())
		require.Equal(t, int64(0), gjson.GetBytes(got, "progress").Int())

		for _, leaked := range []string{"quoted_price_usd", "job_id", "usage"} {
			require.False(t, gjson.GetBytes(got, leaked).Exists(), "%s must be removed: %s", leaked, string(got))
		}
	})

	t.Run("status done reports full progress and keeps content url", func(t *testing.T) {
		upstream := []byte(`{"id":"task_1","status":"done","metadata":{"url":"https://upstream.example.com/v1/videos/task_1/content"},"usage":{"total_tokens":12}}`)
		got := renderMultimodalVideoResponse(
			MultimodalMediaEndpointVideoStatus,
			GrokMediaRequestInfo{},
			nil,
			upstream,
		)
		require.Equal(t, "video", gjson.GetBytes(got, "object").String())
		require.Equal(t, int64(100), gjson.GetBytes(got, "progress").Int())
		require.Equal(t, "https://upstream.example.com/v1/videos/task_1/content", gjson.GetBytes(got, "metadata.url").String())
		// 非零 usage 必须保留。
		require.Equal(t, int64(12), gjson.GetBytes(got, "usage.total_tokens").Int())
	})

	t.Run("status polling renames duration and resolution", func(t *testing.T) {
		upstream := []byte(`{"id":"task_2","status":"processing","duration":5,"resolution":"720p"}`)
		got := renderMultimodalVideoResponse(
			MultimodalMediaEndpointVideoStatus,
			GrokMediaRequestInfo{},
			nil,
			upstream,
		)
		require.Equal(t, int64(5), gjson.GetBytes(got, "seconds").Int())
		require.Equal(t, "720p", gjson.GetBytes(got, "size").String())
	})

	t.Run("job_id only payload still yields id", func(t *testing.T) {
		upstream := []byte(`{"job_id":"job_9","status":"queued"}`)
		got := renderMultimodalVideoResponse(
			MultimodalMediaEndpointVideosGenerations,
			GrokMediaRequestInfo{Model: "seedance"},
			[]byte(`{"model":"seedance"}`),
			upstream,
		)
		require.Equal(t, "job_9", gjson.GetBytes(got, "id").String())
		require.Equal(t, "job_9", gjson.GetBytes(got, "task_id").String())
	})

	t.Run("duration -1 omits unknown seconds", func(t *testing.T) {
		upstream := []byte(`{"id":"task_3","status":"queued"}`)
		got := renderMultimodalVideoResponse(
			MultimodalMediaEndpointVideosGenerations,
			GrokMediaRequestInfo{Model: "seedance", Resolution: "480p", DurationSeconds: VideoBillingMaxDurationSeconds},
			[]byte(`{"model":"seedance","duration":-1,"resolution":"480p"}`),
			upstream,
		)
		require.False(t, gjson.GetBytes(got, "seconds").Exists(), "unknown duration must not be reported: %s", string(got))
		require.Equal(t, "480p", gjson.GetBytes(got, "size").String())
	})

	t.Run("non object payload is untouched", func(t *testing.T) {
		upstream := []byte(`[{"id":"task_4"}]`)
		require.JSONEq(t, string(upstream), string(renderMultimodalVideoResponse(
			MultimodalMediaEndpointVideoStatus, GrokMediaRequestInfo{}, nil, upstream,
		)))
	})
}
