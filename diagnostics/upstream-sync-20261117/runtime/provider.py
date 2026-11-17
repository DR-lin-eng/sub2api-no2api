from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import uuid


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{"data":[]}')

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))))
        model = body['model']
        if self.path.endswith('/embeddings'):
            result = {'object': 'list', 'model': model,
                      'data': [{'object': 'embedding', 'index': 0, 'embedding': [0.1, 0.2]}],
                      'usage': {'prompt_tokens': 1000, 'total_tokens': 1000}}
        else:
            result = {'id': 'resp-' + uuid.uuid4().hex, 'object': 'response', 'status': 'completed',
                      'model': model, 'service_tier': body.get('service_tier', 'default'),
                      'output': [{'type': 'message', 'role': 'assistant', 'status': 'completed',
                                  'content': [{'type': 'output_text', 'text': 'ok'}]}],
                      'usage': {'input_tokens': 1000, 'output_tokens': 100, 'total_tokens': 1100}}
        self.send_response(200)
        self.send_header('X-Request-Id', 'fixture-' + uuid.uuid4().hex)
        if body.get('stream'):
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            events = [{'type': 'response.output_text.delta', 'delta': 'ok'},
                      {'type': 'response.completed', 'response': result}]
            for event in events:
                self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
        else:
            raw = json.dumps(result).encode()
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)


ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
