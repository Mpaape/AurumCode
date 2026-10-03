#!/usr/bin/env bash
# AUR-586 acceptance (offline): tutorials and acceptance programs carry no
# volatile values and no hand-enumerated package copies.
#
# Selectors:
#   all             every check below, then both mutations
#   AC-001          the tutorial text (blocks == out/) still matches when the
#                   board count and the clock differ (out/ rewritten in a
#                   sealed copy: AUR-563/564 AC-002 stay green), and a value
#                   outside the notation (board valid: abc ...) is rejected
#   AC-002          run.sh --check fails with a clear reason when out/.imagem
#                   is missing or recorded for another tree, and passes when it
#                   matches (the message says whether the image was verified)
#   AC-003          the eight acceptance programs (430 432 434 435 459 461 467
#                   475) copy cmd internal pkg whole, never an enumerated
#                   package list, and parse; their behavioral run is in the spec
#   AC-004          no expected/*.txt and no tutorial output block holds a date,
#                   a generated tag or the board count as a literal
#   MUT-001         a literal board count put back in an operacao.md block
#                   turns AC-004 and AUR-564 AC-002 red
#   MUT-002         making --check ignore out/.imagem turns the AC-002 rejection
#                   probe red
# Exit codes: 0 holds, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-586'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
for tool in awk grep sed tar cp mktemp find sort sha256sum; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-tool:$tool"
done
for f in demo/tutoriais/_lib/tutorial.sh demo/tutoriais/_lib/normaliza.sed docs/tutorials/operacao.md \
  docs/tutorials/dados-de-analise.md Dockerfile go.mod go.sum tests/acceptance/AUR-563.sh tests/acceptance/AUR-564.sh; do
  [[ -f "$repo_root/$f" ]] || infra "missing:$f"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a586.XXXXXX")" || infra mktemp
trap 'chmod -R u+rwX "${work:?}" 2>/dev/null || true; rm -rf "${work:?}"' EXIT

readonly tutorials=(revisao skills politica-central qualquer-linguagem benchmark operacao gate excecoes auditoria-sarif reaproveitamento sast sbom-dependency-track assinatura xbom dados-de-analise)

# clone DEST: a sealed copy with what the tutorials' checks read. Tutorial
# scratch state (.estado) is left out; cmd internal pkg are needed because the
# tree identity of out/.imagem is computed from them.
clone() {
  local dest="$1"
  mkdir -p "$dest"
  (cd "$repo_root" && tar --exclude=.estado -cf - demo docs tests/acceptance Dockerfile go.mod go.sum cmd internal pkg) | tar -x -C "$dest"
  chmod -R u+w -- "$dest"
}

# Patterns of volatile values that must never be literal in expected/ or in an
# output block of the texts.
literal_patterns='board valid: [0-9]|analysis-data/[0-9]{8}T[0-9]{6}Z|[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z|is [0-9]+(\.[0-9]+)? days old'

