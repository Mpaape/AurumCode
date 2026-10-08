#!/usr/bin/env bash
# Tutorial executavel: revisao (AUR-561). Veja ../README.md e docs/tutorials/revisao.md.
#
#   run.sh primeira-revisao|sem-provedor|com-provedor|fix|pr-workflow|falha-nao-revisado|modelo-pondera|achado-refutado
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(primeira-revisao sem-provedor com-provedor fix pr-workflow falha-nao-revisado modelo-pondera achado-refutado)

# 1. Primeira revisao local: uma troca de mensagem, modelo (fixture) sem achados.
caso_primeira_revisao() {
  tut_repo primeira-revisao repo-exemplo/base repo-exemplo/limpa
  TUT_FIXTURE=fixture-vazia.json
  aurum review --base main
  expect_rc 0 "primeira revisao concluida sem achados"
}

# 2. Sem provedor: so a analise deterministica, declarada como tal.
caso_sem_provedor() {
  tut_repo sem-provedor repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=none
  aurum review --base main --seguranca --fail-on error
  expect_rc 3 "sem provedor, a analise deterministica achou o segredo e reprovou"
  echo "--- sem provedor e com --exigir-qualidade: a ausencia do modelo e falha"
  aurum review --base main --seguranca --exigir-qualidade
  expect_rc 1 "com --exigir-qualidade, sem provedor o comando falha"
}

# 3. Com provedor (fixture): o achado cita a regra do catalogo.
caso_com_provedor() {
  tut_repo com-provedor repo-exemplo/base repo-exemplo/segredo
  aurum review --base main --fail-on error
  expect_rc 3 "achado do modelo citando security/hardcoded-secret reprovou"
}

# 4. fix: aplica uma sugestao como patch.
caso_fix() {
  tut_repo fix repo-exemplo/base repo-exemplo/segredo
  echo "--- antes"; sed -n '6p' "$TUT_WORK/app.go"
  cp "$HERE/fix/sugestoes.json" "$TUT_WORK/sugestoes.json"
  printf '$ aurumcode fix --file sugestoes.json > fix.patch\n'
  set +e; aurum_raw -- fix --file sugestoes.json > "$TUT_WORK/fix.patch"; LAST_RC=$?; set -e
  echo "exit_code=$LAST_RC"
  expect_rc 0 "fix imprimiu o patch (nada foi escrito no repositorio)"
  cat "$TUT_WORK/fix.patch"
  git -C "$TUT_WORK" apply --check fix.patch && echo "git apply --check: o patch aplica"
  git -C "$TUT_WORK" apply fix.patch
  echo "--- depois"; sed -n '6p' "$TUT_WORK/app.go"
  # a mesma sugestao agora esta velha (o current_code ja nao existe): exit 1, nenhum patch
  aurum fix --file sugestoes.json
  expect_rc 1 "sugestao velha recusada, sem patch"
}

# 5. Revisao de PR pelo workflow reutilizavel. O que roda aqui e a conferencia do
# workflow do chamador contra as entradas reais do reutilizavel e, contra um GitHub
# falso local, a revisao de um PR grande (pr_grande, abaixo).
caso_pr_workflow() {
  local wf="$HERE/workflow-chamador.yml" reuse="$REPO_ROOT/.github/workflows/review.yml" k
  echo "workflow do chamador: workflow-chamador.yml"
  grep -q '^  pull_request:' "$wf" && echo "gatilho: pull_request"
  grep -qE 'uses: OWNER/AurumCode/\.github/workflows/review\.yml@[0-9a-f]{40}$' "$wf" && echo "uses: workflow reutilizavel fixado em SHA"
  # cada entrada em with: existe em workflow_call.inputs; cada secret existe em workflow_call.secrets
  for k in $(awk '/^    with:/{w=1;next} w&&/^    [a-z]/{w=0} w&&/^      [a-z_]+:/{sub(":","",$1);print $1}' "$wf"); do
    grep -qE "^      $k:\$" "$reuse" || { echo "ERRO: input $k nao existe em review.yml"; return 1; }
    echo "input $k: existe em review.yml"
  done
  for k in $(awk '/^    secrets:/{w=1;next} w&&/^[^ ]/{w=0} w&&/^      [A-Z_]+:/{sub(":","",$1);print $1}' "$wf"); do
    grep -qE "^      $k:\$" "$reuse" || { echo "ERRO: secret $k nao existe em review.yml"; return 1; }
    echo "secret $k: existe em review.yml"
  done
  for k in "contents: read" "pull-requests: write" "statuses: write" "checks: read"; do
    grep -qE "^      $k\$" "$reuse" && grep -qE "^  $k\$" "$wf" || { echo "ERRO: permissao '$k' nao coberta pelo chamador"; return 1; }
    echo "permissao $k: o chamador concede o que o reutilizavel declara"
  done
  echo "--- PR grande contra um GitHub falso local"
  pr_grande
}

