"""Offline Responses provider with per-case timestamps and bounded retry fixtures."""
import json,re,time
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from threading import Lock
CASES={};LOCK=Lock()
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*_):pass
 def write_json(self,code,body,hint=False):
  raw=json.dumps(body).encode();self.send_response(code);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)))
  if hint:self.send_header('Retry-After','1.25')
  self.end_headers();self.wfile.write(raw)
 def event(self,body):
  self.wfile.write(('data: '+json.dumps(body)+'\n\n').encode());self.wfile.flush()
 def do_GET(self):
  with LOCK:body={k:list(v) for k,v in CASES.items()}
  self.write_json(200,body)
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers.get('Content-Length','0'))))
  match=re.search(r'fixture-[A-Z]+-[a-z]+-[a-z]+',json.dumps(body));case=match.group() if match else 'unknown'
  mode=case.split('-')[2] if match else 'http'
  with LOCK:
   times=CASES.setdefault(case,[]);times.append(time.monotonic());attempt=len(times)
  error={'code':'server_is_overloaded','message':'Our servers are currently overloaded. Please try again later.','headers':{'retry-after-ms':'1250'}}
  if mode=='invalid':return self.write_json(400,{'error':{'type':'invalid_request_error','code':'invalid_request','message':'fixture invalid input'}},True)
  if mode=='http' and attempt==1:return self.write_json(503,{'error':error},True)
  response={'id':'resp_fixture','object':'response','created_at':1800000000,'status':'completed','model':'gpt-4.1','output':[{'id':'msg_fixture','type':'message','status':'completed','role':'assistant','content':[{'type':'output_text','text':'fixture-end','annotations':[]}]}],'usage':{'input_tokens':10,'output_tokens':5,'total_tokens':15}}
  stream_failure=mode in ['sse','visible'] and attempt==1
  if stream_failure or body.get('stream'):
   self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
   self.event({'type':'response.created','response':{'id':'resp_fixture','status':'in_progress'}})
   if stream_failure:
    if mode=='visible':self.event({'type':'response.output_text.delta','delta':'visible-once'})
    else:
     for kind in ['response.output_text.delta','response.output_text.done','response.content_part.done','response.reasoning_summary_part.done']:
      self.event({'type':kind,'delta':'','text':'','part':{'type':'summary_text' if 'summary' in kind else 'output_text','text':''}})
    self.event({'type':'response.failed','response':{'id':'resp_fixture','status':'failed','error':error}})
   else:
    self.event({'type':'response.output_text.delta','delta':'fixture-end'})
    self.event({'type':'response.completed','response':response})
   return
  self.write_json(200,response)
ThreadingHTTPServer(('127.0.0.1',8000),Handler).serve_forever()
