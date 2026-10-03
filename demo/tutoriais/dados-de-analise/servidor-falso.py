#!/usr/bin/env python3
"""Falso api.github.com para o tutorial dados-de-analise (so stdlib + cryptography).

Roda DENTRO do mesmo container do aurumcode (rede none, loopback): o container
resolve api.github.com para 127.0.0.1 (--add-host) e confia num CA gerado aqui,
em memoria/tmp, a cada execucao (SSL_CERT_FILE). Nenhuma chave e versionada.

uso: servidor-falso.py MODO DIRETORIO
  valido               artefato gerado ha 1 dia, integro
  vencido              artefato gerado ha 30 dias
  adulterado-arquivo   scanners.yml servido diferente do digest do manifesto
  adulterado-manifesto set_digest do manifesto nao confere com os arquivos
  indisponivel         a listagem de releases responde HTTP 503
"""
import datetime, hashlib, http.server, json, os, ssl, sys

modo, d = sys.argv[1], sys.argv[2]
HOST = "api.github.com"

def pki():
    from cryptography import x509
    from cryptography.x509.oid import NameOID
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    now = datetime.datetime.now(datetime.timezone.utc)
    def name(cn): return x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)])
    ck = ec.generate_private_key(ec.SECP256R1())
    ca = (x509.CertificateBuilder().subject_name(name("CA de demonstracao")).issuer_name(name("CA de demonstracao"))
          .public_key(ck.public_key()).serial_number(x509.random_serial_number())
          .not_valid_before(now - datetime.timedelta(days=1)).not_valid_after(now + datetime.timedelta(days=1))
          .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
          .sign(ck, hashes.SHA256()))
    lk = ec.generate_private_key(ec.SECP256R1())
    leaf = (x509.CertificateBuilder().subject_name(name(HOST)).issuer_name(ca.subject)
            .public_key(lk.public_key()).serial_number(x509.random_serial_number())
            .not_valid_before(now - datetime.timedelta(days=1)).not_valid_after(now + datetime.timedelta(days=1))
            .add_extension(x509.SubjectAlternativeName([x509.DNSName(HOST)]), critical=False)
            .sign(ck, hashes.SHA256()))
    pem = serialization.Encoding.PEM
    open(os.path.join(d, "ca.pem"), "wb").write(ca.public_bytes(pem))
    open(os.path.join(d, "leaf.pem"), "wb").write(
        leaf.public_bytes(pem) + lk.private_bytes(pem, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))

pki()

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
base = "https://%s/dl/" % HOST
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

srv = http.server.HTTPServer(("127.0.0.1", 443), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(os.path.join(d, "leaf.pem"))
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
open(os.path.join(d, "pronto"), "w").write("ok")
srv.serve_forever()
