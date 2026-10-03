#!/usr/bin/env python3
"""Servidor local para o tutorial dados-de-analise (so stdlib).

Atende como a API de releases do GitHub em http://127.0.0.1:8080, DENTRO do
mesmo container do aurumcode (rede none, loopback). O produto o encontra porque
AURUMCODE_GITHUB_API_URL aponta para ele; nao ha DNS, CA nem TLS de demonstracao.

uso: servidor-local.py MODO DIRETORIO
  valido               artefato gerado ha 1 dia, integro
  vencido              artefato gerado ha 30 dias
  adulterado-arquivo   scanners.yml servido diferente do digest do manifesto
  adulterado-manifesto set_digest do manifesto nao confere com os arquivos
  indisponivel         a listagem de releases responde HTTP 503
"""
import datetime, hashlib, http.server, json, os, sys

modo, d = sys.argv[1], sys.argv[2]
HOST = "127.0.0.1:8080"

age = datetime.timedelta(days=30 if modo == "vencido" else 1)
gen = (datetime.datetime.now(datetime.timezone.utc) - age).replace(microsecond=0)
gen_s = gen.strftime("%Y-%m-%dT%H:%M:%SZ")
tag = "analysis-data/" + gen.strftime("%Y%m%dT%H%M%SZ")
scanners = b"vuln_scanner_version: 0.73.0\n"
sha = "sha256:" + hashlib.sha256(scanners).hexdigest()
files = [{"path": "scanners.yml", "sha256": sha, "size": len(scanners), "kind": "scanners"}]
set_digest = "sha256:" + hashlib.sha256("".join(sorted(f["path"] + "\x00" + f["sha256"] + "\n" for f in files)).encode()).hexdigest()
if modo == "adulterado-manifesto":
    set_digest = "sha256:" + "0" * 64
manifest = json.dumps({"schema": "aurum-analysis-data/v1", "generated_at": gen_s, "sources": [], "files": files,
                       "scanners": {"vuln_scanner_version": "0.73.0"}, "set_digest": set_digest}).encode()
if modo == "adulterado-arquivo":
    scanners = b"vuln_scanner_version: 9.9.9\n"
base = "http://" + HOST + "/dl/"
release = [{"tag_name": tag, "created_at": gen_s, "draft": False, "prerelease": False, "assets": [
    {"name": "manifest.json", "browser_download_url": base + "manifest.json"},
    {"name": "scanners.yml", "browser_download_url": base + "scanners.yml"}]}]

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def send(self, code, body=b""):
        open(os.path.join(d, "requests.log"), "a").write("%s %s -> %d\n" % (self.command, self.path.split("?")[0], code))
        self.send_response(code); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
    def do_GET(self):
        p = self.path.split("?")[0]
        if p.endswith("/releases"):
            return self.send(503) if modo == "indisponivel" else self.send(200, json.dumps(release).encode())
        if p == "/dl/manifest.json": return self.send(200, manifest)
        if p == "/dl/scanners.yml": return self.send(200, scanners)
        self.send(404)

srv = http.server.HTTPServer(("127.0.0.1", 8080), H)
open(os.path.join(d, "pronto"), "w").write("ok")
srv.serve_forever()