# Falha: arquivo nao revisado (gerado) nunca conta como aprovado; binario e caminho em ignore sao declarados ignorados, nunca parciais.
caso_falha_nao_revisado() {
  tut_repo falha-nao-revisado repo-exemplo/base repo-exemplo/gerado
  TUT_FIXTURE=fixture-vazia.json
  echo "--- sem gate: veredito 'Comment', nunca 'Approve'"
  aurum review --base main
  expect_rc 0 "sem gate, exit 0 mas o veredito nao e Approve"
  echo "--- com gate.inconclusive: block"
  tut_repo falha-nao-revisado-gate repo-exemplo/base repo-exemplo/gate-estrito repo-exemplo/gerado
  aurum review --base main
  expect_rc 1 "com gate.inconclusive: block, cobertura parcial reprova"
  echo "--- binario com gate.inconclusive: block: declarado ignorado, nao parcial"
  tut_repo falha-nao-revisado-binario repo-exemplo/base repo-exemplo/gate-estrito repo-exemplo/binario
  aurum review --base main
  expect_rc 0 "o binario e declarado ignorado, a revisao nao fica parcial e o gate em block nao reprova"
}

# 7. O modelo pondera a evidencia: o passe de seguranca e o catalogo embutido rodam antes do
# modelo e os dois achados entram no prompt como evidencias [E1] e [E2]. O modelo contesta
# uma e confirma a outra. Sem politica e sem gate.triage, so o parecer muda: nada e rebaixado.
caso_modelo_pondera() {
  tut_repo modelo-pondera repo-exemplo/base repo-exemplo/pondera
  TUT_FIXTURE=fixture-pondera.json
  aurum review --base main --seguranca
  expect_rc 0 "o relatorio mostra a origem ao lado da avaliacao do modelo (contestado e confirmado), sem gate nada muda de contagem"
}

# 8. Achado do modelo refutado pela verificacao: o modelo afirma que Validar e
# chamado sem guarda de nil, mas o metodo comeca com `if l == nil`. Antes do gate,
# o achado bloqueante vai a uma chamada de verificacao com o codigo real; a
# refutacao cita as linhas exatas do arquivo, o achado deixa de bloquear e fica
# marcado no parecer e na auditoria. Com a citacao parafraseada (que nao existe
# no arquivo), a refutacao e descartada e o achado continua bloqueando.
caso_achado_refutado() {
  tut_repo achado-refutado repo-exemplo/base repo-exemplo/refutado
  TUT_FIXTURE=fixture-refutado.json
  aurum review --base main --fail-on error --auditoria auditoria.json
  expect_rc 0 "refutado com citacao literal: o achado deixa de bloquear e fica marcado"
  echo "--- a auditoria guarda o achado refutado, com o motivo e a citacao"
  python3 -c 'import json,sys; [print(r["path"], r["line"], r["rule_id"], r["outcome"], "rebaixado" if r["demoted"] else "mantido") for r in json.load(open(sys.argv[1]))["verification"]]' "$TUT_WORK/auditoria.json"
  echo "--- citacao parafraseada: nao existe no arquivo, o achado continua bloqueando"
  TUT_FIXTURE=fixture-refutado-parafraseado.json
  aurum review --base main --fail-on error
  expect_rc 3 "citacao inexistente: a refutacao e descartada e o achado reprova"
}

