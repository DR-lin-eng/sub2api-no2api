//go:build unit
package service
import("context";"encoding/json";"fmt";"net/http";"net/http/httptest";"testing";"github.com/Wei-Shaw/sub2api/internal/platform/config";"github.com/Wei-Shaw/sub2api/internal/domain";"github.com/gin-gonic/gin";"github.com/tidwall/gjson")
func TestUpstreamSyncBehavior(t *testing.T){
 gin.SetMode(gin.TestMode);c,_:=gin.CreateTestContext(httptest.NewRecorder());c.Request=httptest.NewRequest(http.MethodPost,"/v1/messages",nil)
 s:=&GatewayService{cfg:&config.Config{}}
 body:=[]byte(`{"model":"deepseek-chat","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
 account:=&Account{ID:8074,Platform:PlatformDeepseek,Type:AccountTypeAPIKey,Credentials:map[string]any{"api_key":"fixture-key","base_url":"https://api.deepseek.com","api_protocol":APIProtocolAdaptive,"api_base_urls":map[string]any{APIProtocolAnthropic:"https://anthropic.example.com"}}}
 req,_,err:=s.buildUpstreamRequest(context.Background(),c,account,body,"fixture-key","apikey","deepseek-chat",false,false);if err!=nil{t.Fatal(err)}
 foreign:=&Account{ID:8106,Platform:PlatformOpenAI,Type:AccountTypeOAuth,Credentials:map[string]any{}}
 _,_,foreignErr:=s.buildUpstreamRequest(context.Background(),c,foreign,body,"fixture-token","oauth","gpt-6",false,false)
 tiers:=parseZhipuTokenTiers(gjson.Parse(`{"limits":[{"type":"CREDIT_LIMIT","unit":6,"percentage":90,"nextResetTime":2000},{"type":"CREDIT_LIMIT","unit":3,"percentage":10,"nextResetTime":3000}]}`))
 billing:=NewBillingService(&config.Config{},nil);price:=billing.getFallbackPricing("gemini-4-argon-high");inputPrice:=0.0;if price!=nil{inputPrice=price.InputPricePerToken}
 result:=map[string]any{"anthropic_url":req.URL.String(),"foreign_oauth_rejected":foreignErr!=nil,"zhipu_tiers":tiers,"argon_input_price":inputPrice,"argon_default_mapping":domain.DefaultAntigravityModelMapping["gemini-4-argon"]}
 output,_:=json.Marshal(result);fmt.Println("OBSERVED_SERVICE",string(output))
}
