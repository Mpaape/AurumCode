#!/usr/bin/env bash
# Tutorial executavel: segredos com gitleaks. Veja ../README.md e docs/tutorials/segredos.md.
#
#   run.sh segredo-no-diff|segredo-so-no-historico|allow-sob-politica|ignore-sob-politica|binario-ausente
#   run.sh all | --check | limpar
#
# O gitleaks roda DENTRO da imagem do produto (copiado da imagem fixada por
# digest em scanners.yml), sem rede e com o modelo falso. O token de exemplo
# tem forma de token GitHub, mas so existe em tempo de execucao: e montado por
# concatenacao aqui e escrito no repositorio descartavel de .estado/. Nenhum
# arquivo rastreado carrega um literal com forma de credencial, e nenhuma
# saida (out/) carrega o valor.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

# Datas fixas: os ids de commit do repositorio descartavel saem iguais a cada
# execucao, e o commit citado no achado pode ser conferido em expected/.
export GIT_AUTHOR_DATE="2026-01-01T00:00:00Z" GIT_COMMITTER_DATE="2026-01-01T00:00:00Z"

CASOS=(segredo-no-diff segredo-so-no-historico allow-sob-politica ignore-sob-politica binario-ausente)

# token de exemplo, so em tempo de execucao (forma de PAT do GitHub, valor inventado)
token() { printf '%s%s%s' "ghp" "_" "TutorialDemo0Valor0Inventado0Sem0Uso"; }

# commita_arquivo ARQUIVO CONTEUDO MENSAGEM: escreve e commita no repositorio do caso.
commita_arquivo() {
  printf '%s\n' "$2" > "$TUT_WORK/$1"
  tgit add -A
  tgit commit -q -m "$3"
}

# 1. Segredo no diff: o commit do PR adiciona um token e ele continua na arvore final.
caso_segredo_no_diff() {
  tut_repo segredo-no-diff repo-exemplo/base
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"" "configura o cliente"
  aurum review --base main
  expect_rc 3 "o gitleaks achou o token no intervalo main..HEAD e o gate de segredos reprovou"
}

# 2. Segredo so no historico do PR: adicionado num commit, removido no seguinte.
#    A arvore final esta limpa, mas o merge publica o historico: o vazamento ja aconteceu.
caso_segredo_so_no_historico() {
  tut_repo segredo-so-no-historico repo-exemplo/base
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"" "configura o cliente"
  echo "--- commit intermediario que adicionou o token: $(tgit rev-parse --short=12 HEAD)"
  commita_arquivo config.py 'API_TOKEN = os.environ["API_TOKEN"]' "le o token do ambiente"
  echo "--- a arvore final nao tem o token:"
  cat "$TUT_WORK/config.py"
  aurum review --base main
  expect_rc 3 "o token so existe num commit intermediario do PR e mesmo assim o gate reprova"
}

# 3. gitleaks:allow: vale no config do proprio repositorio, nao sob politica central.
caso_allow_sob_politica() {
  echo "--- sem politica (config do proprio repositorio): gitleaks:allow suprime"
  tut_repo allow-sem-politica repo-exemplo/base
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"  # gitleaks:allow" "token de teste"
  aurum review --base main
  expect_rc 0 "sem politica, o comentario gitleaks:allow do autor silencia o achado"
  echo "--- sob politica central: o mesmo commit, o autor nao consegue silenciar"
  tut_repo allow-sob-politica repo-exemplo/sem-config
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"  # gitleaks:allow" "token de teste"
  TUT_POLICY=politica
  aurum review --base main --politica /policy
  expect_rc 3 "sob politica, gitleaks:allow e ignorado e o achado reprova"
  TUT_POLICY=
}

# 4. .gitleaksignore: o gitleaks sempre le o da raiz (nenhuma flag desliga);
#    sob politica, a presenca do arquivo e ela mesma um achado bloqueante.
caso_ignore_sob_politica() {
  local fp
  echo "--- sem politica: o .gitleaksignore do repositorio esconde o achado"
  tut_repo ignore-sem-politica repo-exemplo/base
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"" "configura o cliente"
  fp="$(tgit rev-parse HEAD):config.py:github-pat:1"
  commita_arquivo .gitleaksignore "$fp" "ignora o achado"
  aurum review --base main
  expect_rc 0 "sem politica, a impressao digital no .gitleaksignore esconde o achado"
  echo "--- sob politica central: o .gitleaksignore vira achado"
  tut_repo ignore-sob-politica repo-exemplo/sem-config
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"" "configura o cliente"
  fp="$(tgit rev-parse HEAD):config.py:github-pat:1"
  commita_arquivo .gitleaksignore "$fp" "ignora o achado"
  TUT_POLICY=politica
  aurum review --base main --politica /policy
  expect_rc 3 "sob politica, o .gitleaksignore na raiz e um achado bloqueante"
  TUT_POLICY=
}

# 5. Falha: sem o binario gitleaks no PATH => inconclusivo, nunca "zero achados".
caso_binario_ausente() {
  tut_repo binario-ausente repo-exemplo/base
  tgit checkout -q -b feature
  commita_arquivo config.py "API_TOKEN = \"$(token)\"" "configura o cliente"
  TUT_ENVS=(-e PATH=/usr/sbin:/usr/bin:/sbin:/bin)
  aurum review --base main
  expect_rc 1 "sem gitleaks a varredura e inconclusiva (secrets_unavailable) e o gate reprova"
  TUT_ENVS=()
}

tut_main "$@"
