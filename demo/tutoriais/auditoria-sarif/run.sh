#!/usr/bin/env bash
# Tutorial executavel: auditoria e SARIF (AUR-562). Veja ../README.md e docs/tutorials/auditoria-sarif.md.
#
#   run.sh auditoria-reprovado|sarif-campos|sarif-inconclusivo|canario-de-redacao|upload-workflow|falha-caminho-invalido
#   run.sh all | --check | limpar
#
# O envio ao Code Scanning (upload-sarif) NAO e executado: nao ha runner nem GitHub aqui.
# O caso upload-workflow confere o workflow do chamador contra o review.yml real.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(auditoria-reprovado sarif-campos sarif-inconclusivo canario-de-redacao upload-workflow falha-caminho-invalido)

SHA_REVISADO=3333333333333333333333333333333333333333
SHA_WORKFLOW=4444444444444444444444444444444444444444
IDENT=(-e "GITHUB_SHA=$SHA_REVISADO" -e "AURUMCODE_WORKFLOW_SHA=$SHA_WORKFLOW" -e GITHUB_REPOSITORY=OWNER/REPO)

# 1. Auditoria de uma revisao que reprova: cada campo.
caso_auditoria_reprovado() {
  tut_repo auditoria-reprovado repo-exemplo/base repo-exemplo/segredo
  tgit remote add origin https://git.example.com/OWNER/REPO.git
  TUT_FIXTURE=fixture-erro.json; TUT_POLICY=politica; TUT_ENVS=("${IDENT[@]}")
  aurum review --base main --politica /policy --modelo modelo-demo --auditoria /work/auditoria.json
  expect_rc 3 "a revisao reprova e a auditoria e escrita mesmo assim"
  echo "--- campos da auditoria"
  python3 - "$TUT_WORK/auditoria.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print("chaves:", ", ".join(d.keys()))
print("policy_digest: %d caracteres hex" % len(d["policy_digest"]))
for k in ("workflow_sha", "repo", "reviewed_sha", "model", "verdict"):
    print("%s: %s" % (k, d[k]))
print("gate.decision: %s" % d["gate"]["decision"])
for b in d["blocking_findings"]:
    print("blocking_findings: %s %s:%s %s origin=%s" % (b["rule_id"], b["path"], b["line"], b["severity"], b.get("origin")))
print("exceptions_applied: %s" % d["exceptions_applied"])
print("coverage.complete: %s" % d["coverage"]["complete"])
PY
}

# 2. SARIF do mesmo caso.
caso_sarif_campos() {
  tut_repo sarif-campos repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-erro.json; TUT_POLICY=politica
  aurum review --base main --politica /policy --sarif /work/revisao.sarif
  expect_rc 3 "o SARIF e escrito com o gate reprovado"
  echo "--- campos do SARIF"
  python3 - "$TUT_WORK/revisao.sarif" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
r = d["runs"][0]
print("version: %s" % d["version"])
print("tool.driver: %s" % r["tool"]["driver"]["name"])
print("executionSuccessful: %s" % r["invocations"][0]["executionSuccessful"])
for x in r["results"]:
    loc = x["locations"][0]["physicalLocation"]
    print("result: %s level=%s %s:%s origin=%s" % (x["ruleId"], x["level"], loc["artifactLocation"]["uri"], loc["region"]["startLine"], x.get("properties", {}).get("origin")))
    print("partialFingerprints: %s" % ", ".join(x["partialFingerprints"].keys()))
PY
}

# 3. Revisao inconclusiva: auditoria e SARIF dizem isso.
caso_sarif_inconclusivo() {
  tut_repo sarif-inconclusivo repo-exemplo/base repo-exemplo/gerado
  TUT_FIXTURE=fixture-vazia.json; TUT_POLICY=politica-bloqueia
  aurum review --base main --politica /policy --auditoria /work/auditoria.json --sarif /work/revisao.sarif
  expect_rc 1 "inconclusivo em block reprova"
  python3 - "$TUT_WORK/auditoria.json" "$TUT_WORK/revisao.sarif" <<'PY'
import json, sys
a = json.load(open(sys.argv[1])); s = json.load(open(sys.argv[2]))
print("auditoria gate: decision=%s reason=%s" % (a["gate"]["decision"], a["gate"]["reason"]))
print("auditoria coverage.complete: %s" % a["coverage"]["complete"])
inv = s["runs"][0]["invocations"][0]
print("sarif executionSuccessful: %s" % inv["executionSuccessful"])
print("sarif notificacoes: %s" % [n["message"]["text"] for n in inv.get("toolExecutionNotifications", [])])
PY
}

