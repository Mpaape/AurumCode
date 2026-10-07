#!/usr/bin/env python3
"""Provedor de LLM falso e local para o tutorial de provedores (AUR-596): imita,
em 127.0.0.1, o que cada dialeto OpenAI-compativel recebe. Nada de rede externa e
nenhum modelo: registra o que o produto enviou (sem a chave) e responde uma
revisao fixa.

uso: provedor-falso.py PORTA DIR  (grava DIR/pronto ao escutar e uma linha por
requisicao em DIR/requests.log)
  POST /<rota>/chat/completions
    rota fora-do-schema: responde JSON sem lista de achados
    rota erro: responde 401 no envelope da Anthropic, ecoando a chave recebida
    demais rotas: responde {"issues": []} no envelope Chat Completions
"""
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

porta, dir_ = int(sys.argv[1]), sys.argv[2]
log_arq = dir_ + "/requests.log"
REVISAO = {"verdict": "approve", "summary": "sem achados", "issues": []}
FORA = {"answer": "parece tudo certo"}


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        caminho, _, query = self.path.partition("?")
        rota = caminho.strip("/").split("/")[0]
        corpo = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        auth = self.headers.get("Authorization", "")
        visto = {
            "rota": rota,
            "caminho": caminho,
            "query": query or "-",
            "authorization": "Bearer <chave>" if auth.startswith("Bearer ") else (auth or "ausente"),
            "api-key": "<chave>" if self.headers.get("api-key") else "ausente",
            "x-gw-key": "<chave>" if self.headers.get("x-gw-key") else "ausente",
            "limite": ",".join(k for k in ("max_tokens", "max_completion_tokens") if k in corpo) or "-",
            "response_format": (corpo.get("response_format") or {}).get("type", "ausente"),
        }
        with open(log_arq, "a") as f:
            f.write("provedor recebeu: %(caminho)s query=%(query)s authorization=%(authorization)s api-key=%(api-key)s x-gw-key=%(x-gw-key)s limite=%(limite)s response_format=%(response_format)s\n" % visto)
        if rota == "erro":
            chave = auth[len("Bearer "):] if auth.startswith("Bearer ") else ""
            return self._send(401, {"type": "error", "error": {"type": "authentication_error", "message": "invalid x-api-key " + chave}})
        resposta = FORA if rota == "fora-do-schema" else REVISAO
        self._send(200, {"model": "modelo-falso", "choices": [{"index": 0, "message": {"role": "assistant", "content": json.dumps(resposta)}, "finish_reason": "stop"}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}})

    def _send(self, code, obj):
        data = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


servidor = HTTPServer(("127.0.0.1", porta), H)
open(dir_ + "/pronto", "w").close()
servidor.serve_forever()
