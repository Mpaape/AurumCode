#!/usr/bin/env bash
# Tutorial executavel: camadas (AUR-614). Veja ../README.md e docs/tutorials/camadas.md.
#
# Tres repositorios de exemplo, um por camada: security-policy/ (politica de
# seguranca, --politica), devops-policy/ (padroes de engenharia, --padroes) e
# servico/ (o repositorio revisado). Cada caso monta os dois centrais fora da
# arvore revisada (/policy e /standards) e roda o produto sobre o servico.
#
#   run.sh tres-camadas|servico-nao-afrouxa|mesmo-nome|padroes-com-gate
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(tres-camadas servico-nao-afrouxa mesmo-nome padroes-com-gate)

# padroes DIR: a camada de padroes DIR (do tutorial) montada em /standards,
# somente leitura, fora do repositorio em /work; passe --padroes /standards.
padroes() { TUT_ENVS=(-v "$HERE/$1:/standards:ro"); }

# camada_de TRECHO TEXTO: conclusao do script, impressa so se a saida do
# produto contem TRECHO (o achado); diz de que repositorio a regra veio.
camada_de() {
  if grep -qF -- "$1" <<<"$LAST_OUT"; then echo "$1: $2"; else echo "ERRO: achado ausente: $1"; return 1; fi
}

# 1. As tres camadas no mesmo parecer: cada achado cita a regra do seu
# repositorio; o da camada de padroes ganha o rotulo da camada.
caso_tres_camadas() {
  tut_repo tres-camadas servico servico-mudanca
  TUT_FIXTURE=fixture-llm.json
  TUT_POLICY=security-policy
  padroes devops-policy
  aurum review --base main --politica /policy --padroes /standards
  expect_rc 3 "a regra da seguranca bloqueia; as da engenharia e do servico orientam"
  echo "--- de onde veio cada achado (conclusao do script, lendo a saida acima)"
  camada_de "rule seguranca#seg-001-verificacao-de-tls-nunca-e-desligada" "politica de seguranca (security-policy), bloqueia pelo gate dela"
  camada_de "[padrões de engenharia]" "camada de padroes (devops-policy), rotulada pelo produto"
  camada_de "rule app#app-001-mensagem-para-a-pessoa-em-portugues" "o proprio servico (servico/.aurumcode/skills/app)"
  camada_de "[perfil padroes]" "analista do profiles.yml da devops-policy, selecionado pelo servico"
}

# 2. O servico declara gate mais frouxo e ignore, e a politica vence.
caso_servico_nao_afrouxa() {
  tut_repo servico-nao-afrouxa servico-afrouxa servico-mudanca
  TUT_FIXTURE=fixture-llm.json
  TUT_POLICY=
  padroes devops-policy
  echo "--- sem a politica: o gate e o ignore do servico valem"
  aurum review --base main --padroes /standards
  expect_rc 0 "sozinho, o servico aprova a propria mudanca"
  echo "--- com a politica: o gate e o ignore do servico sao descartados, com aviso"
  TUT_POLICY=security-policy
  aurum review --base main --politica /policy --padroes /standards
  expect_rc 3 "o servico nao consegue afrouxar a politica"
}

# 3. Uma skill com o mesmo nome nas tres camadas: vale a da politica, com aviso
# nomeando a camada que perdeu e a que venceu.
caso_mesmo_nome() {
  tut_repo mesmo-nome servico-mesmo-nome servico-mesmo-nome-mudanca
  TUT_FIXTURE=fixture-mesmo-nome.json
  TUT_POLICY=security-policy-mesmo-nome
  padroes devops-policy-mesmo-nome
  aurum review --base main --politica /policy --padroes /standards
  expect_rc 3 "a skill da politica venceu: severity error, o gate reprova"
  if grep -qF '[padrões de engenharia]' <<<"$LAST_OUT"; then
    echo "ERRO: o achado foi atribuido a camada de padroes, que perdeu"; return 1
  fi
  echo "o achado nao leva o rotulo da camada de padroes: a regra que valeu e a da politica"
}

# Falha: a camada de padroes com gate declarado e recusada (fail-closed).
caso_padroes_com_gate() {
  tut_repo padroes-com-gate servico servico-mudanca
  TUT_FIXTURE=fixture-llm.json
  TUT_POLICY=
  padroes devops-policy-com-gate
  aurum review --base main --padroes /standards
  expect_rc 1 "a camada de padroes com gate e recusada antes de qualquer chamada ao modelo"
}

tut_main "$@"
