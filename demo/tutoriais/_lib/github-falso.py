#!/usr/bin/env python3
"""GitHub falso e local para os tutoriais (AUR-562): o minimo que `aurumcode review --pr`
chama. Nada de rede externa: escuta em 127.0.0.1 e grava o que o produto publica.

uso: github-falso.py PORTA DIFF_ARQUIVO LOG_ARQUIVO SHA [BASE]
  GET  repos/OWNER/REPO, pulls/N (diff e json), contents (404), reviews/comments/commits ([])
  DIFF_ARQUIVO=406: o diff e recusado como o GitHub recusa um PR acima do limite de
  linhas (406, codigo too_large); BASE, quando dado, e o base.sha do pull request.
  POST reviews, comments, statuses/<sha>: o corpo e gravado em LOG_ARQUIVO, uma linha JSON cada.
  PATCH issues/comments/ID e pulls/comments/ID (parecer editado numa rodada nova): idem.
"""
import base64
import json
import os
import urllib.parse, sys, re
from http.server import BaseHTTPRequestHandler, HTTPServer

porta, diff_arq, log_arq, sha = int(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4]
# GITHUB_FALSO_ARVORE=<dir>: contents/<caminho> servido dos arquivos desse
# diretorio (uma exportacao da branch base), como o GitHub serve skills e config
# da base. Lidos uma vez na partida; sem a variavel, contents responde 404.
def instantaneo(raiz):
    if not raiz:
        return {}
    lidos = {}
    for pasta, _, nomes in os.walk(raiz):
        for nome in nomes:
            caminho = os.path.join(pasta, nome)
            with open(caminho, "rb") as f:
                lidos[os.path.relpath(caminho, raiz).replace(os.sep, "/")] = f.read()
    return lidos


arquivos = instantaneo(os.environ.get("GITHUB_FALSO_ARVORE", ""))
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
            return self._contents(p)
        if re.search(r"/(reviews|comments|commits|files)$", p):
            return self._send(200, "[]")
        self._send(404, "{}")

    def _grava(self, metodo, codigo):
        n = int(self.headers.get("Content-Length", "0"))
        corpo = self.rfile.read(n).decode()
        with open(log_arq, "a") as f:
            f.write(json.dumps({metodo: self.path.split("?")[0], "corpo": json.loads(corpo or "{}")}, ensure_ascii=False) + "\n")
        self._send(codigo, '{"id":1}')

    def _contents(self, p):
        caminho = urllib.parse.unquote(p.split("/contents/", 1)[1]).strip("/")
        if caminho in arquivos:
            return self._send(200, json.dumps({"type": "file", "encoding": "base64", "content": base64.b64encode(arquivos[caminho]).decode()}))
        prefixo = caminho + "/"
        filhos = {}
        for nome in arquivos:
            if nome.startswith(prefixo):
                resto = nome[len(prefixo):]
                filhos[resto.split("/", 1)[0]] = "dir" if "/" in resto else "file"
        if filhos:
            return self._send(200, json.dumps([{"name": n, "path": prefixo + n, "type": t} for n, t in sorted(filhos.items())]))
        return self._send(404, "{}")

    def do_POST(self):
        self._grava("POST", 201)

    def do_PATCH(self):
        self._grava("PATCH", 200)


HTTPServer(("127.0.0.1", porta), H).serve_forever()
