#!/usr/bin/env bash
# AUR-587 acceptance: the site's extension guide (docs/extensao.md) cites the
# real contract of the four extension points, the example engine exists only
# in a binary built with the tag aurum_exemplo, and the extensao tutorial's
# recorded run shows the engine's finding with origin exemplo (gate line,
# audit, SARIF), the skill text in the prompt, the scanner_exemplo tool call
# in the audit transcript and the default binary refusing `engine: exemplo`.
# The sealed profile has no docker: the engine is proved with go test/go list
# under both tag sets and the tutorial by its recorded out/ and --check; the
# real run and the site build are recorded in docs/specs/AUR-587.md.
#
# Selectors:
#   all        AC-001..AC-004, MUT-001, MUT-002
#   AC-001     guide in the nav, four points, every cited name exists
#   AC-002     engine only with the tag; tutorial --check; origin exemplo
#   AC-003     skill text in the prompt; tool call in the audit transcript
#   AC-004     architecture table still read by its test; index links
#   MUT-001    dropping the tagged registration turns AC-002 RED
#   MUT-002    citing a method the contract lacks turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-587'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
readonly tut='demo/tutoriais/extensao'
for input in go.mod go.sum cmd internal pkg mkdocs.yml docs/extensao.md docs/index.md docs/architecture.md \
  docs/tutorials/extensao.md "$tut/run.sh" demo/tutoriais/_lib/tutorial.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a587.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

readonly engines_pkg='./internal/scanner/engines/'
readonly exemplo_pkg='github.com/Mpaape/AurumCode/internal/scanner/engines/exemplo'

# stage copies the whole module (never enumerated packages), the docs the
# guide test reads, and the tutorial with its framework, to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root/demo/tutoriais"
  for source in go.mod go.sum cmd internal pkg docs; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  [[ ! -f "$repo_root/Dockerfile" ]] || cp "$repo_root/Dockerfile" "$root/Dockerfile"
  cp -R "$repo_root/demo/tutoriais/_lib" "$repo_root/$tut" "$root/demo/tutoriais/"
  rm -rf "$root/$tut/.estado"
  chmod -R u+w -- "$root"
}

# go_in root log args...: runs go in root; rc is go's.
go_in() {
  local root="$1" log="$2"; shift 2
  ( cd "$root" && go "$@" ) >"$log" 2>&1
}

# deps_count root tags: how many times the example package is a dependency
# of the binary (awk, so zero matches is not a pipefail error).
deps_count() {
  ( cd "$1" && go list -buildvcs=false ${2:+-tags "$2"} -deps ./cmd/aurumcode ) >"$run_dir/deps.txt" 2>&1 || { cat "$run_dir/deps.txt" >&2; infra go-list; }
  awk -v p="$exemplo_pkg" '$0 == p { n++ } END { print n + 0 }' "$run_dir/deps.txt"
}

guide_test() { go_in "$1" "$2" test -buildvcs=false -count=1 -p 1 -v -run TestExtensionGuideCitesTheRealContract "$engines_pkg"; }

run_ac001() {
  local root="$run_dir/ac001" sec
  stage "$root"
  grep -qE '^  - Estendendo o Aurum: extensao\.md$' "$repo_root/mkdocs.yml" || fail AC-001/fora-do-nav
  for sec in '## 1. Engine de scanner' '## 2. Ferramenta de deliberação' '## 3. Skill' '## 4. Fonte de contexto (`ContextProvider`)' '## O que NÃO é ponto de extensão'; do
    grep -qF -- "$sec" "$repo_root/docs/extensao.md" || fail "AC-001/secao-ausente:$sec"
  done
  for term in 'Plugin dinâmico' 'Texto do modelo alterando o gate' 'AUR-469'; do
    grep -qF -- "$term" "$repo_root/docs/extensao.md" || fail "AC-001/nao-extensao-sem:$term"
  done
  guide_test "$root" "$run_dir/ac001.log" || { cat "$run_dir/ac001.log" >&2; fail AC-001/guia-cita-nome-inexistente; }
  grep -q '^--- PASS: TestExtensionGuideCitesTheRealContract' "$run_dir/ac001.log" || fail AC-001/teste-nao-rodou
  printf '%s/AC-001/ok (guia no nav, quatro pontos, nomes citados conferidos com go/ast)\n' "$card"
}

