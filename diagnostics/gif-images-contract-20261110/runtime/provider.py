"""Controlled App-capable account without Codex-native GIF access."""
import base64
import json
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

calls = []


def animated_gif(frames, delay):
    header = b'GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff'
    frame = b'\x21\xf9\x04\x00' + int(delay // 10).to_bytes(2, 'little') + b'\x00\x00\x2c\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00'
    return base64.b64encode(header + frame * frames + b'\x3b').decode()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, value, status=200, kind='application/json'):
        raw = (json.dumps(value) if kind == 'application/json' else value).encode()
        self.send_response(status)
        self.send_header('Content-Type', kind)
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        self.send(calls if self.path == '/fixture/calls' else {'data': []})

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        if self.path.endswith('/sentinel/chat-requirements/prepare'):
            return self.send({'token': 'fixture-requirements-token'})
        calls.append({'path': self.path, 'body': body})
        if self.path == '/backend-api/codex/images/generations':
            # Model a capability difference, not an actual OpenAI server error.
            return self.send({'error': {'message': 'fixture: native GIF unavailable', 'type': 'invalid_request_error'}}, 400)
        if self.path == '/backend-api/f/conversation':
            prompt = body['messages'][0]['content']['parts'][-1]
            matched = re.search(r'exactly (\d+) frames and (\d+) milliseconds', prompt)
            frames, delay = map(int, matched.groups()) if matched else (9, 200)
            encoded = animated_gif(frames, delay)
            event = {'conversation_id': 'conv-fixture', 'message': {'author': {'role': 'assistant'}, 'output': [
                {'type': 'image_generation_call', 'status': 'completed', 'result': 'data:image/gif;base64,' + encoded}]}}
            return self.send('data: ' + json.dumps(event) + '\n\ndata: [DONE]\n\n', kind='text/event-stream')
        encoded = animated_gif(body.get('frame_count', 9), body.get('frame_duration_ms', 200))
        self.send({'created': 1794268800, 'output_format': 'gif', 'data': [{'b64_json': encoded}],
                   'usage': {'input_tokens': 100, 'output_tokens': 20}})


ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
