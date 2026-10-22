"""Offline Anthropic/Gemini HTTP fixtures; no external provider credentials."""
import json,time
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*_):pass
 def send_event(self,kind,payload):
  self.wfile.write(('event: '+kind+'\ndata: '+json.dumps(payload)+'\n\n').encode());self.wfile.flush()
 def do_POST(self):
  raw=self.rfile.read(int(self.headers.get('Content-Length',0)))
  if self.path.startswith('/v1/messages'):
   self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
   try:
    self.send_event('message_start',{'type':'message_start','message':{'id':'fixture-msg','type':'message','role':'assistant','model':'claude-sonnet-4-5','content':[],'usage':{'input_tokens':10,'output_tokens':0}}})
    self.send_event('content_block_start',{'type':'content_block_start','index':0,'content_block':{'type':'text','text':''}})
    self.send_event('content_block_delta',{'type':'content_block_delta','index':0,'delta':{'type':'text_delta','text':'fixture-end'}})
    if b'runtime-buffered' not in raw:
     time.sleep(.5)
     for _ in range(20):
      self.send_event('content_block_delta',{'type':'content_block_delta','index':0,'delta':{'type':'text_delta','text':'after-abort'}});time.sleep(.025)
    self.send_event('content_block_stop',{'type':'content_block_stop','index':0})
    self.send_event('message_delta',{'type':'message_delta','delta':{'stop_reason':'end_turn'},'usage':{'output_tokens':50}})
    self.send_event('message_stop',{'type':'message_stop'})
   except (BrokenPipeError,ConnectionResetError):pass
   return
  self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers()
  self.wfile.write(json.dumps({'candidates':[{'content':{'role':'model','parts':[{'text':'fixture-end'}]},'finishReason':'STOP'}],'usageMetadata':{'promptTokenCount':10,'candidatesTokenCount':5,'totalTokenCount':15}}).encode())
ThreadingHTTPServer(('127.0.0.1',8000),Handler).serve_forever()
