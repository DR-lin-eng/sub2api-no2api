//go:build unit

package service

import (
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "github.com/Wei-Shaw/sub2api/internal/platform/config"
)

func TestPR8170Probe(t *testing.T) {
 data,err:=os.ReadFile(filepath.Join("..","..","..","resources","model-pricing","model_prices_and_context_window.json"));if err!=nil{t.Fatal(err)}
 ps:=&PricingService{};parsed,err:=ps.parsePricingData(data);if err!=nil{t.Fatal(err)};ps.pricingData=parsed
 for _,source:=range []string{"fallback","bundled"}{
  svc:=NewBillingService(&config.Config{},nil);if source=="bundled"{svc=NewBillingService(&config.Config{},ps)}
  for _,model:=range []string{"grok-4.7-fast","grok-4.7-build-fast","xai/grok-4.7-fast","x-ai/grok-4.7-build-fast","GROK-4.7-FAST","grok-4.6"}{
   for _,input:=range []int{100000,149999,150000,150001}{
    cost,err:=svc.CalculateCost(model,UsageTokens{InputTokens:input,CacheReadTokens:50000,OutputTokens:10000},1);if err!=nil{t.Fatal(err)}
    fmt.Printf("PR8170 source=%s model=%s prompt=%d input=%.6f cache=%.6f output=%.6f total=%.6f long=%t\n",source,model,input+50000,cost.InputCost,cost.CacheReadCost,cost.OutputCost,cost.TotalCost,cost.LongContextBillingApplied)
   }
  }
 }
}
var pr8170CostSink *CostBreakdown
func BenchmarkPR8170Pricing(b *testing.B){
 svc:=NewBillingService(&config.Config{},nil)
 for _,model:=range []string{"grok-4.6","grok-4.7-fast"}{
  _,_ = svc.CalculateCost(model,UsageTokens{InputTokens:150000,CacheReadTokens:50000,OutputTokens:10000},1)
  b.Run(model,func(b *testing.B){b.ReportAllocs();for i:=0;i<b.N;i++{var err error;pr8170CostSink,err=svc.CalculateCost(model,UsageTokens{InputTokens:150000,CacheReadTokens:50000,OutputTokens:10000},1);if err!=nil{b.Fatal(err)}}})
 }
}
