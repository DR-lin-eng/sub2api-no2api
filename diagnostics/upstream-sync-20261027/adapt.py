from pathlib import Path
import json,hashlib
R=Path(__file__).resolve().parents[2];D=Path(__file__).resolve().parent
old={}
def write(p,text):
 f=R/p;old[p]=hashlib.sha256(f.read_bytes()).hexdigest() if f.exists() else None;f.parent.mkdir(parents=True,exist_ok=True);f.write_text(text)
def replace(p,a,b):
 f=R/p;t=f.read_text();assert a in t,(p,a[:80]);write(p,t.replace(a,b))
# Preserve exact Sol/Terra alias efforts, without changing older model policy.
p='backend/internal/application/service/openai_compat_model.go'
replace(p,'"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"','"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"\n "github.com/Wei-Shaw/sub2api/internal/shared/openai"')
replace(p,'\toriginalModel := strings.TrimSpace(req.Model)','''	if base := normalizeKnownOpenAICodexModel(req.Model); base == "gpt-6.1-sol" || base == "gpt-6.1-terra" {
        canonical := openai.CanonicalizeOpenAIModelAliasSpelling(req.Model)
        if effort, ok := strings.CutPrefix(canonical, base+"-"); ok && effort != "openai-compact" {
            req.Model = base
            if req.OutputConfig == nil || strings.TrimSpace(req.OutputConfig.Effort) == "" {
                req.OutputConfig = &apicompat.AnthropicOutputConfig{Effort: effort}
            }
        }
        return
    }
	originalModel := strings.TrimSpace(req.Model)''')
replace(p,'\ttrimmed := strings.TrimSpace(model)\n\tif trimmed == "" {','\tif base := normalizeKnownOpenAICodexModel(model); base == "gpt-6.1-sol" || base == "gpt-6.1-terra" { return base }\n\ttrimmed := strings.TrimSpace(model)\n\tif trimmed == "" {')
# The shared converter must preserve exact modern efforts, including xhigh vs max.
p='backend/internal/shared/apicompat/anthropic_to_responses.go'
replace(p,'\teffort := anthropicReasoningEffort(req)','\teffort := anthropicReasoningEffort(req)\n\tif effort != "none" && req.OutputConfig != nil && isGPT61CompatModel(req.Model) {\n\t switch req.OutputConfig.Effort { case "none", "low", "medium", "high", "xhigh", "max": effort = req.OutputConfig.Effort }\n\t}')
write('backend/internal/shared/apicompat/gpt61.go','''package apicompat
import "github.com/Wei-Shaw/sub2api/internal/shared/openai"
func isGPT61CompatModel(model string) bool {
 m:=openai.CanonicalizeOpenAIModelAliasSpelling(model)
 return m=="gpt-6.1-sol" || m=="gpt-6.1-terra"
}
''')
# Typed capability gate uses the mapped target, all three compatibility entries.
write('backend/internal/application/service/openai_compat_tool_model.go','''package service
func requiresResponsesForToolCalls(model string) bool {
 switch normalizeKnownOpenAICodexModel(model) { case "gpt-6.1-sol", "gpt-6.1-terra": return true }; return false
}
''')
for p,anchor,has,writer in [
 ('backend/internal/application/service/openai_gateway_chat_completions_raw.go','\treasoningEffort := extractOpenAIReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)','len(gjson.GetBytes(body,"tools").Array()) > 0 || len(gjson.GetBytes(body,"functions").Array()) > 0','writeChatCompletionsError'),
 ('backend/internal/application/service/openai_gateway_messages_chat_fallback.go','\tchatReq.Model = upstreamModel','len(anthropicReq.Tools) > 0','writeAnthropicError'),
 ('backend/internal/application/service/openai_gateway_responses_chat_fallback.go','\treasoningEffort := extractOpenAIReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)','len(effectiveTools) > 0','writeOpenAIResponsesFallbackError')]:
 replace(p,anchor,'''    if requiresResponsesForToolCalls(upstreamModel) && ('''+has+''') {
        err := fmt.Errorf("%s requires Responses for tool calls; this account only supports Chat Completions", normalizeKnownOpenAICodexModel(upstreamModel))
        '''+writer+'''(c, http.StatusBadRequest, "invalid_request_error", err.Error())
        return nil, err
    }
'''+anchor)
# Reuse complete, bounded, identity-checked snapshots rather than a second store.
p='backend/internal/modules/modelcatalog/catalog.go'
replace(p,'\tMetadata map[string]Descriptor `json:"metadata,omitempty"`','\tMetadata map[string]Descriptor `json:"metadata,omitempty"`\n\tDiscoveryScope string `json:"-"`')
replace(p,'\tVersion    int','\tDiscoveryScope string `json:"discovery_scope,omitempty"`\n\tVersion    int')
replace(p,'out := &Snapshot{Version: Version,','out := &Snapshot{DiscoveryScope:c.DiscoveryScope, Version: Version,')
# String-only catalog IDs are valid, with no inferred capabilities.
replace(p,'\t\tvar d Descriptor\n\t\tif json.Unmarshal(raw, &d) != nil || d == nil {','''        var scalar string
        if json.Unmarshal(raw, &scalar)==nil {
            if wanted[scalar] { out[scalar]=Descriptor{} }
            continue
        }
		var d Descriptor
		if json.Unmarshal(raw, &d) != nil || d == nil {''')
