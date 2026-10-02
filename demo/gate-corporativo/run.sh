#!/usr/bin/env bash
# Demonstracao executavel do gate corporativo (AUR-554).
#
#   run.sh build|up|fail|fix|pass|verify|down   uma fase
#   run.sh all                                  build up fail fix pass verify down
#   run.sh --check                              compara out/ com expected/ (sem docker)
#
# Tudo roda em containers: a imagem do produto (Go + Semgrep), Dependency-Track
# + PostgreSQL (compose), e os binarios do Trivy e do Cosign extraidos das
# imagens fixadas por digest em images.lock. No host so ha bash, git, docker,
# curl e python3 (orquestracao). Cada fase grava out/<fase>.log.
set -Eeuo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "$HERE/../.." && pwd -P)"
OUT="$HERE/out"
STATE="$HERE/.estado"
EXPECTED="$HERE/expected"
PROJECT=aurum-gate-corp
IMG=aurum-demo-product:gate-corporativo
DT=http://127.0.0.1:8081
ADMIN_PASS='Demo-Only-Pass-123'
FASES=(build up fail fix pass verify down)

lock() { sed -n "s/^$1=//p" "$HERE/images.lock" | head -n1; }
compose() { docker compose -p "$PROJECT" -f "$HERE/compose.yml" "$@"; }

# aurumcode e as ferramentas rodam na imagem do produto. --network host so
# para alcancar o Dependency-Track em 127.0.0.1 (o gate exige https ou
# loopback por IP). A politica entra somente-leitura, fora da arvore revisada.
aurum() {
  docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e DTRACK_API_KEY -e DTRACK_PROJECT_ID -e AURUMCODE_LLM_FIXTURE=/demo/fixture-llm.json \
    -v "$HERE/fixture-llm.json:/demo/fixture-llm.json:ro" \
    -v "$STATE/bin:/demo/bin:ro" -v "$HERE/wrappers:/demo/wrappers:ro" \
    -v "$STATE/trivy-cache:/tmp/trivy-cache" -v "$STATE/keys:/demo/keys" \
    -v "$STATE/work:/github/workspace" -v "$HERE/politica:/github/policy:ro" \
    -w /github/workspace --entrypoint /app/aurumcode "$IMG" "$@"
}
# cosign (verify) isolado, sem rede.
cosign_offline() {
  docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -v "$STATE/bin:/demo/bin:ro" -v "$STATE/keys:/demo/keys:ro" \
    -v "$STATE/work:/github/workspace:ro" -w /github/workspace \
    --entrypoint /demo/bin/cosign "$IMG" "$@"
}

load_env() {
  [ -f "$STATE/dtrack.env" ] || { echo "ERRO: rode a fase up antes (sem dtrack.env)"; return 1; }
  # shellcheck disable=SC1091
  . "$STATE/dtrack.env"
  export DTRACK_API_KEY DTRACK_PROJECT_ID
}

jget() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }

dt_admin_token() {
  curl -fsS -X POST "$DT/api/v1/user/login" --data-urlencode username=admin --data-urlencode "password=$ADMIN_PASS"
}

# Metricas e violacoes lidas direto da API, com a chave do time (nunca impressa).
# As metricas do v5 sao calculadas no fim do fluxo de processamento do BOM; o
# gate espera o token de processamento terminar antes de le-las, e esta leitura
# so confirma os mesmos numeros (a chave do time nao tem permissao de refresh).
dt_report() {
  curl -fsS -H "X-Api-Key: $DTRACK_API_KEY" "$DT/api/v1/metrics/project/$DTRACK_PROJECT_ID/current" |
    jget '"dependency-track metricas: critical=%d high=%d policyViolationsTotal=%d" % (d.get("critical",0), d.get("high",0), d.get("policyViolationsTotal",0))'
  curl -fsS -H "X-Api-Key: $DTRACK_API_KEY" "$DT/api/v1/violation/project/$DTRACK_PROJECT_ID" |
    python3 -c '
import json, sys
v = json.load(sys.stdin)
if not v:
    print("dependency-track violacoes: nenhuma")
for x in v:
    pol = x["policyCondition"]["policy"]["name"]
    c = x["component"]
    print("dependency-track violacao: politica=%s componente=%s versao=%s estado=%s" % (pol, c["name"], c.get("version"), x["policyCondition"]["policy"]["violationState"]))
'
}

