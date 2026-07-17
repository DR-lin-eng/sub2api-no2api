package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestThinkingDisplayNeedsOptIn(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		// display 默认 omitted 的模型族：必须显式补 summarized
		{"opus-4-8", "claude-opus-4-8", true},
		{"opus-4-7", "claude-opus-4-7", true},
		{"sonnet-5", "claude-sonnet-5", true},
		{"fable-5", "claude-fable-5", true},
		{"mythos-5", "claude-mythos-5", true},
		{"大写", "Claude-Opus-4-8", true},
		{"带空格", "  claude-sonnet-5  ", true},
		{"Bedrock 前缀", "anthropic.claude-opus-4-8", true},
		{"带部署后缀", "claude-opus-4-8[1m]", true},
		{"带日期后缀", "claude-opus-4-8-20260701", true},

		// display 默认已是 summarized，不需要也不应该改
		{"opus-4-6", "claude-opus-4-6", false},
		{"sonnet-4-6", "claude-sonnet-4-6", false},

		// 更老的模型：不认识 adaptive，绝不能碰
		{"sonnet-4-5", "claude-sonnet-4-5", false},
		{"opus-4-5 带日期", "claude-opus-4-5-20251101", false},
		{"haiku-4-5", "claude-haiku-4-5-20251001", false},
		{"opus-4-1", "claude-opus-4-1", false},
		{"相邻 minor 版本", "claude-opus-4-80", false},
		{"相邻主版本", "claude-sonnet-50", false},
		{"无分隔部署后缀", "claude-fable-5preview", false},

		// 非 Anthropic / 空
		{"空", "", false},
		{"gpt", "gpt-5.1", false},
		{"deepseek", "deepseek-v4-pro", false},
		{"gemini", "gemini-3-pro-preview", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := thinkingDisplayNeedsOptIn(tt.modelID); got != tt.want {
				t.Errorf("thinkingDisplayNeedsOptIn(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

// 4-6 与 4-8 只差一个字符，任何 "claude-opus-4" 级别的前缀匹配都会把两者混为一谈，
// 而它们的 display 默认值恰好相反。单独立一个测试钉住这条边界。
func TestThinkingDisplayNeedsOptIn_MinorVersionBoundary(t *testing.T) {
	if !thinkingDisplayNeedsOptIn("claude-opus-4-8") {
		t.Fatal("claude-opus-4-8 必须命中")
	}
	if thinkingDisplayNeedsOptIn("claude-opus-4-6") {
		t.Fatal("claude-opus-4-6 的 display 默认已是 summarized，不得命中")
	}
}

func TestNormalizeAnthropicThinkingDisplay(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		model   string
		mode    string
		stream  bool
		applied bool
		// 断言：改写后 body 上各字段的期望值（"" 表示期望该字段不存在）
		wantType    string
		wantDisplay string
		wantBudget  string // "" = 必须已被删除
		wantMax     int64  // 0 = 不校验
	}{
		// —— 免费档：已经在思考，只是摘要被隐藏 ——
		{
			name:  "adaptive 缺 display：补齐（零成本）",
			body:  `{"model":"claude-opus-4-8","max_tokens":1024,"thinking":{"type":"adaptive"}}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeDisplayOnly, stream: true,
			applied: true, wantType: "adaptive", wantDisplay: "summarized",
			wantMax: 1024, // display_only 不得改动 max_tokens
		},
		{
			name:  "客户端显式设了 display：尊重，不覆盖",
			body:  `{"model":"claude-opus-4-8","thinking":{"type":"adaptive","display":"omitted"}}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeForce, stream: true,
			applied: false, wantType: "adaptive", wantDisplay: "omitted",
		},

		// —— 老写法归一化：原样转发必 400 ——
		{
			name:  "enabled+budget_tokens：改写为 adaptive 并删除 budget",
			body:  `{"model":"claude-opus-4-8","max_tokens":64000,"thinking":{"type":"enabled","budget_tokens":32000}}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeDisplayOnly, stream: true,
			applied: true, wantType: "adaptive", wantDisplay: "summarized", wantBudget: "",
			wantMax: 64000, // 老写法本就为思考留过余量，不必抬高
		},

		// —— force 档：真正开启思考 ——
		{
			name:  "无 thinking + display_only：不注入",
			body:  `{"model":"claude-opus-4-8","max_tokens":1024}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeDisplayOnly, stream: true,
			applied: false, wantMax: 1024,
		},
		{
			name:  "无 thinking + force：注入并抬高 max_tokens（流式）",
			body:  `{"model":"claude-opus-4-8","max_tokens":1024}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeForce, stream: true,
			applied: true, wantType: "adaptive", wantDisplay: "summarized",
			wantMax: thinkingForceMaxTokens,
		},
		{
			name:  "无 thinking + force：非流式抬高到更保守的上限",
			body:  `{"model":"claude-opus-4-8","max_tokens":1024}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeForce, stream: false,
			applied: true, wantType: "adaptive", wantDisplay: "summarized",
			wantMax: thinkingForceMaxTokensNonStream,
		},
		{
			name:  "无 thinking + force：不得下调已经够大的 max_tokens",
			body:  `{"model":"claude-opus-4-8","max_tokens":128000}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeForce, stream: true,
			applied: true, wantType: "adaptive", wantDisplay: "summarized",
			wantMax: 128000,
		},
		{
			name:  "显式 disabled + force：尊重用户意图，不覆盖",
			body:  `{"model":"claude-opus-4-8","max_tokens":1024,"thinking":{"type":"disabled"}}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeForce, stream: true,
			applied: false, wantType: "disabled", wantMax: 1024,
		},

		// —— 模式与模型的门禁 ——
		{
			name:  "off 模式：什么都不做",
			body:  `{"model":"claude-opus-4-8","thinking":{"type":"adaptive"}}`,
			model: "claude-opus-4-8", mode: ThinkingDisplayModeOff, stream: true,
			applied: false, wantType: "adaptive", wantDisplay: "",
		},
		{
			name:  "未知模式：视为不启用",
			body:  `{"model":"claude-opus-4-8","thinking":{"type":"adaptive"}}`,
			model: "claude-opus-4-8", mode: "bogus", stream: true,
			applied: false, wantDisplay: "",
		},
		{
			name:  "opus-4-6：display 默认已是 summarized，不得改",
			body:  `{"model":"claude-opus-4-6","thinking":{"type":"adaptive"}}`,
			model: "claude-opus-4-6", mode: ThinkingDisplayModeForce, stream: true,
			applied: false, wantDisplay: "",
		},
		{
			name:  "sonnet-4-5：不认识 adaptive，force 也不得注入",
			body:  `{"model":"claude-sonnet-4-5","max_tokens":1024}`,
			model: "claude-sonnet-4-5", mode: ThinkingDisplayModeForce, stream: true,
			applied: false, wantType: "", wantMax: 1024,
		},
		{
			name:  "非 Anthropic 上游：不得注入",
			body:  `{"model":"deepseek-v4-pro","max_tokens":1024}`,
			model: "deepseek-v4-pro", mode: ThinkingDisplayModeForce, stream: true,
			applied: false, wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, applied := NormalizeAnthropicThinkingDisplay([]byte(tt.body), tt.model, tt.mode, tt.stream)
			if applied != tt.applied {
				t.Fatalf("applied = %v, want %v (body=%s)", applied, tt.applied, got)
			}
			if !applied && string(got) != tt.body {
				t.Fatalf("applied=false 时必须原样返回\n got: %s\nwant: %s", got, tt.body)
			}
			if tt.wantType != "" {
				if v := gjson.GetBytes(got, "thinking.type").String(); v != tt.wantType {
					t.Errorf("thinking.type = %q, want %q", v, tt.wantType)
				}
			}
			if tt.wantDisplay != "" {
				if v := gjson.GetBytes(got, "thinking.display").String(); v != tt.wantDisplay {
					t.Errorf("thinking.display = %q, want %q", v, tt.wantDisplay)
				}
			} else if tt.applied && tt.wantType == "adaptive" {
				t.Errorf("注入 adaptive 时必须同时写入 display")
			}
			if tt.wantBudget == "" && gjson.GetBytes(got, "thinking.budget_tokens").Exists() {
				t.Errorf("budget_tokens 必须被删除，实得 %s", gjson.GetBytes(got, "thinking.budget_tokens").Raw)
			}
			if tt.wantMax != 0 {
				if v := gjson.GetBytes(got, "max_tokens").Int(); v != tt.wantMax {
					t.Errorf("max_tokens = %d, want %d", v, tt.wantMax)
				}
			}
		})
	}
}

// 出错/异常输入时必须 fail-safe 返回原 body，与本链路其余整流器一致。
func TestNormalizeAnthropicThinkingDisplay_FailSafe(t *testing.T) {
	for _, body := range []string{
		``,
		`not json at all`,
		`{"model":"claude-opus-4-8"`,
	} {
		got, applied := NormalizeAnthropicThinkingDisplay([]byte(body), "claude-opus-4-8", ThinkingDisplayModeDisplayOnly, true)
		if applied {
			t.Errorf("畸形 body %q 不应报告 applied=true（得到 %s）", body, got)
		}
	}

	for _, body := range []string{
		`not json at all`,
		`{"model":"claude-opus-4-8"`,
	} {
		got, applied := NormalizeAnthropicThinkingDisplay([]byte(body), "claude-opus-4-8", ThinkingDisplayModeForce, true)
		if applied || string(got) != body {
			t.Errorf("force 模式不得改写畸形 body %q（applied=%v, got=%s）", body, applied, got)
		}
	}
}

// 相同输入必须产生逐字节相同的输出：本链路对 body 字节序有既有依赖
// （见 gateway_body_order_test.go 与 400 重试对已签名 body 的处理）。
func TestNormalizeAnthropicThinkingDisplay_Deterministic(t *testing.T) {
	const body = `{"model":"claude-opus-4-8","max_tokens":1024,"messages":[]}`
	first, applied := NormalizeAnthropicThinkingDisplay([]byte(body), "claude-opus-4-8", ThinkingDisplayModeForce, true)
	if !applied {
		t.Fatal("force 模式应注入")
	}
	for i := 0; i < 50; i++ {
		next, _ := NormalizeAnthropicThinkingDisplay([]byte(body), "claude-opus-4-8", ThinkingDisplayModeForce, true)
		if string(next) != string(first) {
			t.Fatalf("输出字节序不稳定\n第一次: %s\n第 %d 次: %s", first, i+2, next)
		}
	}
}

type thinkingDisplaySettingRepo struct {
	value    atomic.Value // string
	getCalls atomic.Int64
	failRead atomic.Bool
}

func newThinkingDisplaySettingRepo(value string) *thinkingDisplaySettingRepo {
	repo := &thinkingDisplaySettingRepo{}
	repo.value.Store(value)
	return repo
}

func (r *thinkingDisplaySettingRepo) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (r *thinkingDisplaySettingRepo) GetValue(context.Context, string) (string, error) {
	r.getCalls.Add(1)
	if r.failRead.Load() {
		return "", errors.New("forced thinking display setting read failure")
	}
	value, ok := r.value.Load().(string)
	if !ok {
		return "", errors.New("thinking display test setting is not a string")
	}
	return value, nil
}

func (r *thinkingDisplaySettingRepo) Set(_ context.Context, _ string, value string) error {
	r.value.Store(value)
	return nil
}

func (r *thinkingDisplaySettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (r *thinkingDisplaySettingRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (r *thinkingDisplaySettingRepo) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (r *thinkingDisplaySettingRepo) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestGetThinkingDisplayModeCachesAndPublishes(t *testing.T) {
	repo := newThinkingDisplaySettingRepo(`{"enabled":true,"thinking_display_mode":"force"}`)
	svc := NewSettingService(repo, nil)
	ctx := context.Background()

	if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeForce {
		t.Fatalf("initial mode = %q, want %q", got, ThinkingDisplayModeForce)
	}
	for i := 0; i < 100; i++ {
		if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeForce {
			t.Fatalf("cached mode = %q, want %q", got, ThinkingDisplayModeForce)
		}
	}
	if calls := repo.getCalls.Load(); calls != 1 {
		t.Fatalf("cached reads queried repository %d times, want 1", calls)
	}

	if err := svc.SetRectifierSettings(ctx, &RectifierSettings{
		Enabled:             true,
		ThinkingDisplayMode: ThinkingDisplayModeOff,
	}); err != nil {
		t.Fatalf("SetRectifierSettings() error = %v", err)
	}
	if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeOff {
		t.Fatalf("published mode = %q, want %q", got, ThinkingDisplayModeOff)
	}
	if calls := repo.getCalls.Load(); calls != 1 {
		t.Fatalf("published cache unexpectedly queried repository %d times", calls)
	}

	repo.value.Store(`{"enabled":true,"thinking_display_mode":"display_only"}`)
	svc.thinkingDisplayModeLoaded.Store(time.Now().Add(-2 * thinkingDisplayModeRefreshInterval).UnixNano())
	if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeDisplayOnly {
		t.Fatalf("refreshed mode = %q, want %q", got, ThinkingDisplayModeDisplayOnly)
	}
	if calls := repo.getCalls.Load(); calls != 2 {
		t.Fatalf("refresh queried repository %d times, want 2", calls)
	}

	repo.value.Store(`{"enabled":true,"thinking_display_mode":"force"}`)
	svc.thinkingDisplayModeLoaded.Store(time.Now().Add(-2 * thinkingDisplayModeRefreshInterval).UnixNano())
	if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeForce {
		t.Fatalf("mode before read failure = %q, want %q", got, ThinkingDisplayModeForce)
	}
	repo.failRead.Store(true)
	svc.thinkingDisplayModeLoaded.Store(time.Now().Add(-2 * thinkingDisplayModeRefreshInterval).UnixNano())
	if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeDisplayOnly {
		t.Fatalf("mode after read failure = %q, want safe fallback %q", got, ThinkingDisplayModeDisplayOnly)
	}
}

func BenchmarkGetThinkingDisplayModeCached(b *testing.B) {
	repo := newThinkingDisplaySettingRepo(`{"enabled":true,"thinking_display_mode":"display_only"}`)
	svc := NewSettingService(repo, nil)
	ctx := context.Background()
	if got := svc.GetThinkingDisplayMode(ctx); got != ThinkingDisplayModeDisplayOnly {
		b.Fatalf("initial mode = %q", got)
	}
	repo.getCalls.Store(0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.GetThinkingDisplayMode(ctx)
	}
	b.StopTimer()
	if calls := repo.getCalls.Load(); calls != 0 {
		b.Fatalf("cached benchmark queried repository %d times", calls)
	}
}
