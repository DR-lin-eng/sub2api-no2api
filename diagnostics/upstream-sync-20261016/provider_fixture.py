"""Disposable EasyPay fixture: require query parameters like PHP/ZPay."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from urllib.parse import urlsplit, parse_qs
import threading

orders = {}
queries = {}
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, payload):
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/fixture/stats":
            with lock:
                self.send(dict(queries))
            return
        self.send({"code": -1})

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        path = urlsplit(self.path)
        if path.path == "/fixture/state":
            with lock:
                orders.update(json.loads(body))
            self.send({"ok": True})
            return
        params = parse_qs(path.query)
        order = params.get("out_trade_no", [""])[0]
        if path.path != "/api.php" or params.get("act") != ["order"]:
            self.send({"code": -1, "msg": "No Act!"})
            return
        with lock:
            queries[order] = queries.get(order, 0) + 1
            state = dict(orders.get(order, {}))
        self.send({"code": "1", "out_trade_no": order, "trade_no": "TX-" + order,
                   "money": "10.00", "status": str(state.get("paid", 0))})


ThreadingHTTPServer(("0.0.0.0", 8000), Handler).serve_forever()
