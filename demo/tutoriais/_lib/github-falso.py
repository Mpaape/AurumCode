#!/usr/bin/env python3
"""GitHub falso e local para os tutoriais (AUR-562): o minimo que `aurumcode review --pr`
chama. Nada de rede externa: escuta em 127.0.0.1 e grava o que o produto publica.

uso: github-falso.py PORTA DIFF_ARQUIVO LOG_ARQUIVO SHA [BASE]
  GET  repos/OWNER/REPO, pulls/N (diff e json), contents (404), reviews/comments/commits ([])
  DIFF_ARQUIVO=406: o diff e recusado como o GitHub recusa um PR acima do limite de
  linhas (406, codigo too_large); BASE, quando dado, e o base.sha do pull request.
  POST reviews, comments, statuses/<sha>: o corpo e gravado em LOG_ARQUIVO, uma linha JSON cada.
"""
import json, sys, re
from http.server import BaseHTTPRequestHandler, HTTPServer

porta, diff_arq, log_arq, sha = int(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4]
base = sys.argv[5] if len(sys.argv) > 5 else ""
RECUSA = '{"message":"Sorry, the diff exceeded the maximum number of lines (20000)","errors":[{"resource":"PullRequest","field":"diff","code":"too_large"}],"status":"406"}'


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype="application/json"):
        data = body if isinstance(body, bytes) else body.encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        p = self.path.split("?")[0]
        if re.fullmatch(r"/repos/[^/]+/[^/]+", p):
            return self._send(200, '{"permissions":{"push":true}}')
        if re.fullmatch(r"/repos/[^/]+/[^/]+/pulls/\d+", p):
            if "diff" in self.headers.get("Accept", ""):
                if diff_arq == "406":
                    return self._send(406, RECUSA)
                return self._send(200, open(diff_arq, "rb").read(), "text/plain")
            meta = {"head": {"sha": sha}}
            if base:
                meta["base"] = {"sha": base}
            return self._send(200, json.dumps(meta))
        if "/contents/" in p:
            return self._send(404, "{}")
        if re.search(r"/(reviews|comments|commits|files)$", p):
            return self._send(200, "[]")
        self._send(404, "{}")

    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        corpo = self.rfile.read(n).decode()
        with open(log_arq, "a") as f:
            f.write(json.dumps({"POST": self.path.split("?")[0], "corpo": json.loads(corpo or "{}")}, ensure_ascii=False) + "\n")
        self._send(201, '{"id":1}')


HTTPServer(("127.0.0.1", porta), H).serve_forever()
