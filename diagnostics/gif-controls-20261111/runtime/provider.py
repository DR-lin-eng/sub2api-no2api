"""Isolated App/static/GIF backend and signed HTTPS reference storage."""
import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import re
import ssl
import sys

calls, uploads = [], []
PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a1S8AAAAASUVORK5CYII='


def animated_gif(frames, delay):
    header = b'GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff'
    frame = b'\x21\xf9\x04\x00' + int(delay // 10).to_bytes(2, 'little') + b'\x00\x00\x2c\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00'
    return base64.b64encode(header + frame * frames + b'\x3b').decode()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, value, status=200, kind='application/json'):
        data = (json.dumps(value) if kind == 'application/json' else value).encode()
        self.send_response(status)
        self.send_header('Content-Type', kind)
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        self.send(uploads if self.path == '/uploads' else calls)

    def do_PUT(self):
        data = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        if not data.startswith(b'\x89PNG\r\n\x1a\n'):
            return self.send({'error': 'expected reference PNG'}, 400)
        uploads.append({'sha256': hashlib.sha256(data).hexdigest(),
                        'has_auth': any(self.headers.get(k) for k in ['Authorization', 'Cookie', 'ChatGPT-Account-ID'])})
        self.send({}, 201)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        if self.path.endswith('/sentinel/chat-requirements/prepare'):
            return self.send({'token': 'fixture-token'})
        calls.append({'path': self.path, 'body': body})
        if '/codex/' in self.path:
            return self.send({'detail': 'fixture: native image route selected'}, 400)
        if self.path.endswith('/files'):
            return self.send({'file_id': 'file_reference', 'upload_url': 'https://signed-storage.oaiusercontent.com:8443/reference.png?sig=fixture'})
        if self.path.endswith('/files/process_upload_stream'):
            return self.send(json.dumps({'file_id': 'file_reference', 'event': 'file.processing.completed'}) + '\n', kind='application/x-ndjson')
        if self.path.endswith('/f/conversation'):
            parts = body['messages'][0]['content']['parts']
            if len(parts) > 1 and parts[0].get('asset_pointer') != 'sediment://file_reference':
                return self.send({'error': 'reference image lost'}, 400)
            animation = re.search(r'exactly (\d+) frames and (\d+) milliseconds', parts[-1])
            fmt, encoded = 'png', PNG
            if animation:
                fmt = 'gif'
                encoded = animated_gif(*map(int, animation.groups()))
            event = {'conversation_id': 'conv-fixture', 'message': {'author': {'role': 'assistant'}, 'output': [
                {'type': 'image_generation_call', 'id': 'image-fixture', 'status': 'completed',
                 'result': 'data:image/' + fmt + ';base64,' + encoded}]}}
            return self.send('data: ' + json.dumps(event) + '\n\ndata: [DONE]\n\n', kind='text/event-stream')
        self.send({'error': 'unexpected endpoint'}, 400)


if len(sys.argv) > 1 and sys.argv[1] == 'storage':
    server = ThreadingHTTPServer(('0.0.0.0', 8443), Handler)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain('/cert/server.pem', '/cert/server.key')
    server.socket = context.wrap_socket(server.socket, server_side=True)
else:
    server = ThreadingHTTPServer(('0.0.0.0', 8000), Handler)
server.serve_forever()
