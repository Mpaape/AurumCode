#!/bin/sh
# Roda DENTRO do container do produto: sobe o servidor local (MODO) e
# executa /app/aurumcode ARGS; ao fim lista as requisicoes que o servidor recebeu
# (o endereco chega por AURUMCODE_GITHUB_API_URL, definida pelo run.sh).
# MODO=nenhum: nao sobe servidor (nada escuta no endereco padrao).
modo="$1"; shift
pki=/tmp/srv
mkdir -p "$pki"; : > "$pki/requests.log"
if [ "$modo" != nenhum ]; then
  python3 /fixtures/servidor-local.py "$modo" "$pki" >"$pki/server.log" 2>&1 &
  i=0
  until [ -e "$pki/pronto" ]; do
    i=$((i + 1))
    if [ "$i" -gt 100 ]; then cat "$pki/server.log"; echo "ERRO: servidor local nao subiu"; exit 70; fi
    sleep 0.1
  done
fi
/app/aurumcode "$@"
rc=$?
echo "--- requisicoes recebidas pelo servidor local (modo $modo): $(wc -l < "$pki/requests.log")"
sed 's/^/    /' "$pki/requests.log"
exit "$rc"
