# Synthetic upstream; never contacts TypeSafe or a payment provider.
import json, uuid, datetime
from http.server import BaseHTTPRequestHandler,HTTPServer
class Mock(BaseHTTPRequestHandler):
 def do_GET(self):
  assert self.path=='/v1/sub2api/billing'
  out={'object':'sub2api.key_billing','schema_version':1,'billing_scope':'token','group_rate_multiplier':1,'resolved_rate_multiplier':1,'peak_rate_enabled':False,'effective_rate_multiplier':1,'observed_at':datetime.datetime.now(datetime.timezone.utc).isoformat()}
  self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(out).encode())
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
  assert self.path=='/v1/systemone'
  assert self.headers.get('Authorization')=='Bearer typesafe-fixture-key'
  if body.get('state')=='force-failover':
   self.send_response(503); self.end_headers();self.wfile.write(b'{"error":{"message":"synthetic outage"}}');return
  out={'model':'jev-1.13.0','answers':{'q':{'type':'noul','noul':0.1}},'usage':{'input_tokens':123,'output_tokens':7},'extension':{'kept':True}}
  self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('x-request-id','fixture-'+uuid.uuid4().hex);self.end_headers();self.wfile.write(json.dumps(out).encode())
 def log_message(self,*args): pass
HTTPServer(('0.0.0.0',8080),Mock).serve_forever()