# Accommodate both data/models catalog arrays without dropping either metadata set.
replace(p,'\tif len(entries) > MaxModels {','''    if _, hasData := envelope["data"]; hasData {
        if more, hasModels := envelope["models"]; hasModels {
            var extra []json.RawMessage
            if json.Unmarshal(more, &extra)!=nil { return nil,fmt.Errorf("invalid model catalog entries") }
            entries=append(entries,extra...)
        }
    }
	if len(entries) > MaxModels {''')
p='backend/internal/application/service/upstream_model_catalog.go'
replace(p,'\tmetadata map[string]modelcatalog.Descriptor','\tmetadata map[string]modelcatalog.Descriptor\n\tscope string')
replace(p,'catalog := &modelcatalog.Catalog{Models: ids, Metadata: capture.metadata}','catalog := &modelcatalog.Catalog{Models: ids, Metadata: capture.metadata, DiscoveryScope:capture.scope}')
# New snapshot appears immediately in diagnostics without another DB query.
replace(p,'\t\tif err := repo.UpdateUpstreamModelCatalog(ctx, account, snapshot); err != nil {\n\t\t\treturn nil, err\n\t\t}', '''\t\tif err := repo.UpdateUpstreamModelCatalog(ctx, account, snapshot); err != nil {
\t\t\treturn nil, err
\t\t}
        if account.IsGrok() {
            extra:=make(map[string]any,len(account.Extra)+1)
            for key,value:=range account.Extra { extra[key]=value }
            extra[modelcatalog.ExtraKey]=snapshot
            account.Extra=extra
        }''')
p='backend/internal/application/service/upstream_models.go'
replace(p,'\tproxyURL := upstreamModelsProxyURL(account)\n\tmodels := []string{}','''    if account.IsGrok() {
        if capture,_:=ctx.Value(modelCatalogCaptureKey{}).(*modelCatalogCapture);capture!=nil { capture.scope=grokModelsScope(account,req.URL.String()) }
    }
\tproxyURL := upstreamModelsProxyURL(account)
\tmodels := []string{}''')
replace(p,'return extractUpstreamModelIDsWithSelector(body, grokUpstreamModelEntryID)','return parseGrokCatalogModelIDs(body)')
write('backend/internal/application/service/grok_model_catalog.go','''package service
import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net/url"
 "strconv"
 "strings"
 "time"
 "github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
)
// Grok parsing rejects ambiguous success/error envelopes and incomplete entries.
func parseGrokCatalogModelIDs(body []byte) ([]string,error) {
 if len(body)>modelcatalog.MaxBodyBytes { return nil,fmt.Errorf("grok catalog exceeds limit") }
 raw:=bytes.TrimSpace(body)
 var entries []json.RawMessage
 if len(raw)==0 { return nil,fmt.Errorf("invalid grok catalog") }
 switch raw[0] {
 case '[': if err:=json.Unmarshal(raw,&entries);err!=nil{return nil,err}
 case '{':
  var envelope map[string]json.RawMessage
  if err:=json.Unmarshal(raw,&envelope);err!=nil{return nil,err}
  if value,ok:=envelope["error"];ok && !bytes.Equal(bytes.TrimSpace(value),[]byte("null")){return nil,fmt.Errorf("grok catalog error response")}
  found:=false
  for _,key:=range []string{"data","models"} { if value,ok:=envelope[key];ok {
   found=true;value=bytes.TrimSpace(value)
   if len(value)==0 || value[0]!='[' {return nil,fmt.Errorf("invalid grok catalog entries")}
   var items []json.RawMessage
   if err:=json.Unmarshal(value,&items);err!=nil{return nil,err};entries=append(entries,items...)
  }}
  if !found {return nil,fmt.Errorf("missing grok catalog")}
 default:return nil,fmt.Errorf("invalid grok catalog")
 }
 if len(entries)==0 || len(entries)>modelcatalog.MaxModels{return nil,fmt.Errorf("invalid grok catalog model count")}
 ids:=make([]string,0,len(entries))
 for _,raw:=range entries {
  var id string
  if json.Unmarshal(raw,&id)!=nil {
   var entry upstreamModelEntry
   if bytes.Equal(bytes.TrimSpace(raw),[]byte("null")){return nil,fmt.Errorf("invalid grok catalog entry")}
   if err:=json.Unmarshal(raw,&entry);err!=nil{return nil,err}
   id=grokUpstreamModelEntryID(entry)
  }
  id=strings.TrimSpace(id)
  if !modelcatalog.ValidID(id){return nil,fmt.Errorf("invalid grok model ID")}
  ids=append(ids,id)
 }
 return dedupeAndSortModelIDs(ids),nil
}
func grokModelsScope(account *Account, endpoint string) string {
 parsed,err:=url.Parse(endpoint)
 if err!=nil || parsed.Host=="" || (parsed.Scheme!="http" && parsed.Scheme!="https") {return ""}
 parsed.Fragment="";parsed.User=nil
 sum:=sha256.Sum256([]byte(strconv.FormatInt(account.ID,10)+"\\n"+account.Type+"\\n"+parsed.String()))
 return hex.EncodeToString(sum[:])
}
// No token refresh, HTTP, DNS lookup, database query, or scheduling mutation.
func GetGrokModelDiagnostics(account *Account) *modelcatalog.GrokDiagnostics {
 if account==nil || !account.IsGrok(){return nil}
 base:=account.GetGrokBaseURL()
 return modelcatalog.EvaluateGrok(modelcatalog.ReadSnapshot(account.Extra),grokModelsScope(account,buildOpenAIModelsURL(base)),account.GetModelMapping(),time.Now())
}
''')
# Keep audit state constant in size; reuse parsed events instead of reparsing JSON.
for p in ['backend/internal/application/service/openai_gateway_forward.go','backend/internal/application/service/openai_gateway_messages.go','backend/internal/application/service/openai_gateway_chat_completions.go']:
 t=(R/p).read_text();needle='\tbeginUpstreamResponseModelObservation(c)'
 assert needle in t,p
 write(p,t.replace(needle,needle+'\n\tif c != nil { c.Set("grok_buffered_model_audit", account != nil && account.IsGrok()) }',1))