# ---------------------------------------------------------------- fases
fase_build() {
  echo "== build: imagem do produto, imagens fixadas, binarios do Trivy e do Cosign"
  docker build -q -t "$IMG" "$REPO_ROOT" >/dev/null
  echo "imagem do produto: construida a partir do Dockerfile"
  local want got
  want="$(sed -n 's/^sast_scanner_version:[[:space:]]*//p' "$REPO_ROOT/.board/bootstrap/locks/scanners.yml")"
  got="$(docker run --rm -e SEMGREP_ENABLE_VERSION_CHECK=0 --entrypoint semgrep "$IMG" --version | tr -d '\r')"
  [ "$want" = "$got" ] || { echo "ERRO: semgrep $got difere de scanners.yml ($want)"; return 1; }
  echo "semgrep $got: igual a scanners.yml"
  local name ref
  for name in dtrack_apiserver postgres trivy semgrep cosign; do
    ref="$(lock "$name")"
    case "$ref" in *@sha256:*) ;; *) echo "ERRO: $name sem digest"; return 1 ;; esac
    docker image inspect "$ref" >/dev/null 2>&1 || docker pull -q "$ref" >/dev/null
    echo "imagem fixada por digest: $name ok"
  done
  # Trivy e Cosign sao binarios estaticos: saem das imagens fixadas.
  rm -rf "${STATE:?}/bin" "${STATE:?}/work" "${STATE:?}/keys"
  mkdir -p "$STATE/bin" "$STATE/keys" "$STATE/trivy-cache"
  local c
  c="$(docker create "$(lock trivy)")"; docker cp "$c:/usr/local/bin/trivy" "$STATE/bin/trivy"; docker rm "$c" >/dev/null
  c="$(docker create "$(lock cosign)")"; docker cp "$c:/ko-app/cosign" "$STATE/bin/cosign"; docker rm "$c" >/dev/null
  chmod +x "$STATE/bin/"*
  echo "trivy e cosign extraidos das imagens fixadas"
  # Repositorio de exemplo como repositorio git: main (vazio) e feature (o PR).
  mkdir -p "$STATE/work"
  cp -R "$HERE/repo-exemplo/." "$STATE/work/"
  git -C "$STATE/work" init -q -b main
  git -C "$STATE/work" -c user.name=Demo -c user.email=demo@example.invalid commit -q --allow-empty -m "base"
  git -C "$STATE/work" checkout -q -b feature
  git -C "$STATE/work" add -A
  git -C "$STATE/work" -c user.name=Demo -c user.email=demo@example.invalid commit -q -m "feature: calculadora"
  echo "repo-exemplo pronto: branch feature com o defeito de SAST e a dependencia plantada"
}