# gera_pr_grande DIR: uma mudanca maior que um prompt (tres diretorios com dois
# arquivos grandes cada), gerada aqui para nao versionar centenas de KB de fixture.
gera_pr_grande() {
  local dir="$1" d f i
  for d in api banco web; do
    mkdir -p "$dir/$d"
    for f in tabela catalogo; do
      { printf 'package %s\n\n' "$d"
        for i in $(seq -w 1 1400); do
          printf 'var %s%s%s = "valor %s/%s linha %s de uma mudanca grande"\n' "$f" "$d" "$i" "$d" "$f" "$i"
        done
      } > "$dir/$d/$f.go"
    done
  done
}

# cobertura_publicada: as linhas de cobertura do review que o produto publicou no PR.
cobertura_publicada() {
  [ ! -s "$TUT_PR_LOG" ] || python3 -c '
import json,sys
for l in open(sys.argv[1]):
    corpo = json.loads(l)["corpo"].get("body") or ""
    for linha in corpo.splitlines():
        if "Review coverage" in linha or "left out" in linha or linha.startswith("  - "):
            print(linha)
' "$TUT_PR_LOG"
}

# PR grande (parte do caso pr-workflow): o GitHub (falso, local) recusa o diff por tamanho (406 too_large). A revisao
# le o mesmo intervalo main...feature do checkout verificado e, como o diff passa do
# orcamento de um prompt, revisa em lotes por diretorio, com um parecer e um gate.
pr_grande() {
  # carregado so aqui: o --check (e as copias que o aceite AUR-561 monta) so precisa de tutorial.sh
  # shellcheck source=../_lib/pr.sh
  . "$HERE/../_lib/pr.sh"
  # datas fixas: os commits (e os SHAs que a saida mostra) sao os mesmos a cada execucao
  export GIT_AUTHOR_DATE='2026-01-01T00:00:00Z' GIT_COMMITTER_DATE='2026-01-01T00:00:00Z'
  tut_repo pr-grande repo-exemplo/base
  tgit checkout -q -b feature
  gera_pr_grande "$TUT_WORK"
  tgit add -A
  tgit commit -q -m "feature: pr grande"
  tgit remote add origin https://github.com/OWNER/REPO.git
  TUT_FIXTURE=fixture-vazia.json
  TUT_POLICY=politica-lotes
  tut_pr_servidor_recusa pr-grande
  aurum_pr review --pr 7 --repo OWNER/REPO --publicar --modo-publicacao review --politica /policy
  tut_pr_log
  tut_pr_parar
  expect_rc 0 "406: o diff veio do checkout verificado, tres lotes (um por diretorio) cobriram os 6 arquivos e o gate em block aprovou"
  echo "--- teto de lotes (batches.max_batches: 1): os arquivos fora sao listados e a aprovacao e retida"
  TUT_POLICY=politica-teto
  tut_pr_servidor_recusa pr-grande-teto
  aurum_pr review --pr 7 --repo OWNER/REPO --publicar --modo-publicacao review --politica /policy
  tut_pr_log
  tut_pr_parar
  cobertura_publicada
  expect_rc 1 "no teto, a revisao fica parcial (partial_coverage) e o gate em block reprova"
  echo "--- checkout nao verificado (um arquivo nao versionado): o diff local e recusado"
  TUT_POLICY=politica-lotes
  echo rascunho > "$TUT_WORK/rascunho.txt"
  tut_pr_servidor_recusa pr-grande-sujo
  aurum_pr review --pr 7 --repo OWNER/REPO --publicar --modo-publicacao review --politica /policy
  tut_pr_log
  tut_pr_parar
  expect_rc 1 "sem checkout verificado nada e revisado nem publicado: falha fechada"
}

tut_main "$@"