# literals ROOT: prints every offending line and returns 1 when one exists.
literals() {
  local root="$1" hits="$work/hits.txt"
  : > "$hits"
  grep -rnE -- "$literal_patterns" "$root"/demo/tutoriais/*/expected >> "$hits" || true
  local md
  for md in "$root"/docs/tutorials/*.md; do
    # only the output blocks (<!-- saida: ... --> then the fence), not prose
    awk -v f="$md" '
      /^<!-- saida: / { armed = 1; next }
      armed && !open && /^```/ { open = 1; next }
      open && /^```/ { open = 0; armed = 0; next }
      open { print f ":" NR ":" $0 }
    ' "$md" | grep -E -- "$literal_patterns" >> "$hits" || true
  done
  if [[ -s "$hits" ]]; then cat "$hits" >&2; return 1; fi
}

ac004() {
  literals "$repo_root" || fail 'literal-volatil-em-expected-ou-bloco'
  printf '%s/AC-004/ok (nenhum expected/ nem bloco de saida com data, tag gerada ou contagem do board literais)\n' "$card"
}

# run_acceptance ROOT NAME: runs a copied acceptance AC-002 in the sealed copy.
run_acceptance() { bash "$1/tests/acceptance/$2" AC-002 >"$work/acc.out" 2>&1; }

ac001() {
  local c="$work/ac001"
  clone "$c"
  # control: the copy as it is passes both
  run_acceptance "$c" AUR-563.sh || { cat "$work/acc.out" >&2; fail 'AC-001/controle-563'; }
  run_acceptance "$c" AUR-564.sh || { cat "$work/acc.out" >&2; fail 'AC-001/controle-564'; }
  # another board count and another clock in out/ (the product output varies;
  # the text carries the notation)
  sed -i -E 's/board valid: [0-9]+ atomic cards/board valid: 4242 atomic cards/' "$c"/demo/tutoriais/operacao/out/*.log
  sed -i -E 's#analysis-data/[0-9]{8}T[0-9]{6}Z#analysis-data/20310101T000000Z#g; s/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z/2031-01-01T00:00:00Z/g; s/is [0-9.]+ days old/is 11.5 days old/' "$c"/demo/tutoriais/dados-de-analise/out/*.log
  grep -q 'board valid: 4242 atomic cards' "$c"/demo/tutoriais/operacao/out/entrega-e-evidencia.log || fail 'AC-001/fixture-nao-aplicada'
  run_acceptance "$c" AUR-563.sh || { cat "$work/acc.out" >&2; fail 'AC-001/563-reprova-com-outro-valor'; }
  run_acceptance "$c" AUR-564.sh || { cat "$work/acc.out" >&2; fail 'AC-001/564-reprova-com-outro-valor'; }
  # a value outside the notation is not normalized and must be rejected
  sed -i -E 's/board valid: [0-9]+ atomic cards/board valid: abc atomic cards/' "$c"/demo/tutoriais/operacao/out/entrega-e-evidencia.log
  if run_acceptance "$c" AUR-564.sh; then fail 'AC-001/valor-fora-da-forma-aceito'; fi
  printf '%s/AC-001/ok (563 e 564 AC-002 verdes com outro N e outra data; valor fora da forma reprova)\n' "$card"
}

# probe_imagem ROOT: 0 when run.sh --check REJECTS an out/ without a matching
# out/.imagem (missing, then recorded for another tree) with the stated reason
# and ACCEPTS it when it matches; 1 otherwise. Prints the reason on stderr.
probe_imagem() {
  local root="$1" t=revisao run="$1/demo/tutoriais/revisao/run.sh" out
  out="$root/demo/tutoriais/$t/out/.imagem"
  [[ -f "$out" ]] || { echo 'out/.imagem ausente na copia' >&2; return 1; }
  cp "$out" "$work/imagem.keep"
  bash "$run" --check >"$work/chk.out" 2>&1 || { cat "$work/chk.out" >&2; echo 'controle: --check deveria passar com out/.imagem gravado' >&2; return 1; }
  grep -qE 'imagem (conferida|nao conferida \(sem docker\))' "$work/chk.out" || { echo 'a saida nao diz se a imagem foi conferida' >&2; return 1; }
  rm -f "$out"
  if bash "$run" --check >"$work/chk.out" 2>&1; then echo 'sem out/.imagem o --check passou' >&2; return 1; fi
  grep -q 'out/.imagem ausente' "$work/chk.out" || { echo 'motivo de out/.imagem ausente nao e claro' >&2; return 1; }
  sed 's/^tree=.*/tree=0000000000000000000000000000000000000000000000000000000000000000/' "$work/imagem.keep" > "$out"
  if bash "$run" --check >"$work/chk.out" 2>&1; then echo 'com arvore de outra imagem o --check passou' >&2; return 1; fi
  grep -q 'outra arvore' "$work/chk.out" || { echo 'motivo de arvore diferente nao e claro' >&2; return 1; }
  cp "$work/imagem.keep" "$out"
}

