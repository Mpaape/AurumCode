#!/usr/bin/env bash
# Tutorial executavel: qualquer linguagem (AUR-564). Veja ../README.md e docs/tutorials/qualquer-linguagem.md.
#
#   run.sh repo-poliglota|arquivo-sem-gramatica|binario-e-gerado|politica-terraform|apelidos-e-instrucoes|falha-extensao-desconhecida
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(repo-poliglota arquivo-sem-gramatica binario-e-gerado politica-terraform apelidos-e-instrucoes falha-extensao-desconhecida txt-com-segredo)

# Imprime as linhas do prompt (AURUMCODE_PROMPT_CAPTURE) que casam com cada trecho
# literal pedido; "AUSENTE" se o trecho nao esta la.
mostra_prompt() {
  local t
  for t in "$@"; do
    if grep -qF -- "$t" "$TUT_WORK/prompt.txt"; then echo "prompt: $t"; else echo "prompt: AUSENTE: $t"; fi
  done
}

# 1. Repositorio poliglota: cada arquivo e classificado por gramatica e o
# contexto estrutural entra no prompt. O prompt e capturado para mostrar o trecho.
caso_repo_poliglota() {
  tut_repo repo-poliglota repo-exemplo/base repo-exemplo/poliglota
  TUT_FIXTURE=fixture-vazia.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main
  expect_rc 0 "revisao do repositorio poliglota concluida"
  echo "--- trecho do prompt que o modelo recebeu: secao Languages"
  sed -n '/^## Languages$/,/^## Code changes$/p' "$TUT_WORK/prompt.txt" | grep -E '^- [a-z_]+: [0-9]+ files$' | sort
  echo "--- trecho do prompt: contexto estrutural capturado (simbolos extraidos das arvores)"
  grep -F -A1 '## Codebase context' "$TUT_WORK/prompt.txt" | sed -n '1,2p'
  # conclusao deste script: cada simbolo abaixo so existe no codigo daquela linguagem
  local par
  for par in java:contarJava c_sharp:SaudarCsharp kotlin:saudarKotlin php:saudarPhp ruby:saudar_ruby hcl:logs dockerfile:alpine; do
    if grep -F -A1 '## Codebase context' "$TUT_WORK/prompt.txt" | grep -qF "\"${par#*:}\""; then
      echo "simbolo da gramatica ${par%%:*}: ${par#*:} esta no contexto"
    else
      echo "ERRO: simbolo da gramatica ${par%%:*}: ${par#*:} ausente do contexto"; return 1
    fi
  done
  mostra_prompt '### File: src/main/java/demo/Greeter.java' '### File: src/main/kotlin/Greeter.kt' \
    '### File: infra.tf' '### File: Dockerfile' '### File: .github/workflows/ci.yml'
}

# 2. Arquivo sem gramatica: vai ao modelo como texto e e declarado; sozinho nao
# torna a revisao inconclusiva, mesmo com gate.inconclusive: block.
caso_arquivo_sem_gramatica() {
  tut_repo arquivo-sem-gramatica repo-exemplo/base-gate repo-exemplo/mudanca-sem-gramatica
  TUT_FIXTURE=fixture-vazia.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main
  expect_rc 0 "arquivo sem gramatica declarado, revisao completa, gate.inconclusive: block nao dispara"
  mostra_prompt '### File: notas.zzqx' '+mas e texto simples e precisa ser lido pelo modelo'
}

# 3. Binario e gerado ficam fora do alcance e retem a aprovacao, sem e com gate.
caso_binario_e_gerado() {
  tut_repo binario-e-gerado repo-exemplo/base repo-exemplo/mudanca-binario-gerado
  TUT_FIXTURE=fixture-vazia.json
  echo "--- sem gate: o parecer deixa de ser Approve, o exit continua 0"
  aurum review --base main
  expect_rc 0 "sem gate, exit 0 mas o veredito nao e Approve (arquivos fora do alcance)"
  echo "--- com gate.inconclusive: block, os mesmos arquivos"
  tut_repo binario-e-gerado-gate repo-exemplo/base-gate repo-exemplo/mudanca-binario-gerado
  aurum review --base main
  expect_rc 1 "com gate.inconclusive: block, cobertura parcial reprova"
}