run_ac002() {
  local root="$run_dir/ac002" n_def n_tag
  stage "$root"
  n_def="$(deps_count "$root" "")"; n_tag="$(deps_count "$root" aurum_exemplo)"
  [[ "$n_def" == 0 ]] || fail "AC-002/binario-padrao-contem-exemplo:$n_def"
  [[ "$n_tag" == 1 ]] || fail "AC-002/binario-com-tag-sem-exemplo:$n_tag"
  go_in "$root" "$run_dir/def.log" test -buildvcs=false -count=1 -p 1 -v ./internal/scanner/engines/... \
    || { cat "$run_dir/def.log" >&2; fail AC-002/testes-padrao; }
  grep -q '^--- PASS: TestDefaultBuildRefusesExampleEngine' "$run_dir/def.log" || fail AC-002/padrao-nao-recusou
  go_in "$root" "$run_dir/tag.log" test -buildvcs=false -count=1 -p 1 -v -tags aurum_exemplo ./internal/scanner/engines/... \
    || { cat "$run_dir/tag.log" >&2; fail AC-002/testes-com-tag; }
  grep -q '^--- PASS: TestTaggedBuildRegistersExampleEngine' "$run_dir/tag.log" || fail AC-002/tag-nao-registrou
  grep -q '^--- PASS: TestRunReportsMarkedLinesDeterministically' "$run_dir/tag.log" || fail AC-002/engine-sem-achado
  bash "$root/$tut/run.sh" --check >"$run_dir/check.out" 2>&1 || { cat "$run_dir/check.out" >&2; fail AC-002/check-falhou; }
  grep -q '^CHECK OK$' "$run_dir/check.out" || fail AC-002/check-sem-ok
  grep -qF '(severidade error, limiar error, origem exemplo, secao repo)' "$root/$tut/out/engine-no-gate.log" || fail AC-002/linha-do-gate-sem-origem
  grep -qF 'auditoria blocking_findings: rule_id=exemplo:marca path=app.py line=2 origin=exemplo' "$root/$tut/out/engine-no-gate.log" || fail AC-002/auditoria-sem-origem
  grep -qF 'sarif result: exemplo:marca app.py:2 origin=exemplo' "$root/$tut/out/engine-no-gate.log" || fail AC-002/sarif-sem-origem
  grep -qF 'unknown engine "exemplo"' "$root/$tut/out/falha-binario-padrao.log" || fail AC-002/padrao-nao-recusou-no-tutorial
  grep -qx 'build_args=GO_TAGS=aurum_exemplo' "$root/$tut/out/.imagem" || fail AC-002/imagem-sem-build-args
  if grep -l '^ERRO:' "$root/$tut"/out/*.log | awk 'END{exit NR==0}'; then fail AC-002/out-com-erro; fi
  printf '%s/AC-002/ok (exemplo fora do binario padrao e dentro com a tag; --check; origem exemplo no gate, auditoria e SARIF)\n' "$card"
}

run_ac003() {
  local out="$repo_root/$tut/out"
  grep -qxF 'prompt: Uma funcao faz uma coisa so. MARCA-SKILL-EXEMPLO' "$out/skill-no-prompt.log" || fail AC-003/skill-fora-do-prompt
  grep -qF 'o prompt recebido trazia a skill de exemplo (MARCA-SKILL-EXEMPLO)' "$out/skill-no-prompt.log" || fail AC-003/fixture-nao-ecoou
  grep -qxF 'auditoria chamada: rodada=1 ferramenta=scanner_exemplo status=executed resultado=1 achado(s)' "$out/ferramenta-pedida.log" || fail AC-003/chamada-fora-do-transcript
  grep -qF 'auditoria deliberation: oferecidas=scanner_exemplo,codebase_context pedidas=scanner_exemplo' "$out/ferramenta-pedida.log" || fail AC-003/oferta-fora-do-transcript
  printf '%s/AC-003/ok (texto da skill no prompt e eco da fixture; scanner_exemplo no transcript da auditoria)\n' "$card"
}

run_ac004() {
  local root="$run_dir/ac004"
  stage "$root"
  go_in "$root" "$run_dir/arch.log" test -buildvcs=false -count=1 -p 1 -v -run TestAUR558ArchitectureDocCitesEveryInternalPackage ./cmd/aurumcode/ \
    || { cat "$run_dir/arch.log" >&2; fail AC-004/tabela-de-pacotes; }
  grep -qF '](extensao.md)' "$repo_root/docs/architecture.md" || fail AC-004/arquitetura-sem-guia
  grep -qF '](extensao.md)' "$repo_root/docs/index.md" || fail AC-004/index-sem-guia
  grep -qF '](tutorials/extensao.md)' "$repo_root/docs/index.md" || fail AC-004/index-sem-tutorial
  printf '%s/AC-004/ok (teste da tabela de pacotes verde; arquitetura e index ligam guia e tutorial)\n' "$card"
}

run_mut001() {
  local root="$run_dir/mut001" log
  stage "$root"
  rm -- "$root/internal/scanner/engines/exemplo_registro.go"
  [[ "$(deps_count "$root" aurum_exemplo)" == 0 ]] || fail MUT-001/mutante-ainda-contem-exemplo
  if go_in "$root" "$run_dir/mut001.log" test -buildvcs=false -count=1 -p 1 -v -tags aurum_exemplo -run TestTaggedBuildRegistersExampleEngine "$engines_pkg"; then
    fail MUT-001/teste-com-tag-passou-sem-engine
  fi
  grep -q 'example engine not registered' "$run_dir/mut001.log" || { cat "$run_dir/mut001.log" >&2; fail MUT-001/nao-reprovou-pelo-motivo; }
  # sem a engine, o produto nao produz a origem: o out/ sem ela reprova o --check
  log="$root/$tut/out/engine-no-gate.log"
  sed -i -e 's/origem exemplo/origem ausente/g' -e 's/origin=exemplo/origin=/g' "$log"
  if bash "$root/$tut/run.sh" --check >"$run_dir/mut001.check" 2>&1; then fail MUT-001/check-passou-sem-origem; fi
  grep -q 'DIVERGENCIA caso=engine-no-gate' "$run_dir/mut001.check" || { cat "$run_dir/mut001.check" >&2; fail MUT-001/check-sem-motivo; }
  printf '%s/MUT-001/rejected (sem o registro com tag a engine some do binario, o teste com tag e o --check ficam vermelhos)\n' "$card"
}

run_mut002() {
  local root="$run_dir/mut002"
  stage "$root"
  guide_test "$root" "$run_dir/mut002.ctl" || { cat "$run_dir/mut002.ctl" >&2; fail MUT-002/controle-vermelho; }
  printf '\nO executor chama `scanner.Scanner.Scan`.\n' >> "$root/docs/extensao.md"
  if guide_test "$root" "$run_dir/mut002.log"; then fail MUT-002/metodo-inexistente-aceito; fi
  grep -q 'scanner.Scanner.Scan' "$run_dir/mut002.log" || { cat "$run_dir/mut002.log" >&2; fail MUT-002/nao-reprovou-pelo-motivo; }
  printf '%s/MUT-002/rejected (um metodo citado que o contrato nao tem reprova AC-001)\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_ac004
    run_mut001
    run_mut002
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
