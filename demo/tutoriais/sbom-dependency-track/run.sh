#!/usr/bin/env bash
# Tutorial executavel: SBOM e Dependency-Track (AUR-563). Veja ../README.md e
# docs/tutorials/sbom-dependency-track.md.
#
#   run.sh up|sbom-versao-minima|upload-e-metricas|limiares|violacao-de-politica|
#          secret-ausente|timeout|projeto-por-microservico|down
#   run.sh all | --check | limpar
#
# Diferencas para os tutoriais sem servidor: o Dependency-Track roda em
# containers (compose.yml, imagens de demo/gate-corporativo/images.lock), e os
# casos que falam com ele usam --network host, porque o gate so aceita https
# ou um IP de loopback (127.0.0.1) como servidor. Os demais casos seguem sem rede.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"
# shellcheck source=../_lib/cadeia.sh
. "$HERE/../_lib/cadeia.sh"

CASOS=(up sbom-versao-minima upload-e-metricas limiares violacao-de-politica secret-ausente timeout projeto-por-microservico down)

PROJECT=aurum-tut-dtrack
ADMIN_PASS='Demo-Only-Pass-123'
TRIVY_WRAP=/fixtures/wrappers/trivy-wrap.sh
TUT_NET=host

compose() {
  DTRACK_IMAGE="$(cad_lock dtrack_apiserver)" POSTGRES_IMAGE="$(cad_lock postgres)" \
    docker compose -p "$PROJECT" -f "$HERE/compose.yml" "$@"
}

# aurum_raw sobrescrito: rede por TUT_NET (host ou none), binarios em /demo/bin,
# TUT_PRE = comando de apoio iniciado em segundo plano no MESMO container.
aurum_raw() {
  local envs=() fx="${TUT_FIXTURE:-fixture-llm.json}"
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do envs+=("$1" "$2"); shift 2; done
  [ "${1:-}" = "--" ] && shift
  [ "$fx" = none ] || envs+=(-e "AURUMCODE_LLM_FIXTURE=/fixtures/$fx")
  local ep=(--entrypoint /app/aurumcode) pre=()
  if [ -n "${TUT_PRE:-}" ]; then
    ep=(--entrypoint sh); pre=(-c "$TUT_PRE & sleep 1; exec /app/aurumcode \"\$@\"" sh)
  fi
  CAD_NET="$TUT_NET" cad_run "${envs[@]}" "${ep[@]}" -- "${pre[@]}" "$@"
}

