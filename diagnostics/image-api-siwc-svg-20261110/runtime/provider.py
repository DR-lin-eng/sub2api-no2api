"""Local image provider; records protocol fields, never bearer values."""
import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json

SVG = '<svg xmlns="http://www.w3.org/2000/svg"><text>向量 🚲</text></svg>'
PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a1S8AAAAASUVORK5CYII='
gif_header = b'GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff'
gif_frame = b'\x21\xf9\x04\x00\x0a\x00\x00\x00\x2c\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00'
GIF = base64.b64encode(gif_header + gif_frame * 4 + b'\x3b').decode()
last = {}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, value, content_type='application/json'):
        raw = (json.dumps(value) if content_type == 'application/json' else value).encode()
        self.send_response(200)
        self.send_header('Content-Type', content_type)
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        self.send(last if self.path == '/fixture/last' else {'data': []})

    def do_POST(self):
        global last
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        last = {'path': self.path, 'body': body, 'headers': [k.lower() for k in self.headers]}
        fmt = body.get('output_format', 'png')
        if fmt == 'svg':
            encoded = base64.b64encode(SVG.encode('utf-16')).decode()
        elif fmt == 'gif':
            encoded = GIF
        else:
            encoded = PNG
        if body.get('prompt') == 'mislabeled':
            encoded = PNG
        response = {'created': 1794268800, 'data': [{'b64_json': encoded, 'output_format': fmt}],
                    'usage': {'input_tokens': 100, 'output_tokens': 20}}
        if fmt == 'svg':
            response['svg_usage'] = {'model': 'gpt-6-astra', 'input_tokens': 50, 'output_tokens': 20}
        if self.path.endswith('/responses'):
            response = {'id': 'resp-fixture', 'status': 'completed',
                        'output': [{'type': 'image_generation_call', 'id': 'ig-fixture', 'result': PNG}],
                        'usage': response['usage']}
            return self.send('data: ' + json.dumps({'type': 'response.completed', 'response': response}) + '\n\ndata: [DONE]\n\n', 'text/event-stream')
        if body.get('stream'):
            return self.send('data: ' + json.dumps({'type': 'image_generation.completed', 'b64_json': encoded,
                             'output_format': fmt, 'usage': response['usage']}) + '\n\ndata: [DONE]\n\n', 'text/event-stream')
        self.send(response)


ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