# 4. Canario de redacao: um valor sigiloso que o modelo "vazaria" nao chega a nenhum destino.
caso_canario_de_redacao() {
  tut_repo canario-de-redacao repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-canario.json; TUT_POLICY=politica
  TUT_ENVS=(-e AURUM_SECRET_CANARY=CANARIO-LOCAL-7f3a91c2)
  aurum review --base main --politica /policy --auditoria /work/auditoria.json --sarif /work/revisao.sarif
  expect_rc 3 "a revisao reprova"
  echo "--- o canario aparece em algum destino?"
  if grep -q 'CANARIO-LOCAL-7f3a91c2' <<<"$LAST_OUT"; then echo "ERRO: canario na saida"; return 1; fi
  echo "stdout/stderr: canario ausente"
  local f
  for f in auditoria.json revisao.sarif; do
    if grep -q 'CANARIO-LOCAL-7f3a91c2' "$TUT_WORK/$f"; then echo "ERRO: canario em $f"; return 1; fi
    echo "$f: canario ausente"
  done
  grep -q 'REDACTED' "$TUT_WORK/revisao.sarif" && echo "revisao.sarif: o valor foi trocado por [REDACTED]"
  echo "RESULTADO: o canario nao vazou para saida, auditoria nem SARIF"
}

# 5. Upload para o Code Scanning pelo job do chamador: conferencia, nao execucao.
caso_upload_workflow() {
  local wf="$HERE/workflow-chamador.yml" reuse="$REPO_ROOT/.github/workflows/review.yml"
  echo "workflow do chamador: workflow-chamador.yml"
  grep -q 'security-events: write' "$wf" && echo "job upload-sarif: concede security-events: write a si mesmo"
  if awk '/^  review:/{r=1;next} /^  upload-sarif:/{r=0} r&&/security-events/' "$wf" | grep -q .; then echo "ERRO: o job review pede security-events"; return 1; fi
  echo "job review: nao pede security-events (um reutilizavel nao concede permissao ao chamador)"
  if grep -qE '^[[:space:]]*(- )?uses: github/codeql-action/upload-sarif' "$reuse"; then echo "ERRO: review.yml chama upload-sarif"; return 1; fi
  echo "review.yml: nao chama upload-sarif; so publica o artefato"
  grep -q 'name: aurumcode-sarif-${{ github.event.pull_request.number }}' "$reuse" && grep -q 'name: aurumcode-sarif-${{ github.event.pull_request.number }}' "$wf" && echo "nome do artefato: igual no review.yml e no download do chamador"
  grep -q 'sarif_file: aurumcode-review.sarif' "$wf" && grep -q 'aurumcode-review.sarif' "$reuse" && echo "arquivo: aurumcode-review.sarif nos dois"
  grep -q 'head.repo.full_name == github.repository' "$wf" && echo "PR de fork: o job de upload e pulado"
  echo "NAO EXECUTADO AQUI: o upload ao Code Scanning (nao ha runner nem GitHub neste ambiente)"
}

# Falha (AUR-568): o artefato pedido nao pode ser gravado. Nunca termina como sucesso.
caso_falha_caminho_invalido() {
  tut_repo falha-caminho-invalido repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-vazia.json; TUT_POLICY=politica-bloqueia
  echo "--- gate.inconclusive: block, auditoria em diretorio inexistente"
  aurum review --base main --politica /policy --auditoria /work/nao-existe/auditoria.json
  expect_rc 1 "block: a revisao reprova com audit_write_failed; a auditoria nao existe"
  [ ! -e "$TUT_WORK/nao-existe/auditoria.json" ] && echo "o arquivo de auditoria nao existe"
  echo "--- sem gate, SARIF cujo pai e um arquivo"
  echo x > "$TUT_WORK/arquivo-regular"
  TUT_POLICY=
  aurum review --base main --sarif /work/arquivo-regular/revisao.sarif
  expect_rc 1 "sem gate: exit diferente de 0 e a mensagem nomeia o caminho"
  echo "--- caminho gravavel: comportamento inalterado"
  aurum review --base main --auditoria /work/auditoria.json
  expect_rc 0 "caminho gravavel: exit 0 e a auditoria existe"
  [ -s "$TUT_WORK/auditoria.json" ] && echo "o arquivo de auditoria existe"
}

tut_main "$@"
