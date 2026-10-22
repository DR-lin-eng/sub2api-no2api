"""Offline HTTP fixtures: no live provider credentials or external requests."""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))))
        stream = request.get('stream', False)
        response = {'id':'resp_sync_fixture','object':'response','status':'completed','model':'gpt-5.4',
                    'output':[{'type':'message','id':'msg_fixture','role':'assistant','status':'completed',
                               'content':[{'type':'output_text','text':'fixture ok'}]}],
                    'usage':{'input_tokens':3,'output_tokens':2,'total_tokens':5}}
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream' if stream else 'application/json')
        self.end_headers()
        if stream:
            frames = [ {'type':'response.created','sequence_number':0,'response':{'id':response['id'],'object':'response'}},
                       {'type':'response.output_text.delta','sequence_number':1,'delta':'fixture ok'},
                       {'type':'response.completed','sequence_number':2,'response':response} ]
            for frame in frames:
                self.wfile.write(('data: '+json.dumps(frame)+'\n\n').encode())
                self.wfile.flush()
        else:
            self.wfile.write(json.dumps(response).encode())

ThreadingHTTPServer(('127.0.0.1', 8000), Handler).serve_forever()
