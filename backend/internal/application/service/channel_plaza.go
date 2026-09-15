package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PlazaOfficialPricing 模型广场展示用的 LiteLLM 官方参考价（USD per token）。
// 字段为 nil 表示官方数据中该项缺失（0 视为未配置）。
type PlazaOfficialPricing struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64 // 5m 缓存写入（= LiteLLM cache_creation）
	CacheWrite1hPrice *float64 // 1h 缓存写入（LiteLLM cache_creation_above_1hr）
	CacheReadPrice    *float64
}

// PlazaModel 模型广场中单个模型条目：渠道定价 + 官方参考价。
type PlazaModel struct {
	Name            string
	Platform        string
	Pricing         *ChannelModelPricing
	OfficialPricing *PlazaOfficialPricing
}

// PlazaGroup 模型广场中以分组为顶层的条目。
//
// 与 AvailableGroupRef 相比多了 Description 与 Models；Models 来自该分组关联渠道的
// 支持模型（按分组平台隔离，防跨平台泄漏），与「可用渠道」页口径一致。
type PlazaGroup struct {
	ID                   int64
	Name                 string
	Description          string
	Platform             string
	SubscriptionType     string
	RateMultiplier       float64
	PeakRateEnabled      bool
	PeakStart            string
	PeakEnd              string
	PeakRateMultiplier   float64
	IsExclusive          bool
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	Models               []PlazaModel
}

// PlazaModelSource supplies the schedulable-account model names used by the
// optional automatic public-group catalog. GatewayService implements this
// interface without making ChannelService depend on gateway orchestration.
type PlazaModelSource interface {
	GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string
}

// PlazaListOptions controls display-only model discovery for the plaza.
type PlazaListOptions struct {
	AutoPublicModels bool
	ModelSource      PlazaModelSource
}

