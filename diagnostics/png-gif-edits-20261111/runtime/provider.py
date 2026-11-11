"""Isolated model backend and HTTPS signed-storage fixture; no real credentials."""
import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import re
import ssl
import sys

calls, uploads = [], []


def animated_gif(frames=4, delay=200):
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
        uploads.append({'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data),
                        'method': self.command, 'path': self.path,
                        'has_auth': any(self.headers.get(k) for k in ['Authorization', 'Cookie', 'ChatGPT-Account-ID'])})
        if not data.startswith(b'\x89PNG\r\n\x1a\n'):
            return self.send({'error': 'expected PNG'}, 400)
        if any(self.headers.get(key) != value for key,value in {'x-ms-blob-type':'BlockBlob','x-ms-version':'2020-04-08','x-ms-blob-content-type':'image/png'}.items()):
            return self.send({'error':'missing Azure upload headers'},400)
        self.send({}, 201)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        if self.path.endswith('/sentinel/chat-requirements/prepare'):
            return self.send({'token': 'fixture-token'})
        calls.append({'path': self.path, 'body': body})
        if self.path.endswith('/codex/images/edits'):
            return self.send({'detail': 'GIF output is not enabled for this request.'}, 400)
        if self.path.endswith('/files'):
            return self.send({'file_id': 'file_reference', 'upload_url': 'https://signed-storage.oaiusercontent.com:8443/reference.png?sig=fixture',
                              'upload_headers': {'Authorization': 'must-not-forward'}})
        if self.path.endswith('/files/process_upload_stream'):
            return self.send(json.dumps({'file_id': 'file_reference', 'event': 'file.processing.completed'}) + '\n', kind='application/x-ndjson')
        if self.path.endswith('/f/conversation'):
            parts = body['messages'][0]['content']['parts']
            if parts[0].get('asset_pointer') != 'sediment://file_reference':
                return self.send({'error': 'missing reference'}, 400)
            match = re.search(r'exactly (\d+) frames and (\d+) milliseconds', parts[-1])
            frames, delay = map(int, match.groups())
            event = {'conversation_id': 'conv-fixture', 'message': {'author': {'role': 'assistant'}, 'output': [
                {'type': 'image_generation_call', 'id': 'image-fixture', 'status': 'completed',
                 'result': 'data:image/gif;base64,' + animated_gif(frames, delay)}]}}
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
