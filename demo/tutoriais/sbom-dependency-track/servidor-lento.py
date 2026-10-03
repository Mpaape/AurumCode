"""Servidor falso que NAO e o Dependency-Track: aceita o upload do BOM e responde
"processing": true para sempre, para provar o timeout do gate de forma
deterministica. Escuta so em 127.0.0.1:8099, dentro do container do caso."""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class H(BaseHTTPRequestHandler):
    def _send(self, body):
        data = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        self._send({"token": "00000000-0000-0000-0000-000000000001"})

    def do_GET(self):
        self._send({"processing": True})

    def log_message(self, *a):
        pass


HTTPServer(("127.0.0.1", 8099), H).serve_forever()
