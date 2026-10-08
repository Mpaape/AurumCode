#!/bin/sh
# Medicao do corpus antes/depois de uma PR de realimentacao (AUR-532), rodada
# por .github/workflows/realimentacao-medicao.yml na imagem golang alpine,
# a partir do checkout do AurumCode. $1 e $2 sao os checkouts da politica na
# base e na PR. Cada um traz o corpus no layout do AUR-523 (corpus/cases) e a
# propria politica (.aurumcode/); o harness do AUR-523 roda uma vez para cada
# lado e grava o relatorio. A comparacao sai em $3 (Markdown); exit 1 em
# regressao ou relatorio ausente (nunca "melhorou" sem medir).
set -eu
base="${1:-}"
head="${2:-}"
out="${3:-}"
if [ -z "$base" ] || [ -z "$head" ] || [ -z "$out" ]; then
  echo "medicao: uso: realimentacao-medir.sh <politica-base> <politica-pr> <saida.md>" >&2
  exit 2
fi
if ! command -v git >/dev/null 2>&1; then
  apk add --no-cache git >/dev/null
fi
export GOFLAGS=-buildvcs=false
tool="$(pwd)"
corpus="$tool/tests/benchmark/testdata/multilang"
measure() {
  side="$1"
  report="$2"
  if [ ! -d "$side/corpus/cases" ] || [ ! -f "$side/.aurumcode/config.yml" ]; then
    echo "medicao: $side sem corpus/cases ou .aurumcode/config.yml" >&2
    return 1
  fi
  rm -rf "$corpus/cases" "$corpus/policy/.aurumcode"
  cp -R "$side/corpus/cases" "$corpus/cases"
  mkdir -p "$corpus/policy"
  cp -R "$side/.aurumcode" "$corpus/policy/.aurumcode"
  rm -f "$tool/tests/benchmark/out/multilang-report.json"
  (cd "$tool/tests/benchmark" && go test -count=1 -run '^(TestAUR523CorpusLabeledVersionedWithDigest|TestAUR523ReportPerLanguage)$' . -args -update-aur523) || return 1
  cp "$tool/tests/benchmark/out/multilang-report.json" "$report"
}
before=/tmp/medicao-antes.json
after=/tmp/medicao-depois.json
status=0
measure "$base" "$before" || { echo "medicao: base nao medida" >&2; rm -f "$before"; status=1; }
measure "$head" "$after" || { echo "medicao: PR nao medida" >&2; rm -f "$after"; status=1; }
set -- --medir
if [ -f "$before" ]; then
  set -- "$@" --medicao-antes "$before"
fi
if [ -f "$after" ]; then
  set -- "$@" --medicao-depois "$after"
fi
go build -o /tmp/aurumcode ./cmd/aurumcode
/tmp/aurumcode realimentacao "$@" > "$out" || status=1
exit "$status"