jget() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }
dt() { python3 "$HERE/dt.py" "$@"; }
load_env() {
  [ -f "$STATE/dtrack.env" ] || { echo "ERRO: rode o caso up antes"; return 1; }
  # shellcheck disable=SC1091
  . "$STATE/dtrack.env"; export DT_AUTH="X-Api-Key: $DTRACK_API_KEY"
}
usa_config() { mkdir -p "$TUT_WORK/.aurumcode"; cp "$HERE/config/$1/.aurumcode/config.yml" "$TUT_WORK/.aurumcode/config.yml"; echo "config: $1"; }
envs_dt() { TUT_ENVS=(-e "DTRACK_API_KEY=$DTRACK_API_KEY" -e "DTRACK_PROJECT_ID=$1"); }
sbom_resumo() {
  python3 - "$TUT_WORK/sbom_app_cyclonedx.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print("sbom: CycloneDX %s, componentes: %s" % (d["specVersion"], ", ".join("%s@%s" % (c["name"], c["version"]) for c in d["components"] if c.get("version"))))
PY
}
gera_sbom() { aurum sbom --repo . --trivy-bin "$TRIVY_WRAP" | sed 's#^sbom: .*/#sbom: #'; sbom_resumo; }
dt_componentes() { # rotulo uuid
  dt GET "/api/v1/component/project/$2" | python3 -c '
import json, sys
print("dependency-track componentes (%s): %s" % (sys.argv[1], ", ".join(sorted("%s@%s" % (c["name"], c.get("version")) for c in json.load(sys.stdin))) or "nenhum"))' "$1"
}
dt_metricas() {
  dt GET "/api/v1/metrics/project/$1/current" |
    jget '"dependency-track metricas: critical=%d high=%d policyViolationsTotal=%d" % (d.get("critical",0), d.get("high",0), d.get("policyViolationsTotal",0))'
}
dt_violacoes() {
  dt GET "/api/v1/violation/project/$1" | python3 -c '
import json, sys
v = json.load(sys.stdin)
if not v: print("dependency-track violacoes: nenhuma")
for x in v:
    p = x["policyCondition"]["policy"]
    print("dependency-track violacao: politica=%s componente=%s versao=%s estado=%s" % (p["name"], x["component"]["name"], x["component"].get("version"), p["violationState"]))'
}
# dt_assenta PROJETO N: as metricas do servidor so ficam estaveis algum tempo
# DEPOIS de "processing: false" (achado do tutorial). Antes do gate, o script
# envia este mesmo SBOM pela API e espera policyViolationsTotal == N; o gate
# entao le um estado ja assentado. Sem isso o resultado depende da corrida.
dt_assenta() {
  local bom i v
  bom="$(base64 -w0 "$TUT_WORK/sbom_app_cyclonedx.json")"
  dt PUT /api/v1/bom --json "{\"project\":\"$1\",\"bom\":\"$bom\"}" >/dev/null
  for i in $(seq 1 60); do
    v="$(dt GET "/api/v1/metrics/project/$1/current" | jget 'd.get("policyViolationsTotal",0)' 2>/dev/null || echo -1)"
    [ "$v" = "$2" ] && break
    sleep 2
  done
  [ "$v" = "$2" ] || { echo "ERRO: servidor nao assentou em policyViolationsTotal=$2 (leu $v)"; return 1; }
  echo "assentamento: servidor com policyViolationsTotal=$2 antes do gate"
}
review() { aurum review --base main --fail-on error; }

