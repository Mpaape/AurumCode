#!/usr/bin/env bash
# Tutorial executavel: SAST com Semgrep (AUR-563). Veja ../README.md e docs/tutorials/sast.md.
#
#   run.sh regra-local|registry-sem-rede|nosemgrep-e-semgrepignore|origem-sast|semgrep-falha|govet-achado|govet-sem-go
#   run.sh all | --check | limpar
#
# Semgrep roda DENTRO da imagem do produto (versao igual a scanners.yml), sem
# rede e com o modelo falso. O aurumcode resolve "semgrep" pelo PATH: nao ha
# --semgrep-bin; o caso semgrep-falha poe um semgrep falso na frente do PATH.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(regra-local registry-sem-rede nosemgrep-e-semgrepignore origem-sast semgrep-falha govet-achado govet-sem-go)

# 1. Regra local, offline: o pacote de regras e um arquivo do proprio repositorio.
caso_regra_local() {
  tut_repo regra-local repo-exemplo/base repo-exemplo/eval
  aurum review --base main
  expect_rc 3 "a regra local achou eval() e o gate de SAST reprovou, sem rede"
}

# 2. Pacote do registry (p/...): sem rede nao ha varredura confiavel.
caso_registry_sem_rede() {
  tut_repo registry-sem-rede repo-exemplo/base repo-exemplo/eval repo-exemplo/registry
  aurum review --base main
  expect_rc 1 "sem rede, o pacote p/security-audit nao baixa: inconclusivo, gate.inconclusive block reprova"
}

# 3. nosemgrep e .semgrepignore: respeitados sem politica, ignorados sob politica.
caso_nosemgrep_e_semgrepignore() {
  echo "--- sem politica (config do proprio repositorio): nosemgrep e .semgrepignore valem"
  tut_repo nosemgrep-sem-politica repo-exemplo/base repo-exemplo/nosem
  aurum review --base main
  expect_rc 0 "sem politica, o nosemgrep e o .semgrepignore silenciam os dois eval()"
  echo "--- sob politica central: o mesmo codigo, o autor nao consegue silenciar"
  tut_repo nosemgrep-sob-politica repo-exemplo/sem-config repo-exemplo/nosem
  TUT_POLICY=politica
  aurum review --base main --politica /policy
  expect_rc 3 "sob politica, os dois eval() aparecem apesar de nosemgrep e .semgrepignore"
  TUT_POLICY=
}

# 4. Origem sast no gate: parecer, auditoria e gate.sources.
caso_origem_sast() {
  tut_repo origem-sast repo-exemplo/sem-config repo-exemplo/eval
  TUT_POLICY=politica
  aurum review --base main --politica /policy --auditoria auditoria.json
  expect_rc 3 "a politica conta o achado de origem sast"
  python3 - "$TUT_WORK/auditoria.json" <<'PY'
import json, sys
a = json.load(open(sys.argv[1]))
for f in a.get("blocking_findings", []):
    print("auditoria blocking_findings: rule_id=%s path=%s line=%s origin=%s" % (f["rule_id"], f["path"], f["line"], f["origin"]))
PY
  echo "--- gate.sources sem sast"
  TUT_POLICY=politica-so-sast
  aurum review --base main --politica /policy
  expect_rc 0 "com gate.sources sem sast, o achado do Semgrep e publicado mas nao reprova o gate"
  TUT_POLICY=
}

# 5. Falha: semgrep sai != 0, sem relatorio => inconclusivo, nunca "limpo".
caso_semgrep_falha() {
  tut_repo semgrep-falha-block repo-exemplo/base repo-exemplo/eval
  TUT_ENVS=(-e PATH=/fixtures/fake-semgrep:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin)
  echo "--- gate.inconclusive: block"
  mkdir -p "$TUT_WORK/.aurumcode"
  { cat "$HERE/repo-exemplo/base/.aurumcode/config.yml"; printf 'gate:\n  fail_on: [error]\n  inconclusive: block\n'; } > "$TUT_WORK/.aurumcode/config.yml"
  aurum review --base main
  expect_rc 1 "semgrep falhou: inconclusivo e, com block, o gate reprova"
  echo "--- gate.inconclusive: warn"
  { cat "$HERE/repo-exemplo/base/.aurumcode/config.yml"; printf 'gate:\n  fail_on: [error]\n  inconclusive: warn\n'; } > "$TUT_WORK/.aurumcode/config.yml"
  aurum review --base main
  expect_rc 0 "semgrep falhou: com warn o alerta e publicado e o comando sai 0, sem aprovar"
  TUT_ENVS=()
}

# 6. Linter real (engine govet, categoria lint): so a linha que o PR adicionou
# vira achado, e os segredos do processo do aurumcode nao chegam ao go.
caso_govet_achado() {
  tut_repo govet-achado repo-exemplo/lint-base repo-exemplo/lint-erro
  TUT_ENVS=(-e PATH=/fixtures/fake-go:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
    -e LLM_API_KEY=segredo-de-demonstracao -e GITHUB_TOKEN=segredo-de-demonstracao)
  aurum review --base main
  expect_rc 3 "go vet achou o printf errado na linha adicionada; o achado antigo de legado.go nao entra"
  cat "$TUT_WORK/ambiente-do-go.txt"
  TUT_ENVS=()
}

# 7. Falha: a imagem do produto nao traz o Go => lint_unavailable, nunca "limpo".
caso_govet_sem_go() {
  tut_repo govet-sem-go repo-exemplo/lint-base repo-exemplo/lint-erro
  aurum review --base main
  expect_rc 1 "sem go no PATH: inconclusivo (lint_unavailable) e o gate reprova; nada e instalado"
}

tut_main "$@"
