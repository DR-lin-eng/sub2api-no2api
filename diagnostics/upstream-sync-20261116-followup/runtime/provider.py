from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json

last = {}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, value, status=200):
        raw = json.dumps(value).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        if self.path == '/v1beta/models':
            return self.send({'error':{'code':403,'message':'insufficient authentication scopes'}},403)
        self.send(last if self.path == '/fixture/last' else {'data': []})

    def do_POST(self):
        global last
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        # These are fixture labels, never real account credentials.
        label = self.headers.get('Authorization', '').removeprefix('Bearer ')
        last = {'path': self.path, 'label': label, 'body': body}
        if body.get('prompt') == 'fixture-flat-error':
            return self.send({'code':'invalid_image','error':'Image fixture is invalid.'},400)
        for tool in body.get('tools', []):
            pattern = tool.get('parameters', {}).get('properties', {}).get('label', {}).get('pattern', '')
            if pattern == '(?<name>abc)':
                return self.send({'error':{'type':'invalid_request_error','message':'Unsupported tool pattern'}},400)
        if str(body.get('model', '')).startswith('kimi-'):
            return self.send({'error': {'message': "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."}}, 403)
        if '/images/' in self.path:
            return self.send({'created': 1793779200, 'data': [{'b64_json': 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII='}, {'b64_json': 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII='}]})
        if 'generateContent' in self.path or 'streamGenerateContent' in self.path:
            result = {'candidates':[{'content':{'role':'model','parts':[{'text':'ok'}]},'finishReason':'STOP'}],
                'usageMetadata':{'promptTokenCount':1000,'candidatesTokenCount':100,'totalTokenCount':1100}, 'modelVersion':'gemini-4-carbon'}
            if 'streamGenerateContent' not in self.path: return self.send(result)
            self.send_response(200)
            self.send_header('Content-Type','text/event-stream')
            self.end_headers()
            self.wfile.write(('data: '+json.dumps(result)+'\n\n').encode())
            self.wfile.flush()
            return
        response = {'id': 'resp-fixture', 'object': 'response', 'status': 'completed',
                    'model': body.get('model'), 'instructions': body.get('instructions'),
                    'output': [{'type': 'message', 'role': 'assistant', 'content': [{'type': 'output_text', 'text': 'ok'}]}],
                    'usage': {'input_tokens': 1000, 'output_tokens': 100, 'total_tokens': 1100}}
        if 'reasoning-fixture' in json.dumps(body.get('input')):
            response['output'].insert(0,{'type':'reasoning','summary':[],'content':[{'type':'reasoning_text','text':'think more'}]})
        if not body.get('stream'):
            return self.send(response)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        for event in [{'type': 'response.created', 'response': {**response, 'output': [], 'status': 'in_progress'}},
                      {'type': 'response.output_text.delta', 'delta': 'ok'},
                      {'type': 'response.completed', 'response': response}]:
            self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
        self.wfile.write(b'data: [DONE]\n\n')
        self.wfile.flush()


ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
