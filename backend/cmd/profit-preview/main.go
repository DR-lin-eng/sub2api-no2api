// profit-preview evaluates a read-only JSON export before profit control is
// enabled for production groups.
//
// Usage:
//
//	go run ./cmd/profit-preview -input dump.json [-assume-enabled] [-json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
)

type inputGroup struct {
	ID                   int64   `json:"id"`
	Name                 string  `json:"name"`
	Platform             string  `json:"platform"`
	RateMultiplier       float64 `json:"rate_multiplier"`
	SubscriptionType     string  `json:"subscription_type"`
	ProfitControlEnabled bool    `json:"profit_control_enabled"`
	ProfitMinMargin      float64 `json:"profit_min_margin"`
	ProfitSafetyBuffer   float64 `json:"profit_safety_buffer"`
	PeakRateEnabled      bool    `json:"peak_rate_enabled"`
	PeakStart            string  `json:"peak_start"`
	PeakEnd              string  `json:"peak_end"`
	PeakRateMultiplier   float64 `json:"peak_rate_multiplier"`
}

type inputAccount struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	Platform       string            `json:"platform"`
	Type           string            `json:"type"`
	RateMultiplier *float64          `json:"rate_multiplier"`
	Extra          map[string]any    `json:"extra"`
	ModelMapping   map[string]string `json:"model_mapping"`
}

type inputEntry struct {
	Group         inputGroup          `json:"group"`
	Accounts      []inputAccount      `json:"accounts"`
	UserOverrides map[string]*float64 `json:"user_overrides"`
	Models        []string            `json:"models"`
}

type inputDoc struct {
	Groups []inputEntry `json:"groups"`
}

func main() {
	inputPath := flag.String("input", "", "read-only production export JSON")
	assumeEnabled := flag.Bool("assume-enabled", false, "evaluate saved settings for currently disabled groups")
	jsonOut := flag.Bool("json", false, "write the complete report as JSON")
	flag.Parse()
	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: profit-preview -input dump.json [-assume-enabled] [-json]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(1)
	}
	inputs, err := parsePreviewInputs(raw, *assumeEnabled)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse input: %v\n", err)
		os.Exit(1)
	}

	evalAt := time.Now()
	reports := service.PreviewProfitAdmission(inputs, evalAt)
	if len(reports) == 0 {
		fmt.Fprintln(os.Stderr, "input produced no preview reports")
		os.Exit(1)
	}

	if *jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(map[string]any{"evaluated_at": evalAt, "reports": reports}); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("利润门预演 @ %s（U=账号倍率；探测状态仅告警）\n", evalAt.Format(time.RFC3339))
	for _, report := range reports {
		fmt.Printf("\n== 分组 %d %s [%s] ==\n", report.GroupID, report.GroupName, report.Platform)
		fmt.Printf("  利润门生效=%v 假定启用=%v | 默认 D=%.4f 阈值=%.4f | 最低有效 D=%.4f 阈值=%.4f\n",
			report.EffectiveGate, report.AssumedEnabled,
			report.DefaultD, report.ThresholdDefault, report.MinEffectiveD, report.ThresholdMinD)
		counts := make(map[string]int, 3)
		for _, verdict := range report.Verdicts {
			counts[verdict.Class]++
			rate := "-"
			if verdict.AccountRate != nil {
				rate = fmt.Sprintf("%.4f", *verdict.AccountRate)
			}
			flags := make([]string, 0, 2)
			if verdict.RejectedUnderMinD {
				flags = append(flags, "最低有效D下拒绝")
			}
			if len(verdict.Warnings) > 0 {
				flags = append(flags, strings.Join(verdict.Warnings, ","))
			}
			suffix := ""
			if len(flags) > 0 {
				suffix = "  [" + strings.Join(flags, "; ") + "]"
			}
			fmt.Printf("  账号 %-4d %-24s 平台=%-12s U=%-8s 来源=%-19s %s%s\n",
				verdict.AccountID, verdict.Name, verdict.Platform, rate, verdict.RateSource, verdict.Class, suffix)
		}
		fmt.Printf("  分类合计: 准入=%d 利润不足=%d 倍率非法=%d\n",
			counts[service.ProfitPreviewClassAdmitted],
			counts[service.ProfitPreviewClassRejectedThreshold],
			counts[service.ProfitPreviewClassRejectedInvalidRate])
		models := make([]string, 0, len(report.RemainingByModel))
		for model := range report.RemainingByModel {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			fmt.Printf("  模型 %-20s 利润门准入账号: 默认D=%d 最低有效D=%d\n",
				model, report.RemainingByModel[model], report.RemainingByModelMinD[model])
		}
		for _, model := range modelsWithZeroRemaining(report) {
			fmt.Printf("  警告: 模型 %s 启用后利润门准入账号为 0\n", model)
		}
		for _, model := range modelsWithZeroRemainingUnderMinD(report) {
			fmt.Printf("  警告: 模型 %s 在最低有效D（存在低倍率用户覆盖）下利润门准入账号为 0\n", model)
		}
	}
}

