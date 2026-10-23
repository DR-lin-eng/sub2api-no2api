package service

import (
 "fmt"
 "net/http"
 "reflect"
 "testing"
 "time"
)

func probeRetryDelay(e *UpstreamFailoverError) time.Duration {
 f:=reflect.ValueOf(e).Elem().FieldByName("SameAccountRetryDelay")
 if !f.IsValid() { return 0 }
 return time.Duration(f.Int())
}

func TestUpstreamSync20261024Probe(t *testing.T) {
 body:=[]byte(`{"error":{"code":"server_is_overloaded","headers":{"retry-after-ms":"7500"}}}`)
 fmt.Printf("HTTP_CAPACITY_DELAY=%s\n",probeRetryDelay(newOpenAIUpstreamFailoverError(503,http.Header{"Retry-After":[]string{"6.25"}},body,"",false)))
 fmt.Printf("BODY_CAPACITY_DELAY=%s\n",probeRetryDelay(newOpenAIUpstreamFailoverError(503,nil,body,"",false)))
 fmt.Printf("WS_STATUS_DELAY=%s\n",probeRetryDelay(newOpenAIWSOverloadFailoverError(http.Header{"Retry-After":[]string{"6"}},[]byte(`{"type":"error","status":503}`),"")))
 s:=&OpenAIGatewayService{}
 streamBody:=[]byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","headers":{"retry-after":"7"}}}}`)
 fmt.Printf("SSE_CAPACITY_DELAY=%s\n",probeRetryDelay(s.newOpenAIStreamFailoverError(nil,nil,false,"fixture",streamBody,"overloaded")))
 fmt.Printf("ORDINARY_ERROR_DELAY=%s\n",probeRetryDelay(newOpenAIUpstreamFailoverError(400,http.Header{"Retry-After":[]string{"6"}},[]byte(`{"error":{"code":"invalid_request"}}`),"",false)))
 for _,kind:=range []string{"response.output_text.delta","response.output_text.done","response.content_part.done","response.reasoning_summary_part.done"} {
  payload:=`{"type":"`+kind+`","delta":"","text":"","part":{"type":"output_text","text":""}}`
  fmt.Printf("EMPTY_%s_REPLAY_UNSAFE=%t\n",kind,openAIStreamDataStartsClientOutput(payload,kind))
 }
 desktop:="Codex Desktop/0.153.4 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.903.71938)"
 fmt.Printf("DESKTOP_IDENTITY_PRESERVED=%t\n",resolveCodexOutboundIdentity(desktop).userAgent==desktop)
 fmt.Printf("TUI_IDENTITY_PRESERVED=%t\n",resolveCodexOutboundIdentity("codex-tui/0.153.4 (Linux 6.8; x86_64) xterm").originator=="codex-tui")
}

func BenchmarkUpstreamSync20261024(b *testing.B) {
 for _,tc:=range []struct{name,body string; capacity bool}{
  {"ordinary",`{"error":{"code":"invalid_request","message":"invalid input"}}`,false},
  {"capacity_no_hint",`{"error":{"code":"server_is_overloaded"}}`,true},
  {"capacity_header",`{"error":{"code":"server_is_overloaded"}}`,true},
 } {
  b.Run(tc.name,func(b *testing.B){
   body:=[]byte(tc.body); var headers http.Header
   if tc.name=="capacity_header" { headers=http.Header{"Retry-After":[]string{"6.25"}} }
   b.ReportAllocs(); b.ResetTimer()
   for i:=0;i<b.N;i++ { e:=newOpenAIUpstreamFailoverError(503,headers,body,"",false); if e.RetryableOnSameAccount!=tc.capacity { b.Fatal("scope changed") } }
  })
 }
 b.Run("text_delta",func(b *testing.B){for i:=0;i<b.N;i++{if !openAIStreamDataStartsClientOutput(`{"type":"response.output_text.delta","delta":"hello"}`,"response.output_text.delta"){b.Fatal("missing output")}}})
}
