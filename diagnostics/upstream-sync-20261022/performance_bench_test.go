//go:build unit

package service

import (
 "fmt"
 "context"
 "github.com/Wei-Shaw/sub2api/internal/platform/config"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "github.com/gin-gonic/gin"
 "github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
)
var syncMappingSink uint64
var syncSchemaSink any

type syncDiscardWriter struct { header http.Header }
func (w *syncDiscardWriter) Header() http.Header { return w.header }
func (w *syncDiscardWriter) Write(p []byte)(int,error){return len(p),nil}
func (w *syncDiscardWriter) WriteString(p string)(int,error){return len(p),nil}
func (w *syncDiscardWriter) WriteHeader(int){}
func (w *syncDiscardWriter) Flush(){}

func BenchmarkUpstreamSync20261022(b *testing.B) {
 gin.SetMode(gin.TestMode)
 for _,size:=range []int{8,128,1024} {
  m:=make(map[string]any,size)
  for i:=0;i<size;i++ {m[fmt.Sprintf("client-%04d",i)]=fmt.Sprintf("upstream-%04d",i)}
  b.Run(fmt.Sprintf("Mapping%d",size),func(b *testing.B){b.ReportAllocs();for i:=0;i<b.N;i++{syncMappingSink=modelMappingSignature(m)}})
 }
 schema:=map[string]any{"type":"object","properties":map[string]any{"type":map[string]any{"type":"string","const":"fixed","enum":[]any{"fixed","other"}},"value":map[string]any{"type":"integer","exclusiveMinimum":2.0}},"default":map[string]any{"type":"literal","const":"preserved"}}
 b.Run("Schema",func(b *testing.B){b.ReportAllocs();for i:=0;i<b.N;i++{syncSchemaSink=cleanToolSchema(schema)}})
 var stream strings.Builder
 stream.WriteString("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"fixture\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
 for i:=0;i<32;i++{stream.WriteString("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"fixture\"}}\n\n")}
 stream.WriteString("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":32}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
 text:=stream.String()
 for _,route:=range []string{"Chat","Responses","ChatTimeout","ResponsesTimeout"}{ b.Run(route,func(b *testing.B){
  b.ReportAllocs();s:=&GatewayService{}
  if strings.HasSuffix(route,"Timeout") { s.cfg=&config.Config{Gateway:config.GatewayConfig{StreamDataIntervalTimeout:180}} }
  for i:=0;i<b.N;i++{
   w:=&syncDiscardWriter{header:make(http.Header)};c,_:=gin.CreateTestContext(w);c.Request=httptest.NewRequest("POST","/",nil)
   cancel:=func(){}
   if strings.HasSuffix(route,"Timeout") { ctx,done:=context.WithCancel(context.Background());cancel=done;c.Request=c.Request.WithContext(ctx) }
   resp:=&http.Response{StatusCode:200,Header:http.Header{},Body:io.NopCloser(strings.NewReader(text))}
   var result *ForwardResult;var err error
   if strings.HasPrefix(route,"Chat") {result,err=s.handleCCStreamingFromAnthropic(resp,c,"claude-sonnet-4-5","claude-sonnet-4-5",nil,time.Now(),true)}else{result,err=s.handleResponsesStreamingResponse(resp,c,"claude-sonnet-4-5","claude-sonnet-4-5",nil,time.Now(),apicompat.ResponsesClientToolMapping{})}
   cancel()
   if err!=nil||result==nil||result.Usage.OutputTokens!=32{b.Fatalf("invalid fixture result: %v %v",result,err)}
  }
 })}
}