// ListPlazaGroups 返回模型广场数据：每个活跃分组附带其可用模型与定价。
//
// 聚合口径与 ListAvailable 一致（Active 渠道、SupportedModels ∪ 全局定价回落、
// 平台隔离），仅把顶层从渠道换成分组：
//   - 渠道按 lower(name) 排序后遍历，保证同名模型去重结果确定；
//   - 同分组同名模型「先见者胜」，仅当已存条目无定价而新条目有定价时升级替换；
//   - 图片计费模型按实收口径合成展示档位价（分组图片价 > 渠道档位价 >
//     渠道默认按次价），不修改渠道定价缓存；
//   - 每个模型附带 LiteLLM 官方参考价（查不到为 nil）；
//   - AutoPublicModels 开启时，非专属分组会补齐平台默认模型与可调度账号模型；
//   - 只返回 Models 非空的分组；分组按 RateMultiplier 升序（同倍率按名称），
//     组内模型按名称排序。
//
// 可见性过滤（专属分组）不在此层做，由 handler 按登录态裁剪。
func (s *ChannelService) ListPlazaGroups(ctx context.Context, opts PlazaListOptions) ([]PlazaGroup, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	sort.SliceStable(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})

	byGroup := make(map[int64]*PlazaGroup, len(groups))
	groupByID := make(map[int64]*Group, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		byGroup[g.ID] = &PlazaGroup{
			ID:                   g.ID,
			Name:                 g.Name,
			Description:          g.Description,
			Platform:             g.Platform,
			SubscriptionType:     g.SubscriptionType,
			RateMultiplier:       g.RateMultiplier,
			PeakRateEnabled:      g.PeakRateEnabled,
			PeakStart:            g.PeakStart,
			PeakEnd:              g.PeakEnd,
			PeakRateMultiplier:   g.PeakRateMultiplier,
			IsExclusive:          g.IsExclusive,
			ImageRateIndependent: g.ImageRateIndependent,
			ImageRateMultiplier:  g.ImageRateMultiplier,
		}
		groupByID[g.ID] = g
		order = append(order, g.ID)
	}

	// modelIdx[groupID][platform/modelName] = index into byGroup[groupID].Models
	modelIdx := make(map[int64]map[string]int, len(groups))
	addModel := func(pg *PlazaGroup, model PlazaModel) {
		model.Name = strings.TrimSpace(model.Name)
		model.Platform = strings.TrimSpace(model.Platform)
		if model.Name == "" || model.Platform == "" {
			return
		}
		model.Pricing = plazaImageDisplayPricing(model.Pricing, groupByID[pg.ID])
		idx := modelIdx[pg.ID]
		if idx == nil {
			idx = make(map[string]int)
			modelIdx[pg.ID] = idx
		}
		key := strings.ToLower(model.Platform) + "\x00" + strings.ToLower(model.Name)
		if at, seen := idx[key]; seen {
			// 先见者胜；仅当已存条目无定价而新条目有定价时升级。
			if pg.Models[at].Pricing == nil && model.Pricing != nil {
				pg.Models[at].Pricing = model.Pricing
			}
			return
		}
		idx[key] = len(pg.Models)
		pg.Models = append(pg.Models, model)
	}

	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		ch.normalizeBillingModelSource()
		supported := ch.SupportedModels()
		s.fillGlobalPricingFallback(supported)

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			for j := range supported {
				m := supported[j]
				if !isPlatformPricingMatch(pg.Platform, m.Platform) {
					continue
				}
				addModel(pg, PlazaModel{
					Name:     m.Name,
					Platform: m.Platform,
					Pricing:  m.Pricing,
				})
			}
		}
	}

	if opts.AutoPublicModels {
		for _, gid := range order {
			pg := byGroup[gid]
			if pg.IsExclusive {
				continue
			}
			for _, platform := range plazaAutoPlatforms(pg.Platform) {
				candidates := plazaDefaultModelCandidateIDs(platform)
				if opts.ModelSource != nil {
					candidates = mergePlazaModelIDs(candidates, opts.ModelSource.GetAvailableModels(ctx, &pg.ID, platform))
				}
				for _, modelName := range candidates {
					addModel(pg, PlazaModel{
						Name:     modelName,
						Platform: platform,
						Pricing:  s.plazaFallbackPricing(modelName),
					})
				}
			}
		}
	}

	officialMemo := make(map[string]*PlazaOfficialPricing)
	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool {
			if pg.Models[i].Name != pg.Models[j].Name {
				return pg.Models[i].Name < pg.Models[j].Name
			}
			return pg.Models[i].Platform < pg.Models[j].Platform
		})
		for j := range pg.Models {
			pg.Models[j].OfficialPricing = s.lookupOfficialPricing(pg.Models[j].Name, officialMemo)
		}
		out = append(out, *pg)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RateMultiplier != out[j].RateMultiplier {
			return out[i].RateMultiplier < out[j].RateMultiplier
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// plazaImageDisplayPricing returns a display-only clone for image pricing. Each
// standard size uses the group override first, then the matching channel tier,
// then the channel default per-request price. Non-image pricing and groups
// without image overrides retain the original pointer.
func plazaImageDisplayPricing(p *ChannelModelPricing, g *Group) *ChannelModelPricing {
	if p == nil || g == nil || p.BillingMode != BillingModeImage {
		return p
	}
	if g.ImagePrice1K == nil && g.ImagePrice2K == nil && g.ImagePrice4K == nil {
		return p
	}

	channelTierPrice := func(label string) *float64 {
		for i := range p.Intervals {
			interval := &p.Intervals[i]
			if strings.EqualFold(strings.TrimSpace(interval.TierLabel), label) && interval.PerRequestPrice != nil {
				return interval.PerRequestPrice
			}
		}
		return p.PerRequestPrice
	}
	tiers := []struct {
		label      string
		groupPrice *float64
	}{
		{label: "1K", groupPrice: g.ImagePrice1K},
		{label: "2K", groupPrice: g.ImagePrice2K},
		{label: "4K", groupPrice: g.ImagePrice4K},
	}

	clone := *p
	clone.Intervals = make([]PricingInterval, 0, len(tiers))
	for i, tier := range tiers {
		price := tier.groupPrice
		if price == nil {
			price = channelTierPrice(tier.label)
		}
		if price == nil {
			continue
		}
		value := *price
		clone.Intervals = append(clone.Intervals, PricingInterval{
			TierLabel:       tier.label,
			PerRequestPrice: &value,
			SortOrder:       i,
		})
	}
	return &clone
}

func plazaAutoPlatforms(groupPlatform string) []string {
	switch groupPlatform {
	case PlatformComposite:
		return matchingPlatforms(groupPlatform)
	case PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax:
		return []string{groupPlatform}
	default:
		return nil
	}
}

func plazaDefaultModelCandidateIDs(platform string) []string {
	switch platform {
	case PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax:
		return defaultModelsListCandidateIDs(platform)
	default:
		return nil
	}
}

func mergePlazaModelIDs(primary, secondary []string) []string {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	merged := make([]string, 0, len(primary)+len(secondary))
	for _, models := range [][]string{primary, secondary} {
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			key := strings.ToLower(model)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, model)
		}
	}
	return merged
}

// plazaFallbackPricing converts the global catalog price into the same
// display-only channel shape used by existing plaza rows. It never changes the
// channel cache or the billing path.
func (s *ChannelService) plazaFallbackPricing(modelName string) *ChannelModelPricing {
	if s.pricingService == nil {
		return nil
	}
	return synthesizePricingFromLiteLLM(s.pricingService.GetModelPricing(modelName), nil)
}

// lookupOfficialPricing 查询模型的 LiteLLM 官方参考价，带 memo 避免同名模型重复转换。
// pricingService 为 nil（测试场景）或查不到时返回 nil。
func (s *ChannelService) lookupOfficialPricing(modelName string, memo map[string]*PlazaOfficialPricing) *PlazaOfficialPricing {
	if s.pricingService == nil {
		return nil
	}
	if cached, ok := memo[modelName]; ok {
		return cached
	}
	var result *PlazaOfficialPricing
	if lp := s.pricingService.GetModelPricing(modelName); lp != nil && !lp.TokenPricingAbsent {
		result = &PlazaOfficialPricing{
			InputPrice:        nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:       nonZeroPtr(lp.OutputCostPerToken),
			CacheWritePrice:   nonZeroPtr(lp.CacheCreationInputTokenCost),
			CacheWrite1hPrice: nonZeroPtr(lp.CacheCreationInputTokenCostAbove1hr),
			CacheReadPrice:    nonZeroPtr(lp.CacheReadInputTokenCost),
		}
		if result.InputPrice == nil && result.OutputPrice == nil &&
			result.CacheWritePrice == nil && result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil {
			result = nil
		}
	}
	memo[modelName] = result
	return result
}
