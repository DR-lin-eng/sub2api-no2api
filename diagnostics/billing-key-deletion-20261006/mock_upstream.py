"""Local-only model fixture; block completion until the key has been deleted."""
import json
import hashlib
import pathlib
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

CASES = {}
LOCK = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def json_reply(self, data, status=200):
        body = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = urlparse(self.path).path
        if path in ("/pricing.json", "/pricing.sha256"):
            raw = pathlib.Path("/audit/pricing.json").read_bytes()
            if path.endswith(".sha256"):
                raw = hashlib.sha256(raw).hexdigest().encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)
            return
        case = parse_qs(urlparse(self.path).query).get("case", [""])[0]
        with LOCK:
            state = CASES.get(case)
        self.json_reply({"started": state is not None, "requests": 0 if state is None else state[1]})

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        if self.path == "/control/release":
            with LOCK:
                state = CASES.get(body["case"])
            if state:
                state[0].set()
            self.json_reply({"released": state is not None})
            return
        match = re.search(r"audit-[a-z0-9-]+", json.dumps(body))
        if not match:
            self.json_reply({"error": "missing audit marker"}, 400)
            return
        case = match.group()
        with LOCK:
            state = CASES.setdefault(case, [threading.Event(), 0])
            state[1] += 1
        stream = bool(body.get("stream"))
        model = body.get("model", "gpt-4o-mini")
        path = urlparse(self.path).path
        is_messages = path.endswith("/messages")
        is_chat = path.endswith("/chat/completions")
        usage = {"input_tokens": 1000, "output_tokens": 500, "total_tokens": 1500,
                 "input_tokens_details": {"cached_tokens": 0}}
        response = {"id": "resp_" + case, "object": "response", "model": model,
                    "status": "completed", "usage": usage,
                    "output": [{"id": "msg_" + case, "type": "message", "role": "assistant",
                                "status": "completed", "content": [{"type": "output_text", "text": "audit result", "annotations": []}]}]}
        message = {"id": "msg_" + case, "type": "message", "role": "assistant", "model": model,
                   "content": [{"type": "text", "text": "audit result"}], "stop_reason": "end_turn",
                   "stop_sequence": None, "usage": {"input_tokens": 1000, "output_tokens": 500}}
        chat = {"id": "chatcmpl_" + case, "object": "chat.completion", "created": 1, "model": model,
                "choices": [{"index": 0, "message": {"role": "assistant", "content": "audit result"}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 1000, "completion_tokens": 500, "total_tokens": 1500}}
        if stream:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("x-request-id", case)
            self.end_headers()
            if is_messages:
                initial = dict(message, content=[], stop_reason=None, usage={"input_tokens": 1000, "output_tokens": 0})
                self.event("message_start", {"type": "message_start", "message": initial})
            elif is_chat:
                self.event(None, dict(chat, object="chat.completion.chunk", usage=None,
                                     choices=[{"index": 0, "delta": {"role": "assistant", "content": "audit "}, "finish_reason": None}]))
            else:
                self.event("response.created", {"type": "response.created", "response": dict(response, status="in_progress", usage=None, output=[])})
                self.event("response.output_item.added", {"type": "response.output_item.added", "output_index": 0,
                                                          "item": {"id": "msg_" + case, "type": "message", "role": "assistant", "content": []}})
                self.event("response.output_text.delta", {"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "audit "})
            if not state[0].wait(30):
                raise TimeoutError("key deletion test never released upstream")
            if is_messages:
                self.event("content_block_start", {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}})
                self.event("content_block_delta", {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "audit result"}})
                self.event("content_block_stop", {"type": "content_block_stop", "index": 0})
                self.event("message_delta", {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 500}})
                self.event("message_stop", {"type": "message_stop"})
            elif is_chat:
                self.event(None, dict(chat, object="chat.completion.chunk", choices=[{"index": 0, "delta": {}, "finish_reason": "stop"}]))
                self.wfile.write(b"data: [DONE]\n\n")
            else:
                self.event("response.output_text.delta", {"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "result"})
                self.event("response.completed", {"type": "response.completed", "response": response})
            self.wfile.flush()
        else:
            if not state[0].wait(30):
                self.json_reply({"error": "test timeout"}, 504)
                return
            self.json_reply(message if is_messages else chat if is_chat else response)

    def event(self, name, body):
        data = ("" if name is None else "event: " + name + "\n") + "data: " + json.dumps(body) + "\n\n"
        self.wfile.write(data.encode())
        self.wfile.flush()


ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
