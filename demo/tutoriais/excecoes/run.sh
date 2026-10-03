#!/usr/bin/env bash
# Tutorial executavel: excecoes aprovadas (AUR-562). Veja ../README.md e docs/tutorials/excecoes.md.
#
#   run.sh excecao-valida|excecao-vencida|nao-casa|excecao-do-repo-ignorada|falha-sem-dono
#   run.sh all | --check | limpar
#
# A identidade do repositorio vem do remoto origin (placeholder OWNER/REPO, host reservado).
# As datas de validade sao fixas (2099-12-31 e 2024-01-01), entao o resultado nao depende do dia.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(excecao-valida excecao-vencida nao-casa excecao-do-repo-ignorada falha-sem-dono)

repo() { # caso base overlay
  tut_repo "$1" "$2" "${3:-repo-exemplo/segredo}"
  tgit remote add origin https://git.example.com/OWNER/REPO.git
}

# 1. Excecao valida: o achado vira "aceito por excecao" e a auditoria a registra.
caso_excecao_valida() {
  repo excecao-valida repo-exemplo/base
  TUT_POLICY=politica-sem-excecao
  echo "--- sem excecao: o achado reprova"
  aurum review --base main --politica /policy
  expect_rc 3 "sem excecao, o warning cruza o limiar medium"
  echo "--- com excecao (dono, motivo, validade)"
  TUT_POLICY=politica-valida
  aurum review --base main --politica /policy --auditoria /work/auditoria.json
  expect_rc 0 "a excecao valida tira o achado do gate"
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("exceptions_applied:", json.dumps(d["exceptions_applied"], ensure_ascii=False)); print("gate:", d["gate"]["decision"])' "$TUT_WORK/auditoria.json"
}

# 2. Excecao vencida: deixa de valer sozinha.
caso_excecao_vencida() {
  repo excecao-vencida repo-exemplo/base
  TUT_POLICY=politica-vencida
  aurum review --base main --politica /policy --auditoria /work/auditoria.json
  expect_rc 3 "a excecao vencida nao vale: o achado volta a reprovar"
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("exceptions_applied:", json.dumps(d["exceptions_applied"])); print("blocking:", [b["rule_id"] for b in d["blocking_findings"]])' "$TUT_WORK/auditoria.json"
}

# 3. A excecao cobre o achado exato: outro caminho, outro repositorio e repositorio sem origin nao casam.
caso_nao_casa() {
  repo nao-casa-caminho repo-exemplo/base
  TUT_POLICY=politica-outro-caminho
  echo "--- outro caminho"
  aurum review --base main --politica /policy
  expect_rc 3 "excecao para outro caminho nao casa"
  repo nao-casa-repo repo-exemplo/base
  TUT_POLICY=politica-outro-repo
  echo "--- outro repositorio"
  aurum review --base main --politica /policy
  expect_rc 3 "excecao de outro repositorio nao casa"
  echo "--- repositorio sem remoto origin: identidade nao confirmada"
  tut_repo nao-casa-sem-origin repo-exemplo/base repo-exemplo/segredo
  TUT_POLICY=politica-valida
  aurum review --base main --politica /policy
  expect_rc 3 "sem identidade confirmada, nenhuma excecao casa (falha fechado)"
}

# 4. Excecao declarada pelo proprio repositorio e ignorada sob politica.
caso_excecao_do_repo_ignorada() {
  tut_repo excecao-do-repo-ignorada repo-exemplo/base-com-excecao repo-exemplo/segredo
  tgit remote add origin https://git.example.com/OWNER/REPO.git
  echo "--- so o repositorio (sem politica): a excecao dele vale"
  aurum review --base main
  expect_rc 0 "sem politica, o gate e a excecao do proprio repositorio valem"
  echo "--- sob politica: a excecao do repositorio e ignorada"
  TUT_POLICY=politica-sem-excecao
  aurum review --base main --politica /policy
  expect_rc 3 "sob politica, a excecao do repositorio nao vale"
}

# Falha: excecao sem dono e erro de carga, antes de qualquer chamada ao modelo.
caso_falha_sem_dono() {
  repo falha-sem-dono repo-exemplo/base
  TUT_POLICY=politica-sem-dono
  aurum review --base main --politica /policy
  expect_rc 1 "excecao sem dono invalida a politica inteira (falha fechado)"
}

tut_main "$@"
