#!/usr/bin/env bash
# Tutorial executavel: estendendo o Aurum (AUR-587). Veja ../README.md,
# docs/tutorials/extensao.md e o guia docs/extensao.md.
#
#   run.sh engine-no-gate|skill-no-prompt|ferramenta-pedida|falha-binario-padrao
#   run.sh all | --check | limpar
#
# A imagem deste tutorial e o produto compilado com a tag aurum_exemplo
# (docker build --build-arg GO_TAGS=aurum_exemplo): so ela contem a engine de
# exemplo (internal/scanner/engines/exemplo). O caso de falha usa a imagem
# padrao, sem build args, que todo usuario recebe. Sem rede e com o modelo falso.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
TUT_BUILD_ARGS="GO_TAGS=aurum_exemplo"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(engine-no-gate skill-no-prompt ferramenta-pedida falha-binario-padrao)

# mostra_origens: a origem do achado na auditoria (blocking_findings) e no SARIF.
mostra_origens() {
  python3 - "$TUT_WORK/auditoria.json" "$TUT_WORK/revisao.sarif" <<'PY'
import json, sys
a = json.load(open(sys.argv[1]))
for f in a.get("blocking_findings", []):
    print("auditoria blocking_findings: rule_id=%s path=%s line=%s origin=%s" % (f["rule_id"], f["path"], f["line"], f["origin"]))
s = json.load(open(sys.argv[2]))
for x in s["runs"][0]["results"]:
    loc = x["locations"][0]["physicalLocation"]
    print("sarif result: %s %s:%s origin=%s" % (x["ruleId"], loc["artifactLocation"]["uri"], loc["region"]["startLine"], x.get("properties", {}).get("origin")))
PY
}

# 1. A engine de exemplo declarada em quality_gates.scanners: o achado reprova
#    o gate e chega a linha do gate, a auditoria e ao SARIF com origem exemplo.
caso_engine_no_gate() {
  tut_repo engine-no-gate repo-exemplo/base repo-exemplo/marca
  aurum review --base main --auditoria /work/auditoria.json --sarif /work/revisao.sarif
  expect_rc 3 "a engine de exemplo achou a marca no diff e o gate reprovou com origem exemplo"
  mostra_origens
}

# 2. Uma skill de exemplo entra no prompt: o modelo falso ecoa a marca que viu.
caso_skill_no_prompt() {
  tut_repo skill-no-prompt repo-exemplo/base-skill repo-exemplo/marca
  TUT_FIXTURE=fixture-skill.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main
  expect_rc 0 "a skill chegou ao prompt; sem engine nem gate o exit e 0"
  if grep -qF 'Uma funcao faz uma coisa so. MARCA-SKILL-EXEMPLO' "$TUT_WORK/prompt.txt"; then
    echo "prompt: Uma funcao faz uma coisa so. MARCA-SKILL-EXEMPLO"
  else
    echo "ERRO: o texto da skill nao chegou ao prompt"; return 1
  fi
  TUT_ENVS=()
}

# 3. A engine opcional vira a ferramenta scanner_exemplo: o modelo a pede, o
#    achado conta no gate e a chamada fica no transcript da auditoria.
caso_ferramenta_pedida() {
  tut_repo ferramenta-pedida repo-exemplo/base-ferramenta repo-exemplo/marca
  TUT_FIXTURE=fixture-ferramenta.json
  aurum review --base main --auditoria /work/auditoria.json
  expect_rc 3 "o modelo pediu scanner_exemplo, o achado contou no gate com origem exemplo"
  python3 - "$TUT_WORK/auditoria.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))["deliberation"]
print("auditoria deliberation: oferecidas=%s pedidas=%s rodadas=%d desfecho=%s" % (
    ",".join(d["offered"]), ",".join(d["requested"]), d["rounds"], d["outcome"]))
for c in d["calls"]:
    print("auditoria chamada: rodada=%d ferramenta=%s status=%s resultado=%s" % (c["round"], c["tool"], c["status"], c["result"]))
PY
}

# Falha: o binario padrao (imagem sem build args) nao contem a engine de
# exemplo; `engine: exemplo` e recusado ao ler a configuracao, antes da revisao.
caso_falha_binario_padrao() {
  tut_image_padrao
  tut_repo falha-binario-padrao repo-exemplo/base repo-exemplo/marca
  TUT_RUN_IMAGE="$TUT_IMAGE_PADRAO"
  aurum review --base main
  TUT_RUN_IMAGE=
  expect_rc 1 "o binario padrao recusa engine: exemplo como engine desconhecida"
}

tut_main "$@"