fase_up() {
  echo "== up: Dependency-Track v5 + PostgreSQL"
  mkdir -p "$STATE"
  compose up -d >/dev/null 2>&1
  local i
  for i in $(seq 1 90); do
    curl -fsS "$DT/api/version" >/dev/null 2>&1 && break
    sleep 2
  done
  curl -fsS "$DT/api/version" | jget '"servidor: %s %s" % (d["application"], d["version"])'
  local tok=""
  for i in $(seq 1 60); do
    if tok="$(dt_admin_token 2>/dev/null)" && [ -n "$tok" ]; then break; fi
    # Senha inicial exige troca: automatizada, so no primeiro uso.
    curl -fsS -o /dev/null -X POST "$DT/api/v1/user/forceChangePassword" \
      --data-urlencode username=admin --data-urlencode password=admin \
      --data-urlencode "newPassword=$ADMIN_PASS" --data-urlencode "confirmPassword=$ADMIN_PASS" 2>/dev/null || true
    tok=""; sleep 2
  done
  [ -n "$tok" ] || { echo "ERRO: login admin"; return 1; }
  echo "admin: senha inicial trocada e login ok"
  local H=(-H "Authorization: Bearer $tok" -H "Content-Type: application/json")
  curl -sS -o /dev/null "${H[@]}" -X POST "$DT/api/v1/configProperty" \
    -d '{"groupName":"telemetry","propertyName":"submission.enabled","propertyValue":"false"}' || true

  # time + chave (reaproveita o time se ja existir)
  local team
  team="$(curl -fsS "${H[@]}" "$DT/api/v1/team" | jget 'next((t["uuid"] for t in d if t["name"]=="aurum-demo"), "")')"
  if [ -z "$team" ]; then
    team="$(curl -fsS "${H[@]}" -X PUT "$DT/api/v1/team" -d '{"name":"aurum-demo"}' | jget 'd["uuid"]')"
  fi
  local key
  key="$(curl -fsS "${H[@]}" -X PUT "$DT/api/v1/team/$team/key" | jget 'd["key"]')"
  local perm
  for perm in BOM_UPLOAD VIEW_PORTFOLIO VIEW_POLICY_VIOLATION; do
    curl -sS -o /dev/null "${H[@]}" -X POST "$DT/api/v1/permission/$perm/team/$team" || true
  done
  echo "time aurum-demo com permissoes BOM_UPLOAD, VIEW_PORTFOLIO, VIEW_POLICY_VIOLATION e chave de API criada"

  # projeto (um por servico)
  local proj
  proj="$(curl -fsS "${H[@]}" "$DT/api/v1/project?name=servico-exemplo" | jget 'next((p["uuid"] for p in d if p["name"]=="servico-exemplo"), "")')"
  if [ -z "$proj" ]; then
    proj="$(curl -fsS "${H[@]}" -X PUT "$DT/api/v1/project" -d '{"name":"servico-exemplo","version":"1.0.0","active":true}' | jget 'd["uuid"]')"
  fi
  echo "projeto servico-exemplo criado"

  # politica de violacao: reprova o componente plantado, de forma deterministica
  local have
  have="$(curl -fsS "${H[@]}" "$DT/api/v1/policy" | jget 'next((p["uuid"] for p in d if p["name"]=="demo-componente-proibido"), "")')"
  if [ -z "$have" ]; then
    local pol
    pol="$(curl -fsS "${H[@]}" -X PUT "$DT/api/v1/policy" -d '{"name":"demo-componente-proibido","operator":"ANY","violationState":"FAIL"}' | jget 'd["uuid"]')"
    curl -fsS -o /dev/null "${H[@]}" -X PUT "$DT/api/v1/policy/$pol/condition" \
      -d '{"subject":"COORDINATES","operator":"MATCHES","value":"{\"group\":\"*\",\"name\":\"lodash\",\"version\":\"4.17.15\"}"}'
  fi
  echo "politica demo-componente-proibido: reprova lodash 4.17.15 (coordenadas)"

  ( umask 077; printf 'DTRACK_API_KEY=%s\nDTRACK_PROJECT_ID=%s\n' "$key" "$proj" > "$STATE/dtrack.env" )
  echo "DTRACK_API_KEY e DTRACK_PROJECT_ID gravados em arquivo local ignorado pelo git"
}

gate_sbom_review() {
  aurum sbom --repo . --politica /github/policy --trivy-bin /demo/wrappers/trivy-wrap.sh | sed 's#^sbom: .*/#sbom: #'
  python3 - "$STATE/work/sbom_app_cyclonedx.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print("sbom: CycloneDX %s, componentes: %s" % (d["specVersion"], ", ".join("%s@%s" % (c["name"], c["version"]) for c in d["components"] if c.get("version"))))
PY
  set +e
  aurum review --base main --seguranca --politica /github/policy \
    --auditoria /tmp/auditoria.json --sarif /tmp/revisao.sarif 2>&1
  GATE_RC=$?
  set -e
  echo "aurumcode review: exit_code=$GATE_RC"
}

fase_fail() {
  echo "== fail: gate com o defeito de SAST e a dependencia plantada"
  load_env
  gate_sbom_review
  dt_report
  if [ "$GATE_RC" -eq 3 ]; then echo "RESULTADO: gate reprovou, como esperado"; else echo "ERRO: esperado exit 3, obtido $GATE_RC"; return 1; fi
}

fase_fix() {
  echo "== fix: remove o eval e atualiza a dependencia"
  cp -R "$HERE/correcao/." "$STATE/work/"
  git -C "$STATE/work" add -A
  git -C "$STATE/work" -c user.name=Demo -c user.email=demo@example.invalid commit -q -m "fix: sem eval, lodash 4.17.21"
  echo "correcao aplicada: src/calc.js sem eval; lodash 4.17.15 -> 4.17.21"
  git -C "$STATE/work" log --format='commit: %s' main..HEAD
}