p='backend/internal/application/service/openai_gateway_messages.go'
t=(R/p).read_text();start=t.index('func (s *OpenAIGatewayService) readOpenAICompatBufferedTerminal(');end=t.index('\nfunc ',start+10);section=t[start:end]
section=section.replace('\tacc := apicompat.NewBufferedResponseAccumulator()','''    var observer *upstreamResponseModelObserver
    if timingContext!=nil && timingContext.GetBool("grok_buffered_model_audit") { observer=upstreamResponseModelObserverFromContext(timingContext) }
\tacc := apicompat.NewBufferedResponseAccumulator()''')
section=section.replace('acc.ProcessEvent(&event)','acc.ProcessEvent(&event)\n if observer!=nil && event.Response!=nil { observer.Observe(event.Response.Model,isOpenAICompatResponsesTerminalEvent(event.Type)) }')
write(p,t[:start]+section+t[end:])
p='backend/internal/application/service/openai_gateway_grok.go'
replace(p,'\t\tUpstreamModel:   upstreamModel,','\t\tUpstreamModel:   upstreamModel,\n\t\tUpstreamResponseModel: observedUpstreamResponseModel(c),\n\t\tUpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),')
p='backend/internal/application/service/upstream_response_model.go'
replace(p,'\trunes := []rune(model)\n\tif len(runes) > upstreamResponseModelMaxLength {\n\t\tmodel = string(runes[:upstreamResponseModelMaxLength])\n\t}', '''    count:=0
    for offset:=range model {
        count++
        if count>upstreamResponseModelMaxLength { return strings.Clone(model[:offset]) }
    }''')
# Account detail and sync only. Common/list/user DTOs do not compute diagnostics.
p='backend/internal/transport/http/handler/dto/types.go'
replace(p,'\tOllamaCloudUsage        *service.OllamaCloudUsageState', '\tGrokModelDiagnostics *modelcatalog.GrokDiagnostics `json:"grok_model_diagnostics,omitempty"`\n\tOllamaCloudUsage        *service.OllamaCloudUsageState')
t=(R/p).read_text();assert '"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"' not in t
replace(p,'import (','import (\n "github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"')
p='backend/internal/transport/http/handler/admin/account_handler.go'
needle='\tresponse.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))'
t=(R/p).read_text();start=t.index('func (h *AccountHandler) GetByID(');pos=t.index(needle,start)
write(p,t[:pos]+'''    result:=h.buildAccountResponseWithRuntime(c.Request.Context(),account)
    result.GrokModelDiagnostics=service.GetGrokModelDiagnostics(account)
    response.Success(c,result)'''+t[pos+len(needle):])
t=(R/p).read_text();start=t.index('func (h *AccountHandler) SyncUpstreamModels(');pos=t.index('\tresponse.Success(c, catalog)',start)
write(p,t[:pos]+'''    response.Success(c,struct {
        *modelcatalog.Catalog
        Diagnostics *modelcatalog.GrokDiagnostics `json:"grok_model_diagnostics,omitempty"`
    }{catalog,service.GetGrokModelDiagnostics(account)})'''+t[pos+len('\tresponse.Success(c, catalog)'):])
replace(p,'import (','import (\n "github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"')
(D/'PREWRITE_HASHES.json').write_text(json.dumps(old,indent=2)+'\n')
print('ADAPTED '+str(len(old))+' source paths')