# ---------------------------------------------------------------- casos
caso_up() {
  echo "== up: Dependency-Track v5 + PostgreSQL, time, chave e projetos"
  local ref name i
  mkdir -p "$STATE"; touch "$STATE/baixadas"
  for name in dtrack_apiserver postgres; do
    ref="$(cad_lock "$name")"
    case "$ref" in *@sha256:*) ;; *) echo "ERRO: $name sem digest"; return 1 ;; esac
    if ! docker image inspect "$ref" >/dev/null 2>&1; then docker pull -q "$ref" >/dev/null; echo "$name" >> "$STATE/baixadas"; fi
    echo "imagem fixada por digest (images.lock do AUR-554): $name ok"
  done
  cad_bins
  echo "trivy extraido da imagem fixada"
  compose up -d >/dev/null 2>&1
  for i in $(seq 1 90); do dt GET /api/version >/dev/null 2>&1 && break; sleep 2; done
  dt GET /api/version | jget '"servidor: %s %s" % (d["application"], d["version"])'
  local tok=""
  for i in $(seq 1 60); do
    if tok="$(dt POST /api/v1/user/login --form username=admin "password=$ADMIN_PASS" 2>/dev/null)" && [ -n "$tok" ]; then break; fi
    dt POST /api/v1/user/forceChangePassword --form username=admin password=admin \
      "newPassword=$ADMIN_PASS" "confirmPassword=$ADMIN_PASS" >/dev/null 2>&1 || true
    tok=""; sleep 2
  done
  [ -n "$tok" ] || { echo "ERRO: login admin"; return 1; }
  echo "admin: senha inicial trocada e login ok"
  export DT_AUTH="Authorization: Bearer $tok"
  dt POST /api/v1/configProperty --json '{"groupName":"telemetry","propertyName":"submission.enabled","propertyValue":"false"}' >/dev/null 2>&1 || true
  local team key perm
  team="$(dt PUT /api/v1/team --json '{"name":"aurum-tut"}' | jget 'd["uuid"]')"
  key="$(dt PUT "/api/v1/team/$team/key" | jget 'd["key"]')"
  for perm in BOM_UPLOAD VIEW_PORTFOLIO VIEW_POLICY_VIOLATION; do
    dt POST "/api/v1/permission/$perm/team/$team" >/dev/null 2>&1 || true
  done
  echo "time aurum-tut com permissoes BOM_UPLOAD, VIEW_PORTFOLIO, VIEW_POLICY_VIOLATION e chave de API criada"
  local p ids=()
  for p in servico-exemplo compartilhado servico-a servico-b; do
    ids+=("$(dt PUT /api/v1/project --json "{\"name\":\"$p\",\"version\":\"1.0.0\",\"active\":true}" | jget 'd["uuid"]')")
    echo "projeto $p criado"
  done
  local pol
  pol="$(dt PUT /api/v1/policy --json '{"name":"demo-componente-proibido","operator":"ANY","violationState":"FAIL"}' | jget 'd["uuid"]')"
  dt PUT "/api/v1/policy/$pol/condition" --json \
    '{"subject":"COORDINATES","operator":"MATCHES","value":"{\"group\":\"*\",\"name\":\"lodash\",\"version\":\"4.17.15\"}"}' >/dev/null
  echo "politica demo-componente-proibido: reprova lodash 4.17.15 (coordenadas)"
  # Aquecimento: a PRIMEIRA avaliacao de politica de um servidor recem-criado
  # termina depois de "processing: false" (veja "Problemas comuns" no tutorial).
  # Um BOM descartavel com o componente proibido, num projeto a parte, faz o
  # servidor avaliar a politica uma vez; so seguimos quando a violacao aparece.
  local scratch bom
  scratch="$(dt PUT /api/v1/project --json '{"name":"aquecimento","version":"1.0.0","active":true}' | jget 'd["uuid"]')"
  bom="$(printf '{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":[{"type":"library","name":"lodash","version":"4.17.15","purl":"pkg:npm/lodash@4.17.15"}]}' | base64 -w0)"
  dt PUT /api/v1/bom --json "{\"project\":\"$scratch\",\"bom\":\"$bom\"}" >/dev/null
  for i in $(seq 1 60); do
    [ "$(dt GET "/api/v1/violation/project/$scratch" | jget 'len(d)')" -ge 1 ] && break
    sleep 2
  done
  [ "$(dt GET "/api/v1/violation/project/$scratch" | jget 'len(d)')" -ge 1 ] || { echo "ERRO: aquecimento sem violacao"; return 1; }
  echo "aquecimento: o servidor avaliou a politica uma vez (projeto descartavel aquecimento)"
  ( umask 077; printf 'DTRACK_API_KEY=%s\nP_EXEMPLO=%s\nP_COMPARTILHADO=%s\nP_A=%s\nP_B=%s\n' "$key" "${ids[0]}" "${ids[1]}" "${ids[2]}" "${ids[3]}" > "$STATE/dtrack.env" )
  echo "chave e ids dos projetos gravados em arquivo local ignorado pelo git"
  echo "RESULTADO: servidor, time, chave, quatro projetos e uma politica de violacao prontos"
}

