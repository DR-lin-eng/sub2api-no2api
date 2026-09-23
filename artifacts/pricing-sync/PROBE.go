// Run via probe-overlay.json inside the backend module; no application source is rewritten.
package main

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

type remoteCatalog []byte

func (r remoteCatalog) FetchPricingJSON(context.Context, string) ([]byte, error) { return r, nil }
func (r remoteCatalog) FetchHashText(context.Context, string) (string, error)    { return "", nil }

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	phase := os.Args[1]
	body, err := os.ReadFile("/evidence/REMOTE_CATALOG.json")
	must(err)
	dir, err := os.MkdirTemp("", "pricing-probe-")
	must(err)
	defer os.RemoveAll(dir)
	cfg := &config.Config{Pricing: config.PricingConfig{
		RemoteURL: "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json",
		DataDir:   dir, UpdateIntervalHours: 24,
		FallbackFile: "/workspace/backend/resources/model-pricing/model_prices_and_context_window.json",
	}}
	pricing := service.NewPricingService(cfg, remoteCatalog(body))
	must(pricing.Initialize())
	defer pricing.Stop()
	billing := service.NewBillingService(cfg, pricing)
	p, err := billing.GetModelPricing("gpt-5.6-sol")
	must(err)
	wantInput, wantTotal := 5e-6, 0.0063
	if phase == "MODIFIED" {
		wantInput, wantTotal = 4e-6, 0.00484
	}
	if math.Abs(p.InputPricePerToken-wantInput) > 1e-12 {
		panic("unexpected input price")
	}
	fmt.Printf("SOL_RATES_USD_PER_MTOK=%g/%g/%g/%g\n", p.InputPricePerToken*1e6, p.CacheReadPricePerToken*1e6, p.CacheCreationPricePerToken*1e6, p.OutputPricePerToken*1e6)
	tokens := service.UsageTokens{InputTokens: 700, CacheReadTokens: 100, CacheCreationTokens: 200, OutputTokens: 50}
	for _, tier := range []struct {
		name  string
		scale float64
	}{{"standard", 1}, {"fast", 2}, {"flex", 0.5}} {
		cost, err := billing.CalculateCostWithServiceTier("gpt-5.6-sol", tokens, 1, tier.name)
		must(err)
		if math.Abs(cost.TotalCost-wantTotal*tier.scale) > 1e-12 {
			panic("unexpected total for " + tier.name)
		}
		fmt.Printf("%s_TOTAL_USD=%.6f\n", tier.name, cost.TotalCost)
	}
	// Reopen the downloaded raw catalog through the real startup path.
	restarted := service.NewPricingService(cfg, remoteCatalog(body))
	must(restarted.Initialize())
	defer restarted.Stop()
	reloaded, err := service.NewBillingService(cfg, restarted).GetModelPricing("gpt-5.6-sol")
	must(err)
	if reloaded.InputPricePerToken != p.InputPricePerToken || reloaded.OutputPricePerToken != p.OutputPricePerToken {
		panic("restart changed the price")
	}
	fmt.Println("RESTART_PRICE_STABLE=PASS")
	fmt.Println(phase + "_PROBE=PASS")
}
