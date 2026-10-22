"""Deterministic local xAI Chat completions fixtures with cumulative cache usage."""
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
import json,uuid
class Handler(BaseHTTPRequestHandler):
    def log_message(self,*_):pass
    def do_POST(self):
        raw=self.rfile.read(int(self.headers.get('Content-Length',0)))
        body=json.loads(raw);model=body.get('model','grok-4.7-fast');stream=body.get('stream',False)
        total_input=200000 if b'runtime-long' in raw else 150000
        usage={'prompt_tokens':total_input,'completion_tokens':10000,'total_tokens':total_input+10000,'prompt_tokens_details':{'cached_tokens':50000}}
        identity='chatcmpl-fixture-'+uuid.uuid4().hex
        self.send_response(200);self.send_header('Content-Type','text/event-stream' if stream else 'application/json');self.end_headers()
        if stream:
            for chunk in [{'id':identity,'object':'chat.completion.chunk','created':1,'model':model,'choices':[{'index':0,'delta':{'role':'assistant','content':'fixture-ok'},'finish_reason':None}]},{'id':identity,'object':'chat.completion.chunk','created':1,'model':model,'choices':[{'index':0,'delta':{},'finish_reason':'stop'}],'usage':usage}]:
                self.wfile.write(('data: '+json.dumps(chunk)+'\n\n').encode());self.wfile.flush()
            self.wfile.write(b'data: [DONE]\n\n');self.wfile.flush()
        else:
            self.wfile.write(json.dumps({'id':identity,'object':'chat.completion','created':1,'model':model,'choices':[{'index':0,'message':{'role':'assistant','content':'fixture-ok'},'finish_reason':'stop'}],'usage':usage}).encode())
ThreadingHTTPServer(('127.0.0.1',8000),Handler).serve_forever()