# 1. aurumcode sbom: versao minima do CycloneDX e formatos recusados (sem rede).
caso_sbom_versao_minima() {
  tut_repo sbom-versao-minima repo-exemplo/base repo-exemplo/servico config/versao-malformada
  TUT_NET=none; cad_bins
  # o nome do arquivo temporario (.sbom-<numero>.tmp) muda a cada execucao
  TUT_SED='s#/work/\.sbom-[0-9]+\.tmp#<temporario>#'
  echo "--- spec_version \"1.6\" e um MINIMO: o Trivy emite 1.7 e o SBOM e aceito"
  usa_config gate
  gera_sbom
  expect_rc 0 "SBOM CycloneDX 1.7 aceito contra o minimo 1.6 (mesma major)"
  echo "--- major diferente (minimo \"2.0\"): recusado, nenhum arquivo"
  rm -f "$TUT_WORK/sbom_app_cyclonedx.json"; usa_config versao-2-0
  aurum sbom --repo . --trivy-bin "$TRIVY_WRAP"
  expect_rc 1 "SBOM 1.7 recusado contra minimo 2.0 (major diferente)"
  [ ! -e "$TUT_WORK/sbom_app_cyclonedx.json" ] && echo "RESULTADO: nenhum SBOM foi escrito"
  echo "--- spec_version fora do formato major.minor"
  usa_config versao-malformada
  aurum sbom --repo . --trivy-bin "$TRIVY_WRAP"
  expect_rc 2 "\"1.6.0\" falha na carga da configuracao, antes do Trivy"
  echo "--- formato diferente de cyclonedx"
  usa_config formato-spdx
  aurum sbom --repo . --trivy-bin "$TRIVY_WRAP"
  expect_rc 2 "format: spdx falha na carga da configuracao, antes do Trivy"
  TUT_NET=host
}

# 2. Upload e metricas num servidor local.
caso_upload_e_metricas() {
  load_env
  tut_repo upload-e-metricas repo-exemplo/base repo-exemplo/servico config/gate
  envs_dt "$P_EXEMPLO"
  gera_sbom
  dt_assenta "$P_EXEMPLO" 0
  review
  expect_rc 0 "o gate enviou o SBOM, esperou o processamento e aprovou"
  dt_metricas "$P_EXEMPLO"
  dt_componentes servico-exemplo "$P_EXEMPLO"
}

# 3. Limiares: o mesmo SBOM, dois limites.
caso_limiares() {
  load_env
  tut_repo limiares repo-exemplo/base repo-exemplo/lodash config/gate
  envs_dt "$P_EXEMPLO"
  gera_sbom
  dt_assenta "$P_EXEMPLO" 1
  echo "--- policy_violations: 0 (o servidor reprova lodash 4.17.15)"
  review
  expect_rc 3 "uma violacao de politica acima do limite 0 reprova o gate"
  dt_metricas "$P_EXEMPLO"
  echo "--- o mesmo SBOM com policy_violations: 1"
  usa_config limiar-1
  review
  expect_rc 0 "com limite 1, a mesma violacao esta dentro do limiar e o gate aprova"
}

# 4. Violacao da politica do servidor: ler a violacao e corrigi-la.
caso_violacao_de_politica() {
  load_env
  tut_repo violacao-de-politica repo-exemplo/base repo-exemplo/lodash config/gate
  envs_dt "$P_EXEMPLO"
  gera_sbom
  dt_assenta "$P_EXEMPLO" 1
  review
  expect_rc 3 "a politica do servidor reprova o componente"
  dt_violacoes "$P_EXEMPLO"
  echo "--- correcao: lodash sai, semver entra; novo SBOM para o MESMO projeto"
  cp -R "$HERE/repo-exemplo/servico/." "$TUT_WORK/"
  rm -rf "$TUT_WORK/node_modules"
  tgit add -A; tgit commit -q -m "fix: sem lodash 4.17.15"
  gera_sbom
  dt_assenta "$P_EXEMPLO" 0
  review
  expect_rc 0 "sem o componente proibido, o gate aprova"
  dt_violacoes "$P_EXEMPLO"
}

# 5. Falha: secret ausente e inconclusivo (nunca aprovado).
caso_secret_ausente() {
  tut_repo secret-ausente repo-exemplo/base repo-exemplo/servico config/gate
  TUT_ENVS=(); TUT_NET=none
  echo "--- DTRACK_API_KEY e DTRACK_PROJECT_ID nao definidos, gate.inconclusive: block"
  review
  expect_rc 1 "secret ausente com block: o gate reprova (inconclusivo, nunca aprovado)"
  echo "--- o mesmo caso com gate.inconclusive: warn"
  usa_config warn
  review
  expect_rc 0 "com warn, o alerta inconclusivo e publicado e o comando sai 0"
  TUT_NET=host
}

