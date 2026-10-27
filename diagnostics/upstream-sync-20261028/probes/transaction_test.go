//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/tidwall/gjson"
)

type upstreamSync20261028RateRepo struct {
	mockAccountRepoForGemini
	resetAt time.Time
}

func (r *upstreamSync20261028RateRepo) SetRateLimited(_ context.Context, _ int64, until time.Time) error {
	r.resetAt = until
	return nil
}

func TestUpstreamSync20261028Transaction(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"abcdefgh😀ijklmnopqrstuvwxyz"}]}`)
	t.Logf("OBS fingerprint=%s", computeClaudeCodeFingerprint(body, "2.1.161"))
	c, _ := newAntigravityCompatContext(http.MethodPost, "/", nil)
	beginGeminiImageOutputObservation(c)
	observeGeminiImageOutputs(c, []byte(`{"candidates":[{"content":{"parts":[{"fileData":{"mimeType":"image/png","fileUri":"https://fixture.invalid/a.png"}},{"file_data":{"mime_type":"image/webp","file_uri":"gs://fixture/b.webp"}}]}}]}`))
	t.Logf("OBS gemini_file_images=%d", observedGeminiImageOutputs(c))
	bs := newTestBillingService()
	price, err := bs.getModelPricingAt("deepseek-v4-pro", time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	if err != nil { t.Fatal(err) }
	t.Logf("OBS deepseek_pro_input=%.9f output=%.9f cache=%.9f", price.InputPricePerToken, price.OutputPricePerToken, price.CacheReadPricePerToken)
	t.Logf("OBS deepseek_cache_hit=%d", openAICacheReadTokensFromUsage(gjson.Parse(`{"prompt_tokens":1000,"prompt_cache_hit_tokens":600}`)))
	chat, err := normalizeStrictChatDeveloperRoles(&Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}, "https://opencode.ai/zen/v1/chat/completions", []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"developer","content":"fixture"}]}`))
	if err != nil { t.Fatal(err) }
	t.Logf("OBS opencode_role=%s", gjson.GetBytes(chat, "messages.0.role").String())
	params := pagination.PaginationParams{Page: math.MaxInt, PageSize: 20}
	offset := params.Offset()
	t.Logf("OBS pagination_nonnegative=%t addition_safe=%t", offset >= 0, offset >= 0 && offset <= math.MaxInt-params.Limit())
	for _, percent := range []float64{12, 100} {
		repo := &upstreamSync20261028RateRepo{}
		svc := NewRateLimitService(repo, nil, nil, nil, nil)
		svc.handle429(context.Background(), &Account{ID: 1, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Extra: map[string]any{"zhipu_5h_used_percent": percent, "zhipu_5h_reset_at": time.Now().Add(4*time.Hour).Format(time.RFC3339)}}, http.Header{}, []byte(`{"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`), "glm-5")
		t.Logf("OBS cn_frequency percent=%.0f short_cooldown=%t", percent, !repo.resetAt.IsZero() && time.Until(repo.resetAt) < time.Minute)
	}
	group := Group{ID: 1, Name: "fixture", Platform: PlatformOpenAI, Status: StatusActive, ImagePrice1K: testPtrFloat64(.9), ModelPricing: []ChannelModelPricing{{Models: []string{"fixture-image"}, BillingMode: BillingModeImage, PerRequestPrice: testPtrFloat64(.12)}}}
	group.compileModelPricingIndex()
	channel := Channel{ID: 1, Name: "fixture", Status: StatusActive, GroupIDs: []int64{1}, ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"fixture-image"}, BillingMode: BillingModeImage, PerRequestPrice: testPtrFloat64(.5)}}}
	plaza, err := newPlazaChannelService([]Channel{channel}, []Group{group}, nil).ListPlazaGroups(context.Background(), PlazaListOptions{})
	if err != nil || len(plaza) != 1 || len(plaza[0].Models) != 1 { t.Fatalf("plaza: %v %v", plaza, err) }
	card := plaza[0].Models[0].Pricing
	display := *card.PerRequestPrice
	if len(card.Intervals) > 0 { display = *card.Intervals[0].PerRequestPrice }
	t.Logf("OBS plaza_fixed_image=%.2f", display)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	frames := "data: {\"response\":{\"candidates\":[{\"finishReason\":\"MALFORMED_FUNCTION_CALL\",\"content\":{\"parts\":[{\"thoughtSignature\":\"fixture\"}]}}]}}\n\n"
	cc, rec := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
	_, err = svc.handleResponsesNonStreamingFromAntigravity(cc, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(frames))}, time.Now(), "gemini-test")
	t.Logf("OBS malformed_status=%d error=%t success=%t", rec.Code, err != nil, strings.Contains(rec.Body.String(), `"status":"completed"`))
	image := bytes.Repeat([]byte{'x'}, 21<<20)
	data, err := readOpenAIImageMultipartPart(bytes.NewReader(image), "image")
	t.Logf("OBS openai_upload_21mb accepted=%t bytes=%d", err == nil, len(data))
	var multipartBody bytes.Buffer
	w := multipart.NewWriter(&multipartBody)
	f, err := w.CreateFormFile("image", "fixture.png")
	if err != nil { t.Fatal(err) }
	if _, err = f.Write(image); err != nil { t.Fatal(err) }
	if err = w.Close(); err != nil { t.Fatal(err) }
	info := ParseGrokMediaRequest(w.FormDataContentType(), multipartBody.Bytes())
	if len(info.Uploads) != 1 { t.Fatalf("uploads=%d", len(info.Uploads)) }
	t.Logf("OBS grok_upload_21mb bytes=%d complete=%t", len(info.Uploads[0].Data), bytes.Equal(image, info.Uploads[0].Data))
}
