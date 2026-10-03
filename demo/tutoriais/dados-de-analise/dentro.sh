#!/bin/sh
# Roda DENTRO do container do produto: sobe o falso api.github.com (MODO) e
# executa /app/aurumcode ARGS; ao fim lista as requisicoes que o falso recebeu.
# MODO=nenhum: nao sobe servidor (nada escuta em api.github.com).
modo="$1"; shift
pki=/tmp/pki
mkdir -p "$pki"; : > "$pki/requests.log"
if [ "$modo" != nenhum ]; then
  python3 /fixtures/servidor-falso.py "$modo" "$pki" >"$pki/server.log" 2>&1 &
  i=0
  until [ -e "$pki/pronto" ]; do
    i=$((i + 1))
    if [ "$i" -gt 100 ]; then cat "$pki/server.log"; echo "ERRO: falso api.github.com nao subiu"; exit 70; fi
    sleep 0.1
  done
  export SSL_CERT_FILE="$pki/ca.pem"
fi
/app/aurumcode "$@"
rc=$?
echo "--- requisicoes recebidas pelo falso api.github.com (modo $modo): $(wc -l < "$pki/requests.log")"
sed 's/^/    /' "$pki/requests.log"
exit "$rc"
