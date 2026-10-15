//go:build unit
package service
import("fmt";"testing";"github.com/tidwall/gjson")
func TestOpenItemsBehavior(t *testing.T){
 explicit:=convertOpenAIModelListToCodexManifest([]byte(`{"data":[{"id":"gpt-6.1-sol","context_window":777777,"max_output_tokens":32768,"supports_reasoning":true,"supported_reasoning_levels":["low","high"]}]}`))
 fallback:=convertOpenAIModelListToCodexManifest([]byte(`{"data":[{"id":"gpt-6.1-sol"}]}`))
 fmt.Printf("OPEN_METADATA explicit_context=%d fallback_context=%d max_output=%d\n",gjson.GetBytes(explicit,"models.0.context_window").Int(),gjson.GetBytes(fallback,"models.0.context_window").Int(),gjson.GetBytes(explicit,"models.0.max_output_tokens").Int())
}
