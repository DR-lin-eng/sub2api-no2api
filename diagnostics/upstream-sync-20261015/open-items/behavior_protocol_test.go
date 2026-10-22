package apicompat
import("encoding/json";"fmt";"testing";"github.com/Wei-Shaw/sub2api/internal/shared/claude")
func TestOpenItemsBehavior(t *testing.T){
 response:=AnthropicToResponsesResponse(&AnthropicResponse{Model:"claude-opus-5-5",Content:[]AnthropicContentBlock{{Type:"thinking",Thinking:"retained history",Signature:"fixture-anthropic-signature"},{Type:"text",Text:"answer"}}})
 encrypted:=false;for _,o:=range response.Output{encrypted=encrypted||o.EncryptedContent!=""}
 input,_:=json.Marshal(response.Output);request,err:=ResponsesToAnthropicRequest(&ResponsesRequest{Model:"claude-opus-5-5",Input:input,Reasoning:&ResponsesReasoning{Effort:"high"}});if err!=nil{t.Fatal(err)}
 signature:=false;for _,m:=range request.Messages{var blocks []AnthropicContentBlock;_ =json.Unmarshal(m.Content,&blocks);for _,b:=range blocks{signature=signature||b.Signature=="fixture-anthropic-signature"}}
 names:=map[string]bool{};for _,m:=range claude.DefaultModels{names[m.ID]=true}
 mode:="";if request.Thinking!=nil{mode=request.Thinking.Type}
 fmt.Printf("OPEN_PROTOCOL encrypted=%t replay_signature=%t thinking=%s opus55=%t sonnet55=%t haiku55=%t\n",encrypted,signature,mode,names["claude-opus-5-5"],names["claude-sonnet-5-5"],names["claude-haiku-5-5"])
}