ac002() {
  local c="$work/ac002"
  clone "$c"
  probe_imagem "$c" || fail 'AC-002/imagem-nao-exigida'
  # a change in the product source changes the tree identity: the recorded
  # out/.imagem no longer matches
  printf '\n// mudanca\n' >> "$c/cmd/aurumcode/main.go"
  if bash "$c/demo/tutoriais/revisao/run.sh" --check >"$work/chk.out" 2>&1; then fail 'AC-002/fonte-alterada-aceita'; fi
  grep -q 'outra arvore' "$work/chk.out" || fail 'AC-002/motivo-fonte-alterada'
  printf '%s/AC-002/ok (--check rejeita out/.imagem ausente ou de outra arvore, com motivo; passa com o gravado)\n' "$card"
}

ac003() {
  local n f bad retired=0
  for n in 430 432 434 435 459 461 467 475; do
    f="$repo_root/tests/acceptance/AUR-$n.sh"
    [[ -f "$f" ]] || infra "missing:AUR-$n.sh"
    bash -n "$f" || fail "AC-003/$n/sintaxe"
    if grep -qE '^exit 69$' "$f"; then
      # aposentadoria explicita: imprime o motivo e sai 69
      bash "$f" >/dev/null 2>"$work/ret.err" && fail "AC-003/$n/aposentado-saiu-0"
      grep -q "AUR-$n/retired" "$work/ret.err" || fail "AC-003/$n/aposentadoria-sem-motivo"
      retired=$((retired + 1)); continue
    fi
    grep -qE '^  copy "\$root" cmd internal pkg$' "$f" || fail "AC-003/$n/sem-copia-inteira"
    bad="$(grep -nE '^\s*copy "\$root" (cmd/|internal/|pkg/)' "$f" || true)"
    [[ -z "$bad" ]] || { printf '%s\n' "$bad" >&2; fail "AC-003/$n/copia-enumerada"; }
    if grep -qE 'internal/documentation|cmd/regenerate-docs|internal/pipeline' "$f"; then fail "AC-003/$n/pacote-que-nao-existe"; fi
  done
  printf '%s/AC-003/ok (os oito aceites: %d aposentados com motivo, os demais copiam cmd internal pkg inteiros; execucao no container na spec)\n' "$card" "$retired"
}

mut001() {
  local c="$work/mut001"
  clone "$c"
  literals "$c" || infra 'MUT-001/controle-ja-vermelho'
  sed -i 's/^board valid: <N> atomic cards$/board valid: 585 atomic cards/' "$c/docs/tutorials/operacao.md"
  grep -q '^board valid: 585 atomic cards$' "$c/docs/tutorials/operacao.md" || fail 'MUT-001/mutacao-nao-aplicada'
  if literals "$c" 2>/dev/null; then fail 'MUT-001/AC-004-nao-reprova'; fi
  if run_acceptance "$c" AUR-564.sh; then fail 'MUT-001/564-nao-reprova'; fi
  printf '%s/MUT-001/rejected (literal de contagem reprova AC-004 e AUR-564 AC-002)\n' "$card"
}

mut002() {
  local c="$work/mut002"
  clone "$c"
  probe_imagem "$c" || fail 'MUT-002/controle-vermelho'
  sed -i 's/^  tut_check_imagem || return 1$/  : # mutante: ignora out\/.imagem/' "$c/demo/tutoriais/_lib/tutorial.sh"
  grep -q 'mutante: ignora' "$c/demo/tutoriais/_lib/tutorial.sh" || fail 'MUT-002/mutacao-nao-aplicada'
  if probe_imagem "$c" 2>/dev/null; then fail 'MUT-002/nao-reprova'; fi
  printf '%s/MUT-002/rejected (sem conferir out/.imagem a sonda de AC-002 fica vermelha)\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  MUT-001) mut001 ;;
  MUT-002) mut002 ;;
  all) ac004; ac003; ac001; ac002; mut001; mut002; printf '%s/all/pass\n' "$card" ;;
esac
