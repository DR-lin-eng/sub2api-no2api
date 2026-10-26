"""Offline image/Responses provider; no external model calls."""
import json,itertools
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
IMAGE='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aRZcAAAAASUVORK5CYII='
SEQ=itertools.count(1)
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*_):pass
 def event(self,value):self.wfile.write(('data: '+json.dumps(value)+'\n\n').encode());self.wfile.flush()
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
  rid='fixture-'+str(next(SEQ));usage={'input_tokens':1000,'output_tokens':100,'total_tokens':1100}
  images=[{'b64_json':IMAGE},{'b64_json':IMAGE}]
  response={'id':rid,'object':'response','created_at':1800000000,'status':'completed','model':body.get('model'),'output':[{'type':'image_generation_call','id':rid+'-image-'+str(i),'status':'completed','result':IMAGE} for i in range(2)]+[{'type':'message','id':rid+'-message','status':'completed','role':'assistant','content':[{'type':'output_text','text':'fixture-text','annotations':[]}]}],'usage':usage}
  self.send_response(200);self.send_header('x-request-id',rid)
  if body.get('stream'):
   self.send_header('Content-Type','text/event-stream');self.end_headers()
   if '/images/' in self.path:
    for i in range(2):self.event({'type':'image_generation.completed','id':rid+'-'+str(i),'index':i,'b64_json':IMAGE,'usage':usage})
   else:
    self.event({'type':'response.output_text.delta','delta':'fixture-text'});self.event({'type':'response.completed','response':response})
   self.wfile.write(b'data: [DONE]\n\n');return
  self.send_header('Content-Type','application/json');self.end_headers()
  self.wfile.write(json.dumps({'created':1800000000,'data':images,'usage':usage} if '/images/' in self.path else response).encode())
ThreadingHTTPServer(('127.0.0.1',8000),Handler).serve_forever()
