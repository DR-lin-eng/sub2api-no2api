from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import hashlib
from email.parser import BytesParser
from email.policy import default

last = {}

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, value, code=200):
        raw = json.dumps(value).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        self.send(last if self.path == '/fixture/last' else {'data': []})

    def do_POST(self):
        global last
        raw = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        if self.headers.get('Content-Type', '').startswith('multipart/'):
            last = {'path': self.path, 'bytes': len(raw), 'tail': raw[-80:].decode(errors='replace')}
            message = BytesParser(policy=default).parsebytes(('Content-Type: ' + self.headers['Content-Type'] + '\r\nMIME-Version: 1.0\r\n\r\n').encode() + raw)
            for part in message.iter_parts():
                if part.get_filename() == 'fixture.png':
                    data = part.get_payload(decode=True)
                    last.update(image_bytes=len(data), image_sha256=hashlib.sha256(data).hexdigest())
            return self.send({'created': 1793059200, 'data': [{'b64_json': 'eA=='}]})
        body = json.loads(raw or '{}')
        last = body
        if 'generateContent' in self.path or 'streamGenerateContent' in self.path:
            response = {'modelVersion': 'custom-gemini-fixture', 'candidates': [{'content': {'role': 'model', 'parts': [
                {'fileData': {'mimeType': 'image/png', 'fileUri': 'gs://fixture/a.png'}},
                {'file_data': {'mime_type': 'image/webp', 'file_uri': 'gs://fixture/b.webp'}}]}, 'finishReason': 'STOP'}],
                'usageMetadata': {'promptTokenCount': 1000, 'candidatesTokenCount': 100, 'totalTokenCount': 1100}}
            if 'streamGenerateContent' not in self.path:
                return self.send(response)
            events = [response, response]  # Repeated cumulative frame must not double charge.
        else:
            response = {'id': 'chatcmpl_fixture', 'object': 'chat.completion', 'created': 1793059200,
                'model': body.get('model', 'deepseek-v4-pro'), 'choices': [{'index': 0, 'message': {'role': 'assistant', 'content': 'ok'}, 'finish_reason': 'stop'}],
                'usage': {'prompt_tokens': 2000, 'completion_tokens': 500, 'total_tokens': 2500,
                    'prompt_cache_hit_tokens': 1000, 'prompt_cache_miss_tokens': 1000}}
            if not body.get('stream'):
                return self.send(response)
            events = [{'id': response['id'], 'object': 'chat.completion.chunk', 'created': response['created'], 'model': response['model'],
                'choices': [{'index': 0, 'delta': {'role': 'assistant', 'content': 'ok'}, 'finish_reason': None}]},
                {**response, 'object': 'chat.completion.chunk', 'choices': [{'index': 0, 'delta': {}, 'finish_reason': 'stop'}]}]
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        for event in events:
            self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
        self.wfile.write(b'data: [DONE]\n\n')
        self.wfile.flush()

ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
