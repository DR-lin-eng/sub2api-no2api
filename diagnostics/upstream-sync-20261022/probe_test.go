//go:build unit

package service

import (
 "fmt"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "github.com/gin-gonic/gin"
 "github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
)

type sync20261022FailWriter struct { gin.ResponseWriter }
func (w sync20261022FailWriter) Write(p []byte) (int,error) { return 0, io.ErrClosedPipe }
func (w sync20261022FailWriter) WriteString(p string) (int,error) { return 0, io.ErrClosedPipe }

func TestUpstreamSync20261022Probe(t *testing.T) {
 fmt.Printf("METADATA queued=%t metadata=%t\n", openAIStreamEventIsPreambleTrimmed("response.queued"), openAIStreamEventIsPreambleTrimmed("response.metadata"))
 part:=map[string]any{"type":"input_image","image_url":"https://fixture.invalid/a.png","url":"https://fixture.invalid/stale.png"}
 aliasDeepSeekResponsesImagePart(part)
 b,_:=json.Marshal(part); fmt.Printf("DEEPSEEK %s\n",b)
 schema:=map[string]any{"type":"object","properties":map[string]any{"type":map[string]any{"type":"string","const":"fixed"}},"default":map[string]any{"type":"literal","const":"kept"}}
 b,_=json.Marshal(cleanToolSchema(schema)); fmt.Printf("SCHEMA %s\n",b)
 a:=&Account{Type:AccountTypeServiceAccount,Status:"active",Schedulable:true,Extra:map[string]any{"quota_limit":1.0,"quota_used":2.0}}
 fmt.Printf("VERTEX_QUOTA schedulable=%t\n",a.IsSchedulable())
 stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"fixture\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"fixture\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
 for _,route:=range []string{"chat","responses"} {
  c,_:=gin.CreateTestContext(httptest.NewRecorder()); c.Request=httptest.NewRequest("POST","/",nil); c.Writer=sync20261022FailWriter{c.Writer}
  resp:=&http.Response{StatusCode:200,Header:http.Header{},Body:io.NopCloser(strings.NewReader(stream))}
  s:=&GatewayService{}
  var result *ForwardResult; var err error
  if route=="chat" { result,err=s.handleCCStreamingFromAnthropic(resp,c,"claude-sonnet-4-5","claude-sonnet-4-5",nil,time.Now(),true) } else { result,err=s.handleResponsesStreamingResponse(resp,c,"claude-sonnet-4-5","claude-sonnet-4-5",nil,time.Now(),apicompat.ResponsesClientToolMapping{}) }
  if result==nil { fmt.Printf("ABORT %s result=nil err=%v\n",route,err) } else { fmt.Printf("ABORT %s input=%d output=%d client_disconnect=%t err=%v\n",route,result.Usage.InputTokens,result.Usage.OutputTokens,result.ClientDisconnect,err) }
 }
}