# 4. Achado de politica em Terraform bloqueia como em Go.
caso_politica_terraform() {
  tut_repo politica-terraform repo-exemplo/base-terraform repo-exemplo/mudanca-terraform
  TUT_FIXTURE=fixture-terraform.json
  aurum review --base main
  expect_rc 3 "achado citando a regra da skill de seguranca em main.tf bloqueou (gate.fail_on: high)"
}

# 5. Apelidos: escopo por caminho (applyTo) e selecao por SKILL.md/languages (AUR-565).
caso_apelidos_e_instrucoes() {
  tut_repo apelidos-e-instrucoes repo-exemplo/base-instrucoes repo-exemplo/mudanca-kt-tf
  TUT_FIXTURE=fixture-vazia.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main
  expect_rc 0 "revisao feita"
  mostra_prompt 'kotlin.md (applyTo: **/*.kt)' 'Em Kotlin, prefira val a var e evite o operador !!.' \
    'terraform.md (applyTo: **/*.tf)' 'Em Terraform, todo bucket declara acl privada.'
  if grep -qF 'java.md (applyTo' "$TUT_WORK/prompt.txt"; then echo "ERRO: java.md chegou ao modelo"; return 1; fi
  echo "prompt: java.md (applyTo: **/*.java) NAO chegou: a mudanca nao toca .java"
  # AUR-565: a skill de diretorio com languages: [kt] e selecionada pelo apelido (kt = kotlin).
  if ! grep -qF 'SKILL-POR-LINGUAGEM-KOTLIN' "$TUT_WORK/prompt.txt"; then echo "ERRO: a skill estilo-kotlin nao foi lida"; return 1; fi
  echo "prompt: a skill .aurumcode/skills/estilo-kotlin/SKILL.md (languages: [kt]) FOI lida: o apelido kt selecionou a gramatica kotlin"
}

# Falha: extensao desconhecida com conteudo de codigo ainda e revisada.
caso_falha_extensao_desconhecida() {
  tut_repo falha-extensao-desconhecida repo-exemplo/base repo-exemplo/mudanca-extensao
  TUT_FIXTURE=fixture-extensao.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main --fail-on error
  expect_rc 3 "o achado em script.zzqx (sem gramatica) reprovou: extensao desconhecida nao esconde codigo"
  mostra_prompt '### File: script.zzqx' '+    String senha = "hunter2";'
}

# AUR-574: arquivo .txt com segredo chega ao modelo. O runtime escolhe a gramatica
# vimdoc pela extensao generica .txt; isso nao o torna documentacao: o arquivo fica
# em "Code Changes" e e contado como arquivo de codigo da revisao.
caso_txt_com_segredo() {
  tut_repo txt-com-segredo repo-exemplo/base repo-exemplo/mudanca-txt-segredo
  TUT_FIXTURE=fixture-txt-segredo.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main --fail-on error
  expect_rc 3 "o achado em config/tokens.txt reprovou: arquivo .txt e revisado, nao descartado como documentacao"
  mostra_prompt '### File: config/tokens.txt' '+DEMO_API_TOKEN=[REDACTED]' '- Code files in this diff: 1'
  if grep -qF 'Documentation files excluded' "$TUT_WORK/prompt.txt" && grep -qF 'config/tokens.txt' <(sed -n '/Documentation files excluded/,$p' "$TUT_WORK/prompt.txt"); then
    echo "ERRO: config/tokens.txt foi excluido como documentacao"; return 1
  fi
  echo "prompt: config/tokens.txt NAO esta na lista de documentacao excluida"
}

tut_main "$@"
