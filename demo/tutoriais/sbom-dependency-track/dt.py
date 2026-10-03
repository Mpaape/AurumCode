"""Cliente minimo da API do Dependency-Track para o tutorial (so stdlib).
Uso: dt.py METHOD CAMINHO [--json CORPO | --form k=v ...]
Cabecalho de autenticacao em DT_AUTH ("Authorization: Bearer x" ou "X-Api-Key: y")."""
import os, sys, urllib.request, urllib.parse, urllib.error

base = "http://127.0.0.1:8081"
method, path, rest = sys.argv[1], sys.argv[2], sys.argv[3:]
headers, data = {}, None
if os.environ.get("DT_AUTH"):
    k, v = os.environ["DT_AUTH"].split(": ", 1)
    headers[k] = v
if rest[:1] == ["--json"]:
    data = rest[1].encode()
    headers["Content-Type"] = "application/json"
elif rest[:1] == ["--form"]:
    data = urllib.parse.urlencode([tuple(a.split("=", 1)) for a in rest[1:]]).encode()
req = urllib.request.Request(base + path, data=data, method=method, headers=headers)
try:
    with urllib.request.urlopen(req, timeout=30) as r:
        sys.stdout.write(r.read().decode())
except urllib.error.HTTPError as e:
    sys.stderr.write("HTTP %d %s\n" % (e.code, path))
    sys.exit(1)
except Exception as e:
    sys.stderr.write("erro %s\n" % e)
    sys.exit(2)
