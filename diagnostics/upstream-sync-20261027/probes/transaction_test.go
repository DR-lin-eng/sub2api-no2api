//go:build unit
package service
import (
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
 "github.com/gin-gonic/gin"
 "github.com/stretchr/testify/require"
)
func TestUpstreamSync20261027Transaction(t *testing.T) {
 req := &apicompat.AnthropicRequest{Model:"gpt-6.1-terra-max"}
 applyOpenAICompatModelNormalization(req)
 effort := ""; if req.OutputConfig != nil { effort=req.OutputConfig.Effort }
 t.Logf("TERRA model=%s effort=%s",req.Model,effort)
 ids,err := extractGrokUpstreamModelIDs([]byte(`{"error":{"message":"fixture failure"},"data":[{"id":"grok-4.6"}]}`))
 t.Logf("GROK invalid_catalog_rejected=%t models=%v",err!=nil,ids)
 gin.SetMode(gin.TestMode)
 c,_:=gin.CreateTestContext(httptest.NewRecorder());beginUpstreamResponseModelObservation(c);c.Set("grok_buffered_model_audit",true)
 resp:= &http.Response{Header:http.Header{},Body:io.NopCloser(strings.NewReader("data: {\"type\":\"response.created\",\"response\":{\"model\":\"grok-4.5\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"grok-4.6\",\"status\":\"completed\"}}\n\n"))}
 final,_,_,err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(resp,"transaction","fixture",c)
 require.NoError(t,err);require.NotNil(t,final)
 observer:=upstreamResponseModelObserverFromContext(c);observer.Observe(final.Model,true)
 t.Logf("GROK terminal=%s conflict=%t",observer.Model(),observer.Conflict())
}
