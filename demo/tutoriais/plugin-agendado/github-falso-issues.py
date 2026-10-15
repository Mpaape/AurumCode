#!/usr/bin/env python3
"""GitHub falso e local do destino issues (AUR-623): o minimo que
`aurumcode plugin issues` chama. Escuta so em 127.0.0.1, guarda as issues em
memoria e grava cada requisicao (metodo e rota) em LOG e o estado das issues
em ESTADO (JSON), para o tutorial mostrar o que o produto fez.

uso: github-falso-issues.py PORTA VISIBILIDADE LOG ESTADO
  GET  repos/O/R                      {"visibility": VISIBILIDADE, "private": ...}
  GET  repos/O/R/labels/NOME          404 ate o POST repos/O/R/labels
  GET  repos/O/R/issues?labels=&page= 2 por pagina (a listagem cruza paginas)
  POST repos/O/R/issues               abre (com os rotulos pedidos)
  PATCH repos/O/R/issues/N            muda titulo, corpo e estado
  POST repos/O/R/issues/N/comments    comenta
"""
import json
import re
import sys
import urllib.parse
from http.server import BaseHTTPRequestHandler, HTTPServer

porta, visibilidade, log_arq, estado_arq = int(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4]
rotulos, issues, comentarios = set(), [], {}
POR_PAGINA = 2


def grava_estado():
    with open(estado_arq, "w") as f:
        json.dump({"issues": issues, "comentarios": comentarios}, f, ensure_ascii=False)


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, obj):
        data = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _rota(self):
        url = urllib.parse.urlsplit(self.path)
        with open(log_arq, "a") as f:
            f.write("%s %s\n" % (self.command, url.path))
        m = re.fullmatch(r"/repos/[^/]+/[^/]+(/.*)?", url.path)
        return (m.group(1) or "") if m else None, urllib.parse.parse_qs(url.query)

    def _corpo(self):
        n = int(self.headers.get("Content-Length", "0"))
        return json.loads(self.rfile.read(n) or b"{}")

    def _issue(self, i):
        return dict(i, labels=[{"name": r} for r in i["labels"]])

    def do_GET(self):
        rota, q = self._rota()
        if rota == "":
            return self._send(200, {"visibility": visibilidade, "private": visibilidade != "public"})
        if rota is not None and rota.startswith("/labels/"):
            nome = urllib.parse.unquote(rota[len("/labels/"):])
            return self._send(200 if nome in rotulos else 404, {})
        if rota == "/issues":
            rotulo, pagina = q.get("labels", [""])[0], int(q.get("page", ["1"])[0])
            todas = [self._issue(i) for i in issues if rotulo in i["labels"]]
            return self._send(200, todas[(pagina - 1) * POR_PAGINA:pagina * POR_PAGINA])
        self._send(404, {})

    def do_POST(self):
        rota, _ = self._rota()
        corpo = self._corpo()
        if rota == "/labels":
            rotulos.add(corpo["name"])
            return self._send(201, {})
        if rota == "/issues":
            i = {"number": len(issues) + 1, "title": corpo["title"], "body": corpo["body"], "state": "open", "labels": corpo.get("labels", [])}
            issues.append(i)
            grava_estado()
            return self._send(201, self._issue(i))
        m = re.fullmatch(r"/issues/(\d+)/comments", rota or "")
        if m:
            comentarios.setdefault(m.group(1), []).append(corpo["body"])
            grava_estado()
            return self._send(201, {})
        self._send(404, {})

    def do_PATCH(self):
        rota, _ = self._rota()
        corpo = self._corpo()
        m = re.fullmatch(r"/issues/(\d+)", rota or "")
        if not m or int(m.group(1)) > len(issues):
            return self._send(404, {})
        i = issues[int(m.group(1)) - 1]
        for campo in ("title", "body", "state"):
            if campo in corpo:
                i[campo] = corpo[campo]
        grava_estado()
        self._send(200, self._issue(i))


grava_estado()
HTTPServer(("127.0.0.1", porta), H).serve_forever()
