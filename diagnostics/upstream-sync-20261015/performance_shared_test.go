//go:build unit
package antigravity
import("encoding/json";"fmt";"strings";"testing")
func BenchmarkUpstream20261015ToolResult(b *testing.B){
 for _,kind:=range []string{"text","image"}{for _,size:=range []int{1024,1048576}{b.Run(fmt.Sprintf("%s/%d",kind,size),func(b *testing.B){
  result:=[]map[string]any{{"type":"text","text":strings.Repeat("x",size)}}
  if kind=="image"{result=[]map[string]any{{"type":"image","source":map[string]any{"type":"base64","media_type":"image/png","data":strings.Repeat("a",size)}}}}
  content,_:=json.Marshal([]map[string]any{{"type":"tool_result","tool_use_id":"tool","content":result}})
  names:=map[string]string{"tool":"inspect"};b.ReportAllocs();b.ResetTimer()
  for range b.N{_,_,err:=buildParts(content,names,true);if err!=nil{b.Fatal(err)}}
 })}}
}
func TestUpstreamMediaBehavior(t *testing.T){
 content:=json.RawMessage(`[{"type":"tool_result","tool_use_id":"tool","content":[{"type":"text","text":"loaded"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]}]`)
 parts,_,err:=buildParts(content,map[string]string{"tool":"inspect"},true);if err!=nil{t.Fatal(err)}
 images:=0;for _,part:=range parts{if part.InlineData!=nil{images++}}
 fmt.Printf("OBSERVED_MEDIA inline_images=%d function_text=%s\n",images,parts[0].FunctionResponse.Response["result"])
}
