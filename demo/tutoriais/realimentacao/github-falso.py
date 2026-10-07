#!/usr/bin/env python3
"""GitHub falso e local do tutorial de realimentacao (AUR-532). Escuta em
127.0.0.1, sem rede externa. Serve um repositorio de aplicacao (exemplo/app:
alertas de code scanning e comentarios lidos de github/) e um repositorio de
politica (exemplo/politica: arquivos por branch, comecando por politica/ em
main). Cada escrita do produto vira uma linha em LOG_ARQUIVO.

uso: github-falso.py PORTA DIR_TUTORIAL LOG_ARQUIVO
"""
import base64, json, os, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlsplit, parse_qs, unquote

porta, raiz, log_arq = int(sys.argv[1]), sys.argv[2], sys.argv[3]
APP, POL = "/repos/exemplo/app", "/repos/exemplo/politica"


def ler(rel):
    with open(os.path.join(raiz, rel), "rb") as f:
        return f.read()


def arvore(dir_rel):
    out = {}
    base = os.path.join(raiz, dir_rel)
    for d, _, arquivos in os.walk(base):
        for a in arquivos:
            p = os.path.join(d, a)
            out[os.path.relpath(p, base).replace(os.sep, "/")] = open(p, "rb").read()
    return out


branches = {"main": arvore("politica")}
abertas = []


def grava(linha):
    with open(log_arq, "a") as f:
        f.write(json.dumps(linha, ensure_ascii=False) + "\n")


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, body):
        data = body if isinstance(body, bytes) else json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _corpo(self):
        n = int(self.headers.get("Content-Length", "0"))
        return json.loads(self.rfile.read(n) or b"{}")

    def do_GET(self):
        u = urlsplit(self.path)
        p, q = unquote(u.path), parse_qs(u.query)
        if p == APP + "/code-scanning/alerts":
            return self._send(200, ler("github/alertas.json"))
        if p == APP + "/actions/artifacts":
            return self._send(200, {"artifacts": []})
        if p == APP + "/issues/comments":
            return self._send(200, ler("github/comentarios.json"))
        if p == APP + "/pulls/7":
            return self._send(200, {"head": {"sha": "7777777777777777777777777777777777777777"}})
        if p == POL:
            return self._send(200, {"default_branch": "main"})
        if p == POL + "/pulls":
            return self._send(200, [{"number": n, "state": "open", "head": {"sha": "sha-aurum-realimentacao"}} for n in abertas])
        if p.startswith(POL + "/git/ref/heads/"):
            b = p[len(POL + "/git/ref/heads/"):]
            if b not in branches:
                return self._send(404, {"message": "Not Found"})
            return self._send(200, {"object": {"sha": "sha-" + b.replace("/", "-")}})
        if p.startswith(POL + "/contents/"):
            caminho = p[len(POL + "/contents/"):]
            ref = q.get("ref", ["main"])[0]
            conteudo = branches.get(ref, {}).get(caminho)
            if conteudo is None:
                return self._send(404, {"message": "Not Found"})
            return self._send(200, {"content": base64.b64encode(conteudo).decode(), "encoding": "base64", "sha": "blob"})
        return self._send(404, {"message": "Not Found"})

    def do_POST(self):
        p = unquote(urlsplit(self.path).path)
        corpo = self._corpo()
        if p == POL + "/git/refs":
            b = corpo["ref"][len("refs/heads/"):]
            branches[b] = dict(branches["main"])
            grava({"escrita": "branch", "nome": b})
            return self._send(201, {})
        if p == POL + "/pulls":
            numero = 101 + len(abertas)
            abertas.append(numero)
            grava({"escrita": "pr-aberta", "numero": numero, "head": corpo["head"], "base": corpo["base"], "titulo": corpo["title"]})
            return self._send(201, {"number": numero})
        return self._send(404, {"message": "Not Found"})

    def do_PATCH(self):
        p = unquote(urlsplit(self.path).path)
        grava({"escrita": "pr-atualizada", "caminho": p})
        return self._send(200, {})

    def do_PUT(self):
        p = unquote(urlsplit(self.path).path)
        corpo = self._corpo()
        caminho = p[len(POL + "/contents/"):]
        branches[corpo["branch"]][caminho] = base64.b64decode(corpo["content"])
        grava({"escrita": "arquivo", "branch": corpo["branch"], "caminho": caminho})
        return self._send(200, {})


HTTPServer(("127.0.0.1", porta), H).serve_forever()