func parsePreviewInputs(raw []byte, assumeEnabled bool) ([]service.ProfitPreviewGroupInput, error) {
	var doc inputDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Groups) == 0 {
		return nil, fmt.Errorf("input contains no groups; check the export query and target configuration")
	}

	inputs := make([]service.ProfitPreviewGroupInput, 0, len(doc.Groups))
	for index, entry := range doc.Groups {
		if entry.Group.ID <= 0 || strings.TrimSpace(entry.Group.Platform) == "" {
			return nil, fmt.Errorf("invalid group at index %d: id and platform are required", index)
		}
		group := &service.Group{
			ID:                   entry.Group.ID,
			Name:                 entry.Group.Name,
			Platform:             entry.Group.Platform,
			Status:               service.StatusActive,
			Hydrated:             true,
			RateMultiplier:       entry.Group.RateMultiplier,
			SubscriptionType:     entry.Group.SubscriptionType,
			ProfitControlEnabled: entry.Group.ProfitControlEnabled,
			ProfitMinMargin:      entry.Group.ProfitMinMargin,
			ProfitSafetyBuffer:   entry.Group.ProfitSafetyBuffer,
			PeakRateEnabled:      entry.Group.PeakRateEnabled,
			PeakStart:            entry.Group.PeakStart,
			PeakEnd:              entry.Group.PeakEnd,
			PeakRateMultiplier:   entry.Group.PeakRateMultiplier,
		}
		accounts := make([]*service.Account, 0, len(entry.Accounts))
		for _, accountInput := range entry.Accounts {
			account := &service.Account{
				ID:             accountInput.ID,
				Name:           accountInput.Name,
				Platform:       accountInput.Platform,
				Type:           accountInput.Type,
				RateMultiplier: accountInput.RateMultiplier,
				Extra:          accountInput.Extra,
			}
			if len(accountInput.ModelMapping) > 0 {
				mapping := make(map[string]any, len(accountInput.ModelMapping))
				for source, target := range accountInput.ModelMapping {
					mapping[source] = target
				}
				account.Credentials = map[string]any{"model_mapping": mapping}
			}
			accounts = append(accounts, account)
		}
		overrides := make(map[int64]float64, len(entry.UserOverrides))
		for userID, rate := range entry.UserOverrides {
			if rate == nil {
				continue
			}
			id, err := strconv.ParseInt(userID, 10, 64)
			if err == nil && id > 0 {
				overrides[id] = *rate
			}
		}
		inputs = append(inputs, service.ProfitPreviewGroupInput{
			Group:         group,
			Accounts:      accounts,
			UserOverrides: overrides,
			Models:        entry.Models,
			AssumeEnabled: assumeEnabled,
		})
	}
	return inputs, nil
}

func modelsWithZeroRemaining(report service.ProfitPreviewGroupReport) []string {
	var models []string
	for model, count := range report.RemainingByModel {
		if count == 0 {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}

func modelsWithZeroRemainingUnderMinD(report service.ProfitPreviewGroupReport) []string {
	var models []string
	for model, count := range report.RemainingByModelMinD {
		if count == 0 && report.RemainingByModel[model] > 0 {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}
