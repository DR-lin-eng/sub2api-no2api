from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
import json
mode='valid';last={}
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def send(self,value,code=200):
  raw=json.dumps(value).encode();self.send_response(code);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
 def do_GET(self):
  if self.path=='/fixture/last':return self.send(last)
  body={'data':[{'id':'grok-4.6','context_window':12345}]}
  if mode=='invalid':body['error']={'message':'fixture unavailable'}
  return self.send(body)
 def do_POST(self):
  global mode,last
  body=json.loads(self.rfile.read(int(self.headers.get('Content-Length',0))) or '{}')
  if self.path=='/fixture/mode':mode=body['mode'];return self.send({'mode':mode})
  last=body
  model='grok-4.6' if 'grok' in body.get('model','') else 'gpt-6.1-terra'
  response={'id':'resp_fixture','object':'response','model':model,'status':'completed','created_at':1793059200,'output':[{'type':'message','id':'m_fixture','role':'assistant','status':'completed','content':[{'type':'output_text','text':'ok'}]}],'usage':{'input_tokens':1000,'output_tokens':100,'total_tokens':1100}}
  if not body.get('stream'):return self.send(response)
  self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
  prior='grok-4.5' if 'grok' in model else model
  for event in [{'type':'response.created','response':{'id':'resp_fixture','model':prior,'status':'in_progress'}},{'type':'response.output_text.delta','delta':'ok'},{'type':'response.completed','response':response}]:
   self.wfile.write(('data: '+json.dumps(event)+'\n\n').encode());self.wfile.flush()
ThreadingHTTPServer(('0.0.0.0',8000),Handler).serve_forever()
