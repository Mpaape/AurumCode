#!/usr/bin/env bash
# Tutorial executavel: xBOM (AUR-563). Veja ../README.md e docs/tutorials/xbom.md.
#
#   run.sh build-bom|cbom|evidencia|catalogo|enriquecimento|tipos-documentados|falha-catalogo-invalido
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(build-bom cbom evidencia catalogo enriquecimento tipos-documentados falha-catalogo-invalido)

# jq dentro da imagem do produto (o host so tem bash, git, docker e python3).
xjq() {
  printf '$ jq %s %s\n' "$1" "$2"
  docker run --rm --network none --user "$(id -u):$(id -g)" -v "$TUT_WORK:/work:ro" -w /work \
    --entrypoint jq "$TUT_IMAGE" -r "$1" "$2"
}

# Lista componente@versao e o arquivo:linha de cada ocorrencia.
LISTA='.components[] | "\(.name)@\(.version // "-") -> \(.evidence.occurrences[0].location):\(.evidence.occurrences[0].line)"'
PROPS='.metadata.properties[] | select(.name|test("llm|dropped|catalog|candidates")) | "\(.name)=\(.value)"'

# 1. Build BOM: Actions de terceiros e FROM de Dockerfile, sem provedor.
caso_build_bom() {
  tut_repo build-bom repo-exemplo
  TUT_FIXTURE=none
  aurum xbom --type build --repo . --out build-bom.json
  expect_rc 0 "Build BOM gerado e validado (CycloneDX 1.6) sem provedor de modelo"
  xjq '.specVersion' build-bom.json
  xjq "$LISTA" build-bom.json
  xjq "$PROPS" build-bom.json
}

# 2. CBOM: algoritmos citados em codigo e configuracao, com destaque pos-quantico.
caso_cbom() {
  tut_repo cbom repo-exemplo
  TUT_FIXTURE=none
  aurum xbom --type cbom --repo . --out cbom.json
  expect_rc 0 "CBOM gerado e validado"
  xjq "$LISTA" cbom.json
  echo "--- ativos marcados como pos-quanticos (aurumcode:xbom:pqc)"
  xjq '.components[] | select(any(.properties[]?; .name=="aurumcode:xbom:pqc")) | .name' cbom.json
}

# 3. Evidencia: cada ocorrencia aponta uma linha que contem o token (conferido aqui, fora do produto).
caso_evidencia() {
  tut_repo evidencia repo-exemplo
  TUT_FIXTURE=none
  aurum xbom --type cbom --repo . --out cbom.json
  expect_rc 0 "CBOM gerado"
  echo "--- conferencia do script: a linha citada contem o token de evidencia?"
  docker run --rm --network none --user "$(id -u):$(id -g)" -v "$TUT_WORK:/work:ro" -w /work \
    --entrypoint python3 "$TUT_IMAGE" - cbom.json <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
bad = 0
for c in d["components"]:
    for o in c["evidence"]["occurrences"]:
        tok = o.get("additionalContext", "")
        lines = open(o["location"]).read().split("\n")
        ok = tok.lower() in lines[o["line"] - 1].lower()
        bad += 0 if ok else 1
        print("evidencia %s: %s:%d contem '%s'" % ("ok" if ok else "FALHOU", o["location"], o["line"], tok))
print("ocorrencias sem evidencia: %d" % bad)
sys.exit(1 if bad else 0)
PY
  LAST_RC=$?
  echo "exit_code=$LAST_RC"
  expect_rc 0 "toda ocorrencia do CBOM cita uma linha que contem o token (conferencia do script)"
}

# 4. Catalogo do repositorio contra o da politica: a politica decide sozinha.
caso_catalogo() {
  tut_repo catalogo repo-exemplo
  TUT_FIXTURE=none
  cp -R "$HERE/repo-catalogo/.aurumcode" "$TUT_WORK/"
  echo "--- so o catalogo do repositorio"
  aurum xbom --type build --repo . --out repo.json
  expect_rc 0 "catalogo do repositorio usado"
  xjq '.components[] | "\(.name) catalogo=\(.properties[]? | select(.name=="aurumcode:xbom:catalogo") | .value)"' repo.json
  xjq "$PROPS" repo.json
  echo "--- com --politica: o catalogo da politica vence e o do repositorio e ignorado com aviso"
  aurum xbom --type build --repo . --politica /fixtures/politica --out politica.json
  expect_rc 0 "catalogo da politica usado, o do repositorio ignorado"
  xjq '.components[] | "\(.name) catalogo=\(.properties[]? | select(.name=="aurumcode:xbom:catalogo") | .value)"' politica.json
  xjq "$PROPS" politica.json
}

# 5. Enriquecimento pelo modelo (fixture) e descarte do que nao tem evidencia.
caso_enriquecimento() {
  tut_repo enriquecimento repo-exemplo
  TUT_FIXTURE=fixture-llm.json
  aurum xbom --type build --repo . --out build-bom.json
  expect_rc 0 "BOM enriquecido pelo modelo (fixture) e verificado"
  xjq "$LIST_ENR" build-bom.json
  xjq "$PROPS" build-bom.json
}
LIST_ENR='.components[] | "\(.name)@\(.version // "-") -> \(.evidence.occurrences[0].location):\(.evidence.occurrences[0].line) descricao=\(.description // "-")"'

# 6. Tipos apenas documentados e tipo desconhecido.
caso_tipos_documentados() {
  tut_repo tipos-documentados repo-exemplo
  TUT_FIXTURE=none
  local t
  for t in aibom saasbom netbom; do
    aurum xbom --type "$t" --repo .
    expect_rc 2 "tipo $t e apenas documentado: exit 2 com a ancora da documentacao"
  done
  aurum xbom --type sbomx --repo .
  expect_rc 64 "tipo desconhecido: exit 64"
}

# Falha: catalogo invalido e erro, nunca volta ao embutido; nenhum arquivo parcial.
caso_falha_catalogo_invalido() {
  tut_repo falha-catalogo-invalido repo-exemplo
  TUT_FIXTURE=none
  cp -R "$HERE/repo-invalido/.aurumcode" "$TUT_WORK/"
  aurum xbom --type build --repo . --out nao-deve-existir.json
  expect_rc 2 "catalogo invalido do repositorio: exit 2, sem voltar ao catalogo embutido"
  if [ -e "$TUT_WORK/nao-deve-existir.json" ]; then echo "ERRO: arquivo parcial"; return 1; fi
  echo "RESULTADO: nenhum arquivo foi escrito (conferido pelo script)"
}

tut_main "$@"
