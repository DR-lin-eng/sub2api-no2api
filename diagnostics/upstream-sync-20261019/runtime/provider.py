"""Offline Gemini fixtures. No external destinations or provider credentials."""
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        self.rfile.read(int(self.headers.get('Content-Length', 0)))
        stream = 'streamGenerateContent' in self.path
        response = {'candidates': [{'content': {'role': 'model', 'parts': [{'text': 'fixture-end'}]}, 'finishReason': 'STOP'}],
                    'usageMetadata': {'promptTokenCount': 10, 'candidatesTokenCount': 5, 'totalTokenCount': 15}}
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream' if stream else 'application/json')
        self.end_headers()
        if stream:
            self.wfile.write(('data: ' + json.dumps(response) + '\n\n').encode())
            self.wfile.flush()
            # Keep the upstream body open while the downstream disconnects.
            time.sleep(4)
        else:
            response['usageMetadata'] = {'cachedContentTokenCount': 30}
            self.wfile.write(json.dumps(response).encode())


ThreadingHTTPServer(('127.0.0.1', 8000), Handler).serve_forever()