fase_pass() {
  echo "== pass: gate aprova, SBOM enviado e assinado"
  load_env
  gate_sbom_review
  dt_report
  [ "$GATE_RC" -eq 0 ] || { echo "ERRO: esperado exit 0, obtido $GATE_RC"; return 1; }
  echo "RESULTADO: gate aprovou"
  # chave efemera: nasce aqui, nunca e versionada
  docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp -e COSIGN_PASSWORD= \
    -v "$STATE/bin:/demo/bin:ro" -v "$STATE/keys:/demo/keys" -w /demo/keys \
    --entrypoint /demo/bin/cosign "$IMG" generate-key-pair 2>&1 | sed 's/^/cosign: /'
  aurum sign --repo . --politica /github/policy --cosign-bin /demo/wrappers/cosign-wrap.sh 2>&1
  [ -s "$STATE/work/sbom_app_cyclonedx.json.sigstore.json" ] || { echo "ERRO: bundle ausente"; return 1; }
  echo "assinatura: sbom_app_cyclonedx.json.sigstore.json gravado"
}

fase_verify() {
  echo "== verify: cosign verify-blob com a chave efemera"
  cosign_offline verify-blob --key /demo/keys/cosign.pub \
    --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct \
    sbom_app_cyclonedx.json 2>&1
  echo "verify-blob: assinatura aceita"
  # prova negativa: uma copia adulterada do SBOM nao verifica
  local tmp="${STATE:?}/adulterado"
  rm -rf "$tmp"; mkdir -p "$tmp"
  cp "$STATE/work/sbom_app_cyclonedx.json.sigstore.json" "$tmp/"
  { cat "$STATE/work/sbom_app_cyclonedx.json"; echo " "; } > "$tmp/sbom_app_cyclonedx.json"
  local rc=0 err
  err="$(docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \
      -v "$STATE/bin:/demo/bin:ro" -v "$STATE/keys:/demo/keys:ro" -v "$tmp:/w:ro" -w /w \
      --entrypoint /demo/bin/cosign "$IMG" verify-blob --key /demo/keys/cosign.pub \
      --bundle sbom_app_cyclonedx.json.sigstore.json --insecure-ignore-tlog --insecure-ignore-sct \
      sbom_app_cyclonedx.json 2>&1)" || rc=$?
  [ "$rc" -ne 0 ] || { echo "ERRO: SBOM adulterado foi aceito"; return 1; }
  # so conta como rejeicao a falha de verificacao do proprio cosign, nao uma falha do docker
  printf '%s\n' "$err" | grep -q 'invalid signature when validating ASN.1 encoded signature' || {
    echo "ERRO: falha inesperada (nao e rejeicao de assinatura): $err"; return 1; }
  printf '%s\n' "$err" | grep -E 'invalid signature|Error' | head -n1 | sed 's/^/cosign (adulterado): /'
  echo "adulterado: SBOM modificado e rejeitado"
}

fase_down() {
  echo "== down: remove containers, rede e volumes da demonstracao"
  compose down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf "${STATE:?}"
  local n
  n="$(docker ps -a -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)"
  n=$((n + $(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)))
  n=$((n + $(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" | wc -l)))
  echo "recursos restantes da demonstracao: $n"
  [ "$n" -eq 0 ]
}

run_fase() {
  local f="$1"
  mkdir -p "$OUT"
  set +e
  ( set -Ee; "fase_$f" ) 2>&1 | tee "$OUT/$f.log"
  local rc="${PIPESTATUS[0]}"
  set -e
  return "$rc"
}

# --check: cada linha de expected/<fase>.txt precisa aparecer em out/<fase>.log.
check() {
  local f line
  for f in "${FASES[@]}"; do
    [ -f "$OUT/$f.log" ] || { echo "DIVERGENCIA fase=$f: out/$f.log ausente"; return 1; }
    [ -f "$EXPECTED/$f.txt" ] || { echo "DIVERGENCIA fase=$f: expected/$f.txt ausente"; return 1; }
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in ''|'#'*) continue ;; esac
      if ! grep -qF -- "$line" "$OUT/$f.log"; then
        echo "DIVERGENCIA fase=$f: trecho esperado ausente: $line"
        return 1
      fi
    done < "$EXPECTED/$f.txt"
    echo "fase $f: ok"
  done
  echo "CHECK OK"
}

main() {
  case "${1:-}" in
    --check) check ;;
    all) local f; for f in "${FASES[@]}"; do run_fase "$f"; done ;;
    build|up|fail|fix|pass|verify|down) run_fase "$1" ;;
    *) echo "uso: run.sh build|up|fail|fix|pass|verify|down|all|--check" >&2; exit 64 ;;
  esac
}
main "$@"
