#!/bin/sh
# Roda DENTRO do container do produto (rede none): sobe o provedor falso em
# 127.0.0.1:8080 e executa /app/aurumcode ARGS; ao fim lista o que o provedor
# recebeu (o endereco chega por LLM_BASE_URL, definida pelo run.sh).
dir=/tmp/provedor
mkdir -p "$dir"; : > "$dir/requests.log"
python3 /fixtures/provedor-falso.py 8080 "$dir" >"$dir/server.log" 2>&1 &
i=0
until [ -e "$dir/pronto" ]; do
  i=$((i + 1))
  if [ "$i" -gt 100 ]; then cat "$dir/server.log"; echo "ERRO: provedor falso nao subiu"; exit 70; fi
  sleep 0.1
done
/app/aurumcode "$@"
rc=$?
echo "--- requisicoes recebidas pelo provedor falso: $(wc -l < "$dir/requests.log")"
sed 's/^/    /' "$dir/requests.log"
exit "$rc"
