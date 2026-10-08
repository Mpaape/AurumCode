#!/bin/sh
# Roda DENTRO do container do produto (rede none): sobe o GitHub falso em
# 127.0.0.1:8080, com o estado do caso em /work (sobrevive entre os
# containers do mesmo caso), e executa /app/aurumcode ARGS.
dir=/tmp/github
mkdir -p "$dir"
touch /work/github.log
python3 /fixtures/github-falso.py 8080 /fixtures /work/github.log /work/github-estado.json "$dir/pronto" >"$dir/server.log" 2>&1 &
servidor=$!
i=0
until [ -e "$dir/pronto" ]; do
  i=$((i + 1))
  if [ "$i" -gt 100 ]; then cat "$dir/server.log"; echo "ERRO: GitHub falso nao subiu"; exit 70; fi
  sleep 0.1
done
/app/aurumcode "$@"
rc=$?
kill "$servidor" 2>/dev/null
exit "$rc"
