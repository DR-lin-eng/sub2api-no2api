from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import uuid
from urllib.parse import urlsplit


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
        request_path = urlsplit(self.path).path
        if request_path.endswith('/messages'):
            result = {'id': 'msg_fixture', 'type': 'message', 'role': 'assistant', 'model': model,
                      'content': [{'type': 'text', 'text': 'ok'}], 'stop_reason': 'end_turn',
                      'usage': {'input_tokens': 10, 'output_tokens': 7, 'cache_read_input_tokens': 5,
                                'cache_creation': {'ephemeral_5m_input_tokens': 0,
                                                   'ephemeral_1h_input_tokens': 30}}}
        elif request_path.endswith('/chat/completions'):
            result = {'id': 'chatcmpl_fixture', 'object': 'chat.completion', 'model': model,
                      'choices': [{'index': 0, 'message': {'role': 'assistant', 'content': 'ok'},
                                   'finish_reason': 'stop'}],
                      'usage': {'prompt_tokens': 1000, 'completion_tokens': 100, 'total_tokens': 1100}}
        elif request_path.endswith('/embeddings'):
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
            if request_path.endswith('/messages'):
                start = {**result, 'content': [], 'stop_reason': None,
                         'usage': {'input_tokens': 10, 'output_tokens': 0, 'cache_read_input_tokens': 5,
                                   'cache_creation': {'ephemeral_5m_input_tokens': 20}}}
                events = [
                    {'type': 'message_start', 'message': start},
                    {'type': 'content_block_start', 'index': 0, 'content_block': {'type': 'text', 'text': ''}},
                    {'type': 'content_block_delta', 'index': 0, 'delta': {'type': 'text_delta', 'text': 'ok'}},
                    {'type': 'content_block_stop', 'index': 0},
                    {'type': 'message_delta', 'usage': {'cache_creation': {'ephemeral_1h_input_tokens': 30}}},
                    {'type': 'message_delta', 'delta': {'stop_reason': 'end_turn'},
                     'usage': {'output_tokens': 7, 'cache_creation': {'ephemeral_5m_input_tokens': 0}}},
                    {'type': 'message_stop'},
                ]
                for event in events:
                    self.wfile.write(('event: ' + event['type'] + '\ndata: ' + json.dumps(event) + '\n\n').encode())
                return
            if request_path.endswith('/chat/completions'):
                events = [{'id': 'chatcmpl_fixture', 'model': model, 'choices': [
                    {'index': 0, 'delta': {'content': 'ok'}, 'finish_reason': None}]},
                    {'id': 'chatcmpl_fixture', 'model': model, 'choices': [
                        {'index': 0, 'delta': {}, 'finish_reason': 'stop'}], 'usage': result['usage']}]
                for event in events:
                    self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
                self.wfile.write(b'data: [DONE]\n\n')
                return
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