# 6. Falha: timeout de processamento (servidor falso que nunca termina).
caso_timeout() {
  tut_repo timeout repo-exemplo/base repo-exemplo/servico config/lento
  TUT_NET=none
  TUT_ENVS=(-e "DTRACK_API_KEY=chave-de-demonstracao" -e "DTRACK_PROJECT_ID=00000000-0000-0000-0000-000000000000")
  gera_sbom
  echo "--- servidor falso em 127.0.0.1:8099 (servidor-lento.py): aceita o upload e responde processing=true para sempre"
  TUT_PRE='python3 /fixtures/servidor-lento.py'
  review
  expect_rc 1 "timeout_seconds: 2 vencido com block: inconclusivo e reprovado"
  TUT_PRE=; TUT_NET=host
}

# 7. Um projeto por microsservico.
caso_projeto_por_microservico() {
  load_env
  local wa wb
  tut_repo projeto-a repo-exemplo/base repo-exemplo/servico-a config/limiar-1; wa="$TUT_WORK"
  tut_repo projeto-b repo-exemplo/base repo-exemplo/servico-b config/limiar-1; wb="$TUT_WORK"
  echo "--- servico-a e servico-b enviam para o MESMO projeto (compartilhado)"
  TUT_WORK="$wa"; envs_dt "$P_COMPARTILHADO"; gera_sbom; review
  expect_rc 0 "servico-a enviado"
  dt_componentes compartilhado "$P_COMPARTILHADO"
  TUT_WORK="$wb"; envs_dt "$P_COMPARTILHADO"; gera_sbom; review
  expect_rc 0 "servico-b enviado ao mesmo projeto"
  local lista; lista="$(dt_componentes compartilhado "$P_COMPARTILHADO")"; echo "$lista"
  case "$lista" in *ms@2.1.3*) echo "ERRO: o componente do servico-a sobreviveu"; return 1 ;; esac
  case "$lista" in *semver@7.6.0*) echo "RESULTADO: o segundo upload substituiu o primeiro: ms@2.1.3 (servico-a) sumiu do projeto compartilhado (conclusao do script: ms ausente e semver presente)" ;; *) echo "ERRO: semver ausente"; return 1 ;; esac
  echo "--- um projeto por microsservico"
  TUT_WORK="$wa"; envs_dt "$P_A"; review; expect_rc 0 "servico-a no projeto servico-a"
  TUT_WORK="$wb"; envs_dt "$P_B"; review; expect_rc 0 "servico-b no projeto servico-b"
  local la lb; la="$(dt_componentes servico-a "$P_A")"; lb="$(dt_componentes servico-b "$P_B")"; echo "$la"; echo "$lb"
  case "$la" in *ms@2.1.3*) ;; *) echo "ERRO: ms ausente no projeto servico-a"; return 1 ;; esac
  case "$lb" in *semver@7.6.0*) echo "RESULTADO: com um projeto por servico, os dois inventarios coexistem (conclusao do script: ms no projeto servico-a, semver no projeto servico-b)" ;; *) echo "ERRO: semver ausente no projeto servico-b"; return 1 ;; esac
}

caso_down() {
  echo "== down: remove containers, rede, volumes e imagens baixadas so para o tutorial"
  compose down -v --remove-orphans >/dev/null 2>&1 || true
  local n n2
  n="$(docker ps -a -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)"
  n=$((n + $(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)))
  n=$((n + $(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)))
  echo "recursos restantes do tutorial: $n"
  if [ -f "$STATE/baixadas" ]; then
    while IFS= read -r name; do
      [ -z "$name" ] || { docker rmi "$(cad_lock "$name")" >/dev/null 2>&1 || true; echo "imagem $name removida (baixada so para o tutorial)"; }
    done < "$STATE/baixadas"
  fi
  rm -rf "${STATE:?}"
  [ "$n" -eq 0 ]
  echo "RESULTADO: nenhum container, rede ou volume do tutorial ficou vivo"
}

tut_main "$@"
