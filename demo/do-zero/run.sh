#!/usr/bin/env bash
# POC interativa: do zero ao primeiro parecer do AurumCode, em tres repositorios
# (security-policy, devops-policy e o servico), passo a passo, no terminal.
# Veja README.md.
#
#   run.sh [--modo mock|real] [--auto] [--repo NOME] [--privado] [--ref REF] [--mostrar]
#       roteiro guiado: os 12 passos, cada comando mostrado e executado no ⏎
#   run.sh ia [--modo mock|real] [--destino DIR] [--repo NOME] [--privado] [--ref REF]
#       cria os dois repositorios centrais e so prepara o servico (boilerplate +
#       git) com o AurumCode registrado como servidor MCP (.mcp.json); imprime o
#       pedido para o seu agente de IA apontar o servico para as duas camadas
#       (docs/tutorials/instalacao-ia.md)
#   run.sh ia-defeito [--destino DIR]   a mudanca com defeito, numa branch
#   run.sh ia-correcao [--destino DIR]  a correcao, na mesma branch
#   run.sh limpar [--destino DIR]       apaga .estado/ (os tres repositorios) e o projeto do modo ia
#   run.sh --check                      confere out/roteiro-mock.log (a ultima execucao real de
#                                       --modo mock --auto, valores volateis normalizados) com
#                                       expected/roteiro-mock.txt e out/.imagem, sem docker
#
# mock (padrao): tudo local. O "GitHub" sao tres repositorios bare em
#   .estado/remotos/ (security-policy, devops-policy, assistente) e um servidor
#   falso da API em 127.0.0.1 (demo/tutoriais/_lib/github-falso.py); o modelo e
#   o fixture deterministico fixture-llm.json; sem rede, sem segredo.
# real: gh autenticado na sua conta, tres repositorios NOVOS no GitHub com nomes
#   derivados de --repo (NOME-security-policy, NOME-devops-policy e NOME),
#   secrets reais lidos do ambiente (LLM_API_KEY, LLM_BASE_URL e, opcional,
#   LLM_MODEL), PR real revisada pelo workflow; o gate local e o agente usam o
#   mesmo modelo. --ref fixa a versao do AurumCode no workflow (padrao: v2.0.0
#   se a tag existir no GitHub, senao main). --privado cria os tres privados e
#   exige AURUMCODE_POLICY_TOKEN e AURUMCODE_STANDARDS_TOKEN no ambiente (token
#   fine-grained so de leitura nos dois repositorios centrais). --mostrar so
#   imprime os comandos gh que o modo real executaria, sem rede e sem gh.
#
# No host: bash, git, docker, python3 (e gh no modo real). O aurumcode roda na
# imagem do produto, construida do Dockerfile da raiz na primeira execucao.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../tutoriais/_lib/tutorial.sh
. "$HERE/../tutoriais/_lib/tutorial.sh"
# shellcheck source=../tutoriais/_lib/pr.sh
. "$HERE/../tutoriais/_lib/pr.sh"

MODO=mock AUTO=0 NOME="" PRIVADO=0 REF="${AZ_REF:-}" ACAO=roteiro DESTINO="" MOSTRAR=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    ia|ia-defeito|ia-correcao) ACAO="$1"; shift ;;
    --destino) DESTINO="$2"; shift 2 ;;
    --modo) MODO="$2"; shift 2 ;;
    --auto) AUTO=1; shift ;;
    --repo) NOME="$2"; shift 2 ;;
    --privado) PRIVADO=1; shift ;;
    --ref) REF="$2"; shift 2 ;;
    --mostrar) MOSTRAR=1; shift ;;
    limpar) ACAO=limpar; shift ;;
    --check) ACAO=check; shift ;;
    *) echo "uso: run.sh [ia|ia-defeito|ia-correcao] [--modo mock|real] [--auto] [--destino DIR] [--repo NOME] [--privado] [--ref REF] [--mostrar] | limpar | --check" >&2; exit 2 ;;
  esac
done
if [ "$MODO" != mock ] && [ "$MODO" != real ]; then echo "--modo: mock ou real" >&2; exit 2; fi
if [ "$MOSTRAR" = 1 ] && [ "$MODO" != real ]; then echo "--mostrar só faz sentido com --modo real" >&2; exit 2; fi
NOME_DIR="${NOME#*/}"; [ -n "$DESTINO" ] || DESTINO="${HOME:-/tmp}/aurum-poc/${NOME_DIR:-assistente}"
if [ "$ACAO" = limpar ]; then
  rm -rf "$STATE"; echo "apagado: $STATE (security-policy, devops-policy e o serviço)"
  if [ -f "$DESTINO/.git/aurum-poc" ]; then rm -rf "$DESTINO"; echo "apagado: $DESTINO"; fi
  exit 0
fi
# --check: a evidencia versionada do roteiro mock, como nos tutoriais
# (_lib/tutorial.sh): cada linha de expected/roteiro-mock.txt precisa estar em
# out/roteiro-mock.log, e out/.imagem precisa ser desta arvore.
if [ "$ACAO" = check ]; then CASOS=(roteiro-mock); tut_check; exit 0; fi

# ---------------------------------------------------------------- interface
ESC="$(printf '\033')"
if [ -t 1 ]; then B="$ESC[1m" D="$ESC[2m" C="$ESC[36m" G="$ESC[32m" Y="$ESC[33m" N="$ESC[0m"; else B="" D="" C="" G="" Y="" N=""; fi
PASSOS=("Projeto: o boilerplate" "Git e o remoto" "Segurança: o repositório security-policy" "Engenharia: o repositório devops-policy"
  "O serviço aponta para as duas camadas" "Secrets e tokens, fora do código" "Uma mudança com defeito" "O agente pergunta antes do PR"
  "PR: parecer bloqueado pelas três camadas" "O serviço tenta afrouxar e não consegue" "Correção: parecer aprovado" "Resumo")
PASSO=0
ui_passo() {
  PASSO="$1"
  local i
  printf '\n%s════════════════════════════════════════════════════════════════════════%s\n' "$D" "$N"
  printf '%sAurumCode do zero, em camadas%s  %smodo %s%s\n' "$B" "$N" "$D" "$MODO" "$N"
  for i in "${!PASSOS[@]}"; do
    if [ "$((i + 1))" -lt "$PASSO" ]; then printf '  %s✔ %2d. %s%s\n' "$G" "$((i + 1))" "${PASSOS[$i]}" "$N"
    elif [ "$((i + 1))" -eq "$PASSO" ]; then printf '  %s▶ %2d. %s%s\n' "$B" "$((i + 1))" "${PASSOS[$i]}" "$N"
    else printf '  %s  %2d. %s%s\n' "$D" "$((i + 1))" "${PASSOS[$i]}" "$N"; fi
  done
  echo
}
ui_diz() { printf '%s\n' "$*" | fold -s -w 78 | sed 's/^/  /'; }
ui_cmd() { printf '  %s$ %s%s\n' "$C" "$*" "$N"; }
# ui_copia: comando para a pessoa copiar, inteiro numa linha (nunca quebrado)
ui_copia() { printf '      %s%s%s\n' "$C" "$*" "$N"; }
ui_ok() { printf '  %s%s%s\n' "$G" "$*" "$N"; }
ui_aviso() { printf '  %s%s%s\n' "$Y" "$*" "$N"; }
ui_pausa() { [ "$AUTO" = 1 ] || { printf '  %s⏎ para executar%s' "$D" "$N"; read -r _; }; }
ui_saida() { sed 's/^/    │ /'; }
# ui_run DESCRICAO -- comando...: mostra, espera, executa no projeto, imprime a saida.
ui_run() {
  local desc="$1"; shift; [ "${1:-}" = "--" ] && shift
  ui_cmd "$*"; ui_pausa
  (cd "$TUT_WORK" && "$@" 2>&1) | ui_saida || true
}

# ---------------------------------------------------------------- nomes e diretorios
# Tres repositorios. mock: dono "voce", nomes fixos (security-policy,
# devops-policy, assistente). real: nomes derivados de --repo na sua conta
# (NOME-security-policy, NOME-devops-policy, NOME), para nao colidir com nada seu.
az_nomes() {
  if [ "$MODO" = mock ]; then
    DONO=voce; [ -n "$NOME" ] || NOME=assistente; NOME="${NOME#*/}"
    NOME_SEC=security-policy; NOME_DEV=devops-policy; [ -n "$REF" ] || REF=main
  else
    if [ "${NOME#*/}" != "$NOME" ]; then DONO="${NOME%%/*}"; NOME="${NOME#*/}"; fi
    [ -n "$NOME" ] || NOME="aurum-do-zero-$(date +%Y%m%d-%H%M)"
    NOME_SEC="$NOME-security-policy"; NOME_DEV="$NOME-devops-policy"
  fi
}
SEC_DIR="$STATE/security-policy"   # checkout do time de seguranca (e o clone do dev)
DEV_DIR="$STATE/devops-policy"     # checkout do time de engenharia (e o clone do dev)
# As duas camadas, montadas como o workflow as monta (/github/policy e
# /github/standards), para o gate local e o servidor MCP.
CAMADAS_MONTA=(-v "$SEC_DIR:/github/policy:ro" -v "$DEV_DIR:/github/standards:ro")
CAMADAS_FLAGS=(--politica /github/policy --padroes /github/standards)
VIS=--public; [ "$PRIVADO" = 1 ] && VIS=--private

# git_em DIR args...: git com a identidade da demonstracao em DIR (so nos
# repositorios descartaveis; a configuracao git do usuario nunca e tocada).
git_em() {
  local dir="$1"; shift
  git -C "$dir" -c user.name=Demo -c user.email=demo@example.invalid -c commit.gpgsign=false -c init.defaultBranch=main "$@"
}

# ---------------------------------------------------------------- comandos gh (texto e execucao)
# Montados num lugar so: --mostrar os imprime; o modo real os imprime e executa.
cmd_repo_create() { printf 'gh repo create %s/%s %s --source=%s --remote=origin --push' "$DONO" "$1" "$VIS" "$2"; }
cmd_secret() { printf 'gh secret set %s --repo %s/%s --body "$%s"' "$1" "$DONO" "$NOME" "$2"; }
cmd_variable() { printf 'gh variable set %s --repo %s/%s --body "$%s"' "$1" "$DONO" "$NOME" "$2"; }
# az_mostrar: o modo real ate onde nao ha rede: nomes, comandos e tokens.
az_mostrar() {
  printf '%sAurumCode do zero, em camadas%s  %smodo real, --mostrar: nada é executado%s\n\n' "$B" "$N" "$D" "$N"
  ui_diz "Três repositórios na conta $DONO (todos $VIS):"
  ui_copia "$DONO/$NOME_SEC   política de segurança (gate, IA-001, ignore, exceções)"
  ui_copia "$DONO/$NOME_DEV   padrões de engenharia (skills PAD-001 e QA-001, analistas padroes e qa)"
  ui_copia "$DONO/$NOME   o serviço, cujo workflow aponta para os dois"
  ui_diz "Comandos que o roteiro executa, na ordem:"
  ui_copia "$(cmd_repo_create "$NOME_SEC" security-policy)"
  ui_copia "$(cmd_repo_create "$NOME_DEV" devops-policy)"
  ui_copia "$(cmd_repo_create "$NOME" "$NOME")"
  ui_copia "$(cmd_secret LLM_API_KEY LLM_API_KEY)"
  ui_copia "$(cmd_secret LLM_BASE_URL LLM_BASE_URL)"
  ui_copia "$(cmd_variable LLM_MODEL LLM_MODEL)"
  if [ "$PRIVADO" = 1 ]; then
    ui_diz "Repositórios privados: o workflow do serviço precisa ler os dois centrais. Crie um token fine-grained com Contents: read só em $DONO/$NOME_SEC e $DONO/$NOME_DEV, exporte-o como AURUMCODE_POLICY_TOKEN e AURUMCODE_STANDARDS_TOKEN (pode ser o mesmo valor) e o roteiro cadastra:"
    ui_copia "$(cmd_secret AURUMCODE_POLICY_TOKEN AURUMCODE_POLICY_TOKEN)"
    ui_copia "$(cmd_secret AURUMCODE_STANDARDS_TOKEN AURUMCODE_STANDARDS_TOKEN)"
  else
    ui_diz "Repositórios públicos: o checkout das duas camadas usa o github.token do próprio workflow; AURUMCODE_POLICY_TOKEN e AURUMCODE_STANDARDS_TOKEN só são necessários com --privado."
  fi
  ui_diz "No workflow do serviço (.github/workflows/code-review.yml) os dois repositórios centrais entram como inputs, cada um fixado na SHA da main do repositório correspondente, e o AurumCode pela ref fixa:"
  ui_copia "policy_repository: $DONO/$NOME_SEC   policy_ref: <SHA da main de $NOME_SEC>"
  ui_copia "standards_repository: $DONO/$NOME_DEV   standards_ref: <SHA da main de $NOME_DEV>"
  ui_copia "uses: Mpaape/AurumCode/.github/workflows/review.yml@${REF:-v2.0.0 ou main}"
  ui_diz "Pré-requisitos do modo real: gh autenticado (gh auth login) e LLM_API_KEY, LLM_BASE_URL (e LLM_MODEL, se o serviço exigir) exportadas."
}

# ---------------------------------------------------------------- produto
# az_aurum args...: o aurumcode sobre o projeto, com as duas camadas montadas
# (o dev tem os dois repositórios centrais clonados ao lado do serviço).
# mock: sem rede, modelo falso. real: com rede e o modelo do ambiente.
az_aurum() {
  ui_cmd "aurumcode $* ${CAMADAS_FLAGS[*]}"
  set +e
  if [ "$MODO" = mock ]; then
    LAST_OUT="$(aurum_raw -e AURUMCODE_CACHE_DIR=/tmp/cache "${CAMADAS_MONTA[@]}" -- "$@" "${CAMADAS_FLAGS[@]}" 2>&1)"
  else
    LAST_OUT="$(docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp \
      -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL -e AURUMCODE_CACHE_DIR=/tmp/cache "${CAMADAS_MONTA[@]}" \
      -v "$TUT_WORK:/work" -w /work --entrypoint /app/aurumcode "$TUT_IMAGE" "$@" "${CAMADAS_FLAGS[@]}" 2>&1)"
  fi
  LAST_RC=$?
  set -e
  printf '%s\n' "$LAST_OUT" | ui_saida
  echo "    exit_code=$LAST_RC"
}

INICIO='{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"agente-da-poc","version":"1"}}}'
INICIADO='{"jsonrpc":"2.0","method":"notifications/initialized"}'
chama() { printf '{"jsonrpc":"2.0","id":%s,"method":"tools/call","params":{"name":"%s","arguments":%s}}' "$1" "$2" "$3"; }
CLIENTE="$HERE/../tutoriais/agente/cliente-mcp.py"
# az_mcp REQUISICAO...: o agente fala com `aurumcode mcp` por stdio e resume as
# respostas. O servidor recebe as duas camadas por AURUMCODE_POLICY e
# AURUMCODE_STANDARDS (ele le so as variaveis), como .mcp.json do modo ia.
az_mcp() {
  local reqs="$STATE/mcp-requisicoes.jsonl" resp="$STATE/mcp-respostas.jsonl" r
  { printf '%s\n' "$INICIO" "$INICIADO"; for r in "$@"; do printf '%s\n' "$r"; done; } > "$reqs"
  ui_cmd "AURUMCODE_POLICY=/github/policy AURUMCODE_STANDARDS=/github/standards aurumcode mcp   # o agente manda initialize + $# requisição(ões) por stdio"
  set +e
  if [ "$MODO" = mock ]; then
    aurum_raw --interactive=true --init=false -e AURUMCODE_POLICY=/github/policy -e AURUMCODE_STANDARDS=/github/standards \
      "${CAMADAS_MONTA[@]}" -- mcp < "$reqs" > "$resp" 2> "$STATE/mcp-stderr.txt"
  else
    docker run --rm -i --user "$(id -u):$(id -g)" -e HOME=/tmp -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL \
      -e AURUMCODE_POLICY=/github/policy -e AURUMCODE_STANDARDS=/github/standards "${CAMADAS_MONTA[@]}" \
      -v "$TUT_WORK:/work" -w /work --entrypoint /app/aurumcode "$TUT_IMAGE" mcp < "$reqs" > "$resp" 2> "$STATE/mcp-stderr.txt"
  fi
  LAST_RC=$?
  set -e
  python3 -I "$CLIENTE" resume "$resp" "$reqs" | ui_saida
}
# az_id_de REGRA: o id do achado que cita REGRA na ultima resposta do agente.
az_id_de() {
  python3 -I -c 'import json,sys
for l in open(sys.argv[1]):
    sc = json.loads(l).get("result", {}).get("structuredContent", {})
    for f in sc.get("blocking_findings", []) + sc.get("findings", []):
        if f["rule_id"].startswith(sys.argv[2]):
            print(f["id"]); sys.exit(0)
sys.exit(1)' "$STATE/mcp-respostas.jsonl" "$1"
}
# az_publicado: o que o produto publicou no GitHub falso, legivel.
az_publicado() {
  python3 -I -c 'import json,sys
for l in open(sys.argv[1]):
    m = json.loads(l); metodo = "POST" if "POST" in m else "PATCH"; caminho = m[metodo]; c = m["corpo"]
    if "state" in c: print("status %s: %s — %s" % (c.get("context"), c["state"], c.get("description", "")))
    elif "body" in c and caminho.endswith("/comments") and "aurumcode-review" in c["body"]: print("comentário publicado (%s):\n%s" % (metodo, c["body"]))
    elif "body" in c and caminho.endswith("/comments"): print("comentário na linha %s: %s" % (c.get("line", "?"), c["body"].split("\n")[0]))
    elif "event" in c: print("review %s" % c["event"])
' "$TUT_PR_LOG" | ui_saida
}
# az_parecer_publicado: o corpo do ultimo parecer publicado no GitHub falso.
az_parecer_publicado() {
  python3 -I -c 'import json,sys
corpo = ""
for l in open(sys.argv[1]):
    m = json.loads(l); c = m["corpo"]
    if "aurumcode-review" in c.get("body", ""): corpo = c["body"]
print(corpo)
' "$TUT_PR_LOG"
}
# az_pr args...: `review --pr` contra o GitHub falso, como o workflow do
# servico a roda: as camadas vem do que o PROPRIO workflow da main declara
# (policy_repository e standards_repository), cada uma montada da main do
# repositorio central correspondente (az_exporta_main), fora da arvore do
# servico. Sem o input, a camada nao entra: e o que MUT-001 do aceite prova.
az_pr() {
  local wf="$STATE/github/$NOME/.github/workflows/code-review.yml" monta=() flags=() repo
  repo="$(sed -n 's/^ *policy_repository: *\([^ ]*\).*/\1/p' "$wf")"
  if [ -n "$repo" ]; then monta+=(-v "$STATE/github/${repo#*/}:/github/policy:ro"); flags+=(--politica /github/policy); fi
  repo="$(sed -n 's/^ *standards_repository: *\([^ ]*\).*/\1/p' "$wf")"
  if [ -n "$repo" ]; then monta+=(-v "$STATE/github/${repo#*/}:/github/standards:ro"); flags+=(--padroes /github/standards); fi
  ui_cmd "aurumcode $* ${flags[*]:-}"
  set +e
  LAST_OUT="$(docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e "AURUMCODE_GITHUB_API_URL=http://127.0.0.1:$TUT_PR_PORT" -e GITHUB_TOKEN=token-falso-local \
    -e AURUMCODE_PR_PERMISSION_MODE=endpoint -e AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json -e AURUMCODE_CACHE_DIR=/tmp/cache \
    "${monta[@]}" -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /app/aurumcode "$TUT_IMAGE" "$@" "${flags[@]}" 2>&1)"
  LAST_RC=$?
  set -e
  printf '%s\n' "$LAST_OUT" | ui_saida
  echo "    exit_code=$LAST_RC"
}
# az_legenda TEXTO: conclusao do roteiro, nao saida do produto. Para cada regra
# da secao "Regras citadas" do parecer (regra e arquivo:linha), a camada de
# origem: o repositorio central (clonado em .estado/) que tem a skill cujo nome
# prefixa a regra; sem skill, e a verificacao deterministica do proprio
# AurumCode.
az_legenda() {
  local linha regra loc prefixo origem achados
  achados="$(printf '%s\n' "$1" | sed 's/^[[:space:]]*│ //' | sed -n '/^#### Regras citadas/,/^#### /p' | grep -E '^- `[^`]+`' || true)"
  if [ -z "$achados" ]; then echo "nenhuma regra citada no parecer" | ui_saida; return 0; fi
  printf '%s\n' "$achados" | while IFS= read -r linha; do
    regra="$(printf '%s' "$linha" | sed -n 's/^- `\([^`]*\)`.*/\1/p')"
    prefixo="${regra%%#*}"
    if [ "$prefixo" != "$regra" ] && [ -f "$SEC_DIR/.aurumcode/skills/$prefixo/SKILL.md" ]; then origem="$NOME_SEC (política de segurança)"
    elif [ "$prefixo" != "$regra" ] && [ -f "$DEV_DIR/.aurumcode/skills/$prefixo/SKILL.md" ]; then origem="$NOME_DEV (padrões de engenharia)"
    elif [ "$prefixo" != "$regra" ] && [ -f "$TUT_WORK/.aurumcode/skills/$prefixo/SKILL.md" ]; then origem="$NOME (o próprio serviço)"
    else origem="verificação determinística do AurumCode, cobrada pelo gate do $NOME_SEC"; fi
    printf '%s\n' "${linha#*:}" | grep -oE '`[^`]+:[0-9]+`' | tr -d '`' | while IFS= read -r loc; do
      printf '%s:  %s → %s\n' "$loc" "$regra" "$origem"
    done
  done | sort -u | ui_saida
}
# az_remoto_real DIR NOME DESCRICAO: liga DIR ao GitHub. Repositorio ja
# existente (criado por voce, vazio): so adiciona o remoto e envia a main.
# Inexistente: cria com gh repo create (publico, ou privado com --privado).
az_remoto_real() {
  local dir="$1" nome="$2" desc="$3"
  if gh repo view "$DONO/$nome" >/dev/null 2>&1; then
    ui_cmd "git -C $(basename "$dir") remote add origin https://github.com/$DONO/$nome.git && git push -u origin main"
    ui_pausa
    git_em "$dir" remote add origin "https://github.com/$DONO/$nome.git"
    git_em "$dir" push -q -u origin main 2>&1 | ui_saida
  else
    ui_cmd "$(cmd_repo_create "$nome" "$(basename "$dir")")"
    ui_pausa
    (cd "$dir" && gh repo create "$DONO/$nome" "$VIS" --source=. --remote=origin --push --description "$desc") 2>&1 | ui_saida
  fi
}
# az_remoto DIR NOME DESCRICAO: a main de DIR vai para o remoto NOME.
# mock: um repositorio bare em .estado/remotos/NOME.git.
az_remoto() {
  if [ "$MODO" = mock ]; then
    ui_cmd "git init --bare ../remotos/$2.git && git remote add origin ../remotos/$2.git && git push -u origin main"; ui_pausa
    mkdir -p "$STATE/remotos"; rm -rf "$STATE/remotos/$2.git"; git init -q --bare -b main "$STATE/remotos/$2.git"
    git_em "$1" remote add origin "$STATE/remotos/$2.git"; git_em "$1" push -q -u origin main
    git_em "$1" remote -v | head -1 | ui_saida
  else
    az_remoto_real "$1" "$2" "$3"
  fi
}
# az_cria_camada PAPEL NOME MENSAGEM: o repositorio central PAPEL (security-policy
# ou devops-policy) nasce de camadas/PAPEL, com o primeiro commit, e vai para o
# remoto NOME. Define <PAPEL>_SHA com a SHA da main, para o workflow fixar.
az_cria_camada() {
  local dir="$STATE/$1"
  rm -rf "$dir"; mkdir -p "$dir"; cp -R "$HERE/camadas/$1/." "$dir/"
  git_em "$dir" init -q -b main; git_em "$dir" add -A; git_em "$dir" commit -q -m "$3"
  az_remoto "$dir" "$2" "POC AurumCode do zero: $1"
}
# az_exporta NOME: a main do remoto falso NOME exportada em .estado/github/NOME,
# como o GitHub a serviria (contents/ para o servico; checkout para as camadas).
az_exporta() {
  rm -rf "$STATE/github/$1"; mkdir -p "$STATE/github/$1"
  git --git-dir "$STATE/remotos/$1.git" archive main | tar -x -C "$STATE/github/$1"
}
# az_exporta_main: as tres mains exportadas; a do servico e a que o GitHub falso
# serve em contents/ (GITHUB_FALSO_ARVORE), como o GitHub serve a base da PR.
az_exporta_main() {
  az_exporta "$NOME_SEC"; az_exporta "$NOME_DEV"; az_exporta "$NOME"
  export GITHUB_FALSO_ARVORE="$STATE/github/$NOME"
}
# az_workflow: .github/workflows/code-review.yml do servico a partir do modelo,
# com a versao do AurumCode e os dois repositorios centrais fixados na SHA
# da main de cada um (uma ref movel mudaria o gate de todo servico sem PR).
az_workflow() {
  local sec_sha dev_sha
  sec_sha="$(git_em "$SEC_DIR" rev-parse main)"; dev_sha="$(git_em "$DEV_DIR" rev-parse main)"
  mkdir -p "$TUT_WORK/.github/workflows"
  sed -e "s|__REF__|$REF|" -e "s|__POLICY_REPO__|$DONO/$NOME_SEC|" -e "s|__POLICY_REF__|$sec_sha|" \
      -e "s|__STANDARDS_REPO__|$DONO/$NOME_DEV|" -e "s|__STANDARDS_REF__|$dev_sha|" \
      "$HERE/camadas/servico/code-review.yml.modelo" > "$TUT_WORK/.github/workflows/code-review.yml"
}
# az_espera_parecer N ANTERIOR: espera o comentario do AurumCode na PR real (ate 15 min).
az_espera_parecer() {
  local i body
  for i in $(seq 1 45); do
    body="$(gh api "repos/$DONO/$NOME/issues/$1/comments" --jq '[.[] | select(.body | contains("aurumcode-review"))] | last | .body // empty' 2>/dev/null || true)"
    if [ -n "$body" ] && [ "$body" != "$2" ]; then printf '%s\n' "$body" | ui_saida; ULTIMO_PARECER="$body"; return 0; fi
    printf '  %s… esperando o workflow (%d/45)%s\r' "$D" "$i" "$N"; sleep 20
  done
  ui_aviso "o parecer não chegou em 15 min; veja: gh pr view $1 --repo $DONO/$NOME --comments"; return 1
}

# ---------------------------------------------------------------- pre-requisitos
TUT_WORK="$STATE/projeto"
if [ "${ACAO#ia}" != "$ACAO" ]; then TUT_WORK="$DESTINO"; fi
if [ "$MODO" = real ] && [ "$MOSTRAR" = 1 ]; then
  DONO='<seu-usuario>'; az_nomes; az_mostrar; exit 0
fi
mkdir -p "$STATE"
if [ "$MODO" = real ]; then
  command -v gh >/dev/null || { echo "modo real: gh ausente" >&2; exit 79; }
  gh auth status >/dev/null 2>&1 || { echo "modo real: gh não autenticado (gh auth login)" >&2; exit 79; }
  if [ "$ACAO" = roteiro ] || [ "$ACAO" = ia ]; then
    if [ -z "${LLM_API_KEY:-}" ] || [ -z "${LLM_BASE_URL:-}" ]; then
      echo "modo real: exporte LLM_API_KEY e LLM_BASE_URL (e LLM_MODEL se o serviço exigir)" >&2; exit 79
    fi
    if [ "$PRIVADO" = 1 ] && { [ -z "${AURUMCODE_POLICY_TOKEN:-}" ] || [ -z "${AURUMCODE_STANDARDS_TOKEN:-}" ]; }; then
      echo "modo real com --privado: exporte AURUMCODE_POLICY_TOKEN e AURUMCODE_STANDARDS_TOKEN (token fine-grained com Contents: read nos dois repositórios centrais; pode ser o mesmo valor). Veja run.sh --modo real --mostrar --privado" >&2; exit 79
    fi
  fi
  DONO="$(gh api user --jq .login)"
  az_nomes
  if [ -z "$REF" ]; then
    if gh api repos/Mpaape/AurumCode/git/ref/tags/v2.0.0 >/dev/null 2>&1; then REF=v2.0.0; else REF=main; fi
  fi
else
  az_nomes
fi
if [ "$ACAO" = roteiro ]; then
printf '%sAurumCode do zero, em camadas%s — três repositórios: a segurança, a engenharia e o serviço, e o primeiro parecer.\n' "$B" "$N"
ui_diz "Modo $MODO. Repositórios: $DONO/$NOME_SEC, $DONO/$NOME_DEV e $DONO/$NOME. Tudo o que roda aparece como comando; ⏎ executa o próximo."
[ "$MODO" = mock ] && ui_diz "Em mock, o GitHub e o modelo são falsos e locais (sem rede, sem segredo). Com --modo real, os mesmos passos valem na sua conta."
fi
if [ "$ACAO" = roteiro ] || [ "$ACAO" = ia ]; then
  printf '\n  %s(construindo ou reaproveitando a imagem do produto a partir do Dockerfile)%s\n' "$D" "$N"
  tut_image >/dev/null
fi

# az_aplica defeito|correcao: o arquivo pronto em etapas/ vira assistente.py do
# projeto; o defeito ganha uma chave aleatoria a cada execucao (nunca versionada).
az_aplica() {
  [ -n "${CHAVE:-}" ] || CHAVE="sk-demo-$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  if [ "$1" = defeito ]; then
    # 3 linhas de cabecalho do modelo saem; os marcadores viram o defeito
    tail -n +4 "$HERE/etapas/defeito.py.modelo" \
      | sed -e "s|^@@LINHA_DA_CHAVE@@\$|API_KEY = \"$CHAVE\"  # so para testar rapido|" -e "s|@@EXECUTA@@|e""xec|" > "$TUT_WORK/assistente.py"
  else
    cp "$HERE/etapas/$1.py" "$TUT_WORK/assistente.py"
  fi
}

# ---------------------------------------------------------------- passos
passo_1() {
  ui_passo 1
  ui_diz "Um projeto pequeno para \"criar algo com IA\": um assistente de terminal que pergunta a um serviço compatível com OpenAI. O boilerplate veio de um agente de IA; aqui ele já está pronto em boilerplate/."
  rm -rf "$TUT_WORK"; mkdir -p "$TUT_WORK"
  ui_cmd "cp -R boilerplate/. $NOME/"; ui_pausa
  cp -R "$HERE/boilerplate/." "$TUT_WORK/"
  (cd "$TUT_WORK" && find . -type f | sort) | ui_saida
  ui_ok "a chave da API é lida do ambiente (ASSISTENTE_API_KEY): é assim que deve continuar."
}
passo_2() {
  ui_passo 2
  ui_diz "O repositório do serviço nasce no git, com o primeiro commit, e ganha um remoto."
  ui_cmd "git init -b main && git add -A && git commit -m 'projeto: assistente'"; ui_pausa
  tgit init -q -b main; tgit add -A; tgit commit -q -m "projeto: assistente"
  tgit log --oneline | ui_saida
  [ "$MODO" = real ] || ui_diz "Em mock, o \"GitHub\" são repositórios bare locais em .estado/remotos/."
  az_remoto "$TUT_WORK" "$NOME" "POC AurumCode do zero: o serviço"
  ui_ok "main do serviço publicada no remoto."
}
passo_3() {
  ui_passo 3
  ui_diz "Quem cuida da segurança não entra serviço por serviço: mantém um repositório só seu, $NOME_SEC, e todo serviço aponta para ele. Nele ficam o gate obrigatório (reprova em error, inconclusivo bloqueia), o idioma do parecer, um ignore de arquivos gerados, a regra IA-001 (a resposta do modelo nunca é executada) e, quando houver, as exceções aprovadas, com dono e validade. É a política central: o serviço obedece e não consegue afrouxá-la."
  ui_cmd "cp -R camadas/security-policy/. $NOME_SEC/ && git -C $NOME_SEC init -b main && git -C $NOME_SEC add -A && git -C $NOME_SEC commit -m 'segurança: gate, IA-001, ignore'"
  ui_pausa
  az_cria_camada security-policy "$NOME_SEC" "segurança: gate, IA-001, ignore"
  { echo "# $NOME_SEC/.aurumcode/config.yml"; grep -vE '^#|^ *#' "$SEC_DIR/.aurumcode/config.yml"; echo; echo "# regras (skills/ia/SKILL.md)"; grep -h '^## \|^severity' "$SEC_DIR"/.aurumcode/skills/*/SKILL.md; } | ui_saida
  ui_ok "política de segurança publicada: nenhum serviço precisa copiar nada dela."
}
passo_4() {
  ui_passo 4
  ui_diz "O time de engenharia faz o mesmo com o que é dele: o repositório $NOME_DEV traz as convenções (PAD-001, argparse para opções; QA-001, comportamento novo tem teste) e os analistas padroes e qa, cada um com o seu foco. É a camada de padrões: só contribui skills e analistas; gate, ignore e exceções ela não pode declarar (um config.yml com qualquer seção é recusado), e o serviço escolhe quais analistas rodam."
  ui_cmd "cp -R camadas/devops-policy/. $NOME_DEV/ && git -C $NOME_DEV init -b main && git -C $NOME_DEV add -A && git -C $NOME_DEV commit -m 'engenharia: PAD-001, QA-001 e analistas'"
  ui_pausa
  az_cria_camada devops-policy "$NOME_DEV" "engenharia: PAD-001, QA-001 e analistas"
  { echo "# $NOME_DEV/.aurumcode/profiles.yml (analistas)"; grep -E '^  - name:|emphasis:' "$DEV_DIR/.aurumcode/profiles.yml"; echo; echo "# regras (skills/time/SKILL.md)"; grep -h '^## \|^severity' "$DEV_DIR"/.aurumcode/skills/*/SKILL.md; echo; echo "# .aurumcode/config.yml: só comentário (a camada não decide gate nem ignore)"; } | ui_saida
  ui_ok "padrões de engenharia publicados; o gate continua com a segurança."
}
passo_5() {
  ui_passo 5
  ui_diz "O serviço só diz o que é dele: idioma, comentários na linha e quais analistas rodam (seguranca, embutido; padroes e qa, do $NOME_DEV). O workflow chama a revisão em toda PR, fixado na versão $REF do AurumCode, e aponta para os dois repositórios centrais com a ref fixa na SHA da main de cada um: o workflow faz o checkout dos dois fora da árvore do serviço e passa --politica e --padroes ao produto."
  ui_cmd "mkdir -p .aurumcode .github/workflows && cp camadas/servico/.aurumcode/config.yml .aurumcode/"
  ui_cmd "sed 's|__POLICY_REPO__|$DONO/$NOME_SEC|; …' camadas/servico/code-review.yml.modelo > .github/workflows/code-review.yml"
  ui_pausa
  mkdir -p "$TUT_WORK/.aurumcode"
  cp "$HERE/camadas/servico/.aurumcode/config.yml" "$TUT_WORK/.aurumcode/config.yml"
  az_workflow
  { echo "# .aurumcode/config.yml"; grep -vE '^#' "$TUT_WORK/.aurumcode/config.yml"; echo; echo "# .github/workflows/code-review.yml"; grep -E 'uses:|_repository:|_ref:|TOKEN' "$TUT_WORK/.github/workflows/code-review.yml" | sed -E 's/ +#.*//'; } | ui_saida
  ui_cmd "git add -A && git commit -m 'aurum: config do serviço e workflow com as duas camadas' && git push"; ui_pausa
  tgit add -A; tgit commit -q -m "aurum: config do serviço e workflow com as duas camadas"; tgit push -q
  tgit log --oneline -1 | ui_saida
  ui_ok "o serviço aponta para as duas camadas; nenhuma regra foi copiada para ele e nenhuma chave entrou em arquivo."
}
passo_6() {
  ui_passo 6
  ui_diz "As credenciais do modelo ficam no cofre de secrets do serviço, nunca em arquivo versionado. O workflow as recebe como \${{ secrets.LLM_API_KEY }} e \${{ secrets.LLM_BASE_URL }}; o modelo, pela variável LLM_MODEL. Para ler os dois repositórios centrais o workflow usa o próprio github.token quando eles são públicos; privados, cada um precisa de um token só de leitura (fine-grained, Contents: read naquele repositório), guardado como AURUMCODE_POLICY_TOKEN e AURUMCODE_STANDARDS_TOKEN. O token nunca chega ao código revisado."
  ui_cmd "$(cmd_secret LLM_API_KEY LLM_API_KEY)"
  ui_cmd "$(cmd_secret LLM_BASE_URL LLM_BASE_URL)"
  ui_cmd "$(cmd_variable LLM_MODEL LLM_MODEL)"
  if [ "$PRIVADO" = 1 ]; then
    ui_cmd "$(cmd_secret AURUMCODE_POLICY_TOKEN AURUMCODE_POLICY_TOKEN)"
    ui_cmd "$(cmd_secret AURUMCODE_STANDARDS_TOKEN AURUMCODE_STANDARDS_TOKEN)"
  else
    ui_diz "(repositórios públicos: AURUMCODE_POLICY_TOKEN e AURUMCODE_STANDARDS_TOKEN não são necessários; com --privado o roteiro os cadastra a partir do ambiente)"
  fi
  if [ "$MODO" = mock ]; then
    ui_aviso "não executado em mock: o modelo é um fixture local e não há GitHub; em --modo real os comandos rodam com os valores do seu ambiente."
  else
    ui_pausa
    gh secret set LLM_API_KEY --repo "$DONO/$NOME" --body "$LLM_API_KEY" 2>&1 | ui_saida
    gh secret set LLM_BASE_URL --repo "$DONO/$NOME" --body "$LLM_BASE_URL" 2>&1 | ui_saida
    [ -z "${LLM_MODEL:-}" ] || gh variable set LLM_MODEL --repo "$DONO/$NOME" --body "$LLM_MODEL" 2>&1 | ui_saida
    if [ "$PRIVADO" = 1 ]; then
      gh secret set AURUMCODE_POLICY_TOKEN --repo "$DONO/$NOME" --body "$AURUMCODE_POLICY_TOKEN" 2>&1 | ui_saida
      gh secret set AURUMCODE_STANDARDS_TOKEN --repo "$DONO/$NOME" --body "$AURUMCODE_STANDARDS_TOKEN" 2>&1 | ui_saida
    fi
    gh secret list --repo "$DONO/$NOME" 2>&1 | ui_saida
    ui_ok "secrets cadastrados; os valores nunca aparecem em log nem em arquivo."
  fi
}
passo_7() {
  ui_passo 7
  ui_diz "Numa branch, um desenvolvedor faz duas coisas com pressa: cola uma chave fixa \"para testar rápido\" e cria a opção --executar, que roda como Python o código que o modelo devolver."
  ui_cmd "git checkout -b feature"
  ui_cmd "cp etapas/defeito.py.modelo assistente.py   # os marcadores viram a chave fixa e a chamada exec"
  ui_cmd "git commit -am 'assistente: --executar roda o código sugerido'"; ui_pausa
  tgit checkout -q -b feature
  az_aplica defeito
  tgit commit -q -am "assistente: --executar roda o código sugerido"
  tgit diff main feature | sed -E 's/sk-demo-[0-9a-f]+/sk-demo-…/' | grep -E '^[-+][^-+]' | ui_saida
  ui_diz "Duas falhas de natureza diferente: a chave fixa (linha 10) qualquer ferramenta acha; executar a resposta do modelo (linha 36) nenhuma regra fixa reconhece — só quem lê o código e a regra IA-001, que mora no $NOME_SEC."
}
passo_8() {
  ui_passo 8
  ui_diz "Antes de abrir a PR, o agente de IA do terminal pergunta ao AurumCode pelo MCP (aurum_gate): a mesma decisão que o CI daria, antes de a PR existir. O dev tem os dois repositórios centrais clonados ao lado do serviço, e o servidor os recebe por AURUMCODE_POLICY e AURUMCODE_STANDARDS, como o workflow os passa na PR. Reprovou? Ele pede a explicação (aurum_explain)."
  ui_pausa
  az_mcp "$(chama 1 aurum_gate '{"base":"main"}')"
  local id=""
  id="$(az_id_de 'ia#' 2>/dev/null || az_id_de 'analysis/hardcoded-secret' 2>/dev/null || true)"
  if [ -n "$id" ]; then
    ui_diz "Os ids valem dentro de uma sessão do servidor: o agente pergunta de novo e pede a explicação na mesma conversa."
    az_mcp "$(chama 2 aurum_gate '{"base":"main"}')" "$(chama 3 aurum_explain "{\"finding_id\":\"$id\"}")"
  fi
  ui_diz "A mesma pergunta pela linha de comando, para quem não usa agente (--politica e --padroes apontam para os clones):"
  az_aurum review --base main
  ui_ok "o gate reprovou antes do PR; a chave ainda não saiu da máquina do dev."
}
passo_9() {
  ui_passo 9
  ui_diz "O dev abre a PR mesmo assim. O workflow faz o checkout dos dois repositórios centrais, roda a revisão e publica o parecer: uma decisão no topo, o que corrigir antes do merge, e a correção aplicável na própria linha. Cada achado diz quem o encontrou: o analista ([perfil …]) e, quando a regra vem da camada de padrões, a camada ([padrões de engenharia])."
  ui_cmd "git push -u origin feature"; ui_pausa
  tgit push -q -u origin feature
  if [ "$MODO" = mock ]; then
    TUT_PR_SHA="$(tgit rev-parse feature)"
    ui_diz "Em mock, um servidor falso da API do GitHub serve o diff da PR #1 e grava o que o produto publica; as duas camadas são a main de cada repositório central, montadas fora da árvore do serviço, como o workflow faz."
    az_exporta_main   # config do servico e as duas camadas vem da main de cada remoto, como no GitHub
    tut_pr_servidor pr
    az_pr review --pr 1 --repo "$DONO/$NOME" --publicar --check --modo-publicacao comments
    ui_diz "O que foi publicado na PR #1:"
    az_publicado
    ui_diz "De onde veio cada achado (conclusão do roteiro, lida dos repositórios clonados):"
    az_legenda "$(az_parecer_publicado)"
  else
    ia_protege_main
    ui_run "abre a PR na sua conta" -- gh pr create --base main --head feature --title "assistente: chave fixa para testar rápido" --body "POC AurumCode do zero: a revisão deve reprovar esta mudança."
    NUM="$(gh pr view feature --repo "$DONO/$NOME" --json number --jq .number)"
    ui_diz "Esperando o workflow publicar o parecer na PR #$NUM…"
    ULTIMO_PARECER=""; az_espera_parecer "$NUM" "" || true
    gh pr checks "$NUM" --repo "$DONO/$NOME" 2>&1 | ui_saida || true
    ui_diz "De onde veio cada achado (conclusão do roteiro, lida dos repositórios clonados):"
    az_legenda "${ULTIMO_PARECER:-}"
  fi
  ui_ok "bloqueado pelas três camadas: a segurança reprova (IA-001), a engenharia observa (PAD-001, QA-001) e o serviço só escolheu os analistas. A chave, porém, já foi enviada ao GitHub na branch: num caso real, revogue-a."
}
passo_10() {
  ui_passo 10
  ui_diz "O dev com pressa tenta o atalho: escreve no config do serviço um gate que só reprova o crítico e um ignore do próprio arquivo (camadas/servico/afrouxa.yml) e roda a revisão de novo."
  ui_cmd "cp camadas/servico/afrouxa.yml .aurumcode/config.yml && git diff .aurumcode/config.yml"; ui_pausa
  cp "$HERE/camadas/servico/afrouxa.yml" "$TUT_WORK/.aurumcode/config.yml"
  tgit diff .aurumcode/config.yml | grep -E '^[-+][^-+]' | ui_saida
  az_aurum review --base main
  ui_diz "A política vence: o gate e o ignore do serviço são ignorados com aviso, e o bloqueio é o mesmo. Na PR nem isso seria lido: a revisão de PR usa a configuração da main do serviço, nunca a da branch. Para destravar, quem muda é a segurança, por um PR no $NOME_SEC."
  ui_cmd "git checkout -- .aurumcode/config.yml"; ui_pausa
  tgit checkout -q -- .aurumcode/config.yml
  ui_ok "nada mudou: o serviço não consegue afrouxar a segurança."
}
passo_11() {
  ui_passo 11
  ui_diz "O dev aplica as duas correções: a chave volta a vir do ambiente e --executar só mostra o código (a sugestão da linha 36). O parecer da PR é editado no lugar, com a nova decisão: um parecer por PR, nunca uma pilha de comentários."
  ui_cmd "cp etapas/correcao.py assistente.py && git commit -am 'assistente: chave pelo ambiente e --executar só mostra' && git push"; ui_pausa
  az_aplica correcao
  tgit commit -q -am "assistente: chave pelo ambiente e --executar só mostra"; tgit push -q
  tgit log --oneline main..feature | ui_saida
  if [ "$MODO" = mock ]; then
    TUT_PR_SHA="$(tgit rev-parse feature)"
    tut_pr_parar; az_exporta_main; tut_pr_servidor pr2
    az_pr review --pr 1 --repo "$DONO/$NOME" --publicar --check --modo-publicacao comments
    ui_diz "O que foi publicado agora (no GitHub real, o mesmo comentário é editado; o servidor falso não guarda o anterior, então aqui aparece um novo):"
    az_publicado; tut_pr_parar
  else
    ui_diz "Esperando o workflow atualizar o parecer da PR #$NUM…"
    az_espera_parecer "$NUM" "$ULTIMO_PARECER" || true
    gh pr checks "$NUM" --repo "$DONO/$NOME" 2>&1 | ui_saida || true
  fi
  ui_ok "aprovado: a correção passou pelas mesmas três camadas que reprovaram a versão anterior; as observações da engenharia seguem abertas, sem bloquear."
}
passo_12() {
  ui_passo 12
  ui_diz "O que ficou: três repositórios. $NOME_SEC (segurança: gate, IA-001, ignore, exceções), $NOME_DEV (engenharia: PAD-001, QA-001, analistas) e $NOME, cujo workflow (fixado em $REF) aponta para os dois com refs fixas. O serviço só tem o que é dele; as credenciais ficaram fora do código; uma PR foi reprovada e depois aprovada pelas mesmas regras, e a tentativa de afrouxar no serviço não mudou nada."
  if [ "$MODO" = mock ]; then
    ui_diz "Serviço em $TUT_WORK; camadas em $SEC_DIR e $DEV_DIR; remotos falsos em $STATE/remotos/. Para fazer de verdade na sua conta: exporte LLM_API_KEY e LLM_BASE_URL e rode  run.sh --modo real --repo NOME  (antes, run.sh --modo real --repo NOME --mostrar lista o que será criado)."
    ui_diz "Para apagar tudo: run.sh limpar"
  else
    ui_diz "Repositórios: https://github.com/$DONO/$NOME_SEC, https://github.com/$DONO/$NOME_DEV e https://github.com/$DONO/$NOME (PR #${NUM:-?}). Para apagar: gh repo delete em cada um (eu nunca apago por você)."
  fi
}

# ---------------------------------------------------------------- modo ia
# ia_mcp_json: .mcp.json do projeto com o AurumCode como servidor MCP, pela
# imagem do produto, com as duas camadas montadas (mock: modelo = fixture, sem
# rede; real: o modelo do ambiente).
ia_mcp_json() {
  python3 -I - "$TUT_WORK/.mcp.json" "$MODO" "$TUT_IMAGE" "$TUT_WORK" "$HERE" "$(id -u):$(id -g)" "$SEC_DIR" "$DEV_DIR" <<'PY'
import json, sys
dest, modo, img, work, here, uid, sec, dev = sys.argv[1:]
args = ["run", "-i", "--rm", "--user", uid, "-e", "HOME=/tmp", "-e", "AURUMCODE_CACHE_DIR=/tmp/cache",
        "-e", "AURUMCODE_POLICY=/github/policy", "-e", "AURUMCODE_STANDARDS=/github/standards",
        "-v", sec + ":/github/policy:ro", "-v", dev + ":/github/standards:ro"]
srv = {"command": "docker"}
if modo == "mock":
    args += ["--network", "none", "-e", "AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json", "-v", here + ":/fixtures:ro"]
else:
    args += ["-e", "LLM_API_KEY", "-e", "LLM_BASE_URL", "-e", "LLM_MODEL"]
    srv["env"] = {k: "${%s:-}" % k for k in ("LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL")}
args += ["-v", work + ":/work", "-w", "/work", "--entrypoint", "/app/aurumcode", img, "mcp"]
srv["args"] = args
with open(dest, "w") as f:
    json.dump({"mcpServers": {"aurum": srv}}, f, indent=2)
    f.write("\n")
PY
}
# ia_cmd_completo: o comando da varredura inicial do modo ia (#178), na mesma
# imagem e com as mesmas camadas do .mcp.json, numa linha para copiar. mock:
# sem rede, modelo = fixture; real: o modelo do ambiente.
ia_cmd_completo() {
  local modelo
  if [ "$MODO" = mock ]; then
    modelo="--network none -e AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json -v $HERE:/fixtures:ro"
  else
    modelo="-e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL"
  fi
  printf 'docker run --rm --user %s -e HOME=/tmp %s -v %s:/github/policy:ro -v %s:/github/standards:ro -v %s:/work -w /work --entrypoint /app/aurumcode %s review --completo --seguranca --sarif /tmp/inicial.sarif %s' \
    "$(id -u):$(id -g)" "$modelo" "$SEC_DIR" "$DEV_DIR" "$TUT_WORK" "$TUT_IMAGE" "${CAMADAS_FLAGS[*]}"
}
ia_preparar() {
  if [ -e "$TUT_WORK" ] && [ -n "$(ls -A "$TUT_WORK" 2>/dev/null)" ]; then
    echo "o destino $TUT_WORK já existe e não está vazio; use --destino OUTRO ou: run.sh limpar --destino $TUT_WORK" >&2; exit 2
  fi
  printf '%sAurumCode com um agente de IA, em camadas%s  %smodo %s · %s%s\n\n' "$B" "$N" "$D" "$MODO" "$TUT_WORK" "$N"
  ui_diz "Os outros times já fizeram a parte deles: o roteiro cria os dois repositórios centrais; o agente só configura o serviço para apontar para eles."
  AUTO=1 az_cria_camada security-policy "$NOME_SEC" "segurança: gate, IA-001, ignore" >/dev/null
  AUTO=1 az_cria_camada devops-policy "$NOME_DEV" "engenharia: PAD-001, QA-001 e analistas" >/dev/null
  ui_ok "$DONO/$NOME_SEC (segurança: gate, IA-001, ignore, exceções) e $DONO/$NOME_DEV (engenharia: PAD-001, QA-001, analistas padroes e qa) publicados; clones em $SEC_DIR e $DEV_DIR."
  mkdir -p "$TUT_WORK"; cp -R "$HERE/boilerplate/." "$TUT_WORK/"
  tgit init -q -b main; : > "$TUT_WORK/.git/aurum-poc"
  tgit add -A; tgit commit -q -m "projeto: assistente"
  ui_ok "serviço criado e commitado (sem AurumCode ainda):"
  (cd "$TUT_WORK" && git ls-files) | ui_saida
  if [ "$MODO" = real ]; then AUTO=1 az_remoto_real "$TUT_WORK" "$NOME" "POC AurumCode do zero: o serviço"; fi
  ia_mcp_json
  ui_ok "AurumCode registrado como servidor MCP \"aurum\" em .mcp.json, com as duas camadas montadas (Claude Code lê na abertura; fora do git):"
  sed 's/^/    /' "$TUT_WORK/.mcp.json" | head -40
  printf '\n  %sCodex:%s o mesmo servidor em ~/.codex/config.toml:\n' "$B" "$N"
  python3 -I -c 'import json,sys; s=json.load(open(sys.argv[1]))["mcpServers"]["aurum"]; print("    [mcp_servers.aurum]\n    command = \"docker\"\n    args = " + json.dumps(s["args"]))' "$TUT_WORK/.mcp.json"
  local pedido="$STATE/pedido-ia.txt" sec_sha dev_sha
  sec_sha="$(git_em "$SEC_DIR" rev-parse main)"; dev_sha="$(git_em "$DEV_DIR" rev-parse main)"
  cat > "$pedido" <<PEDIDO
Quero instalar e configurar o AurumCode neste repositório, por conversa.
Siga o roteiro para o agente de $REPO_ROOT/docs/tutorials/instalacao-ia.md
(o mesmo guia está em https://mpaape.github.io/AurumCode/tutorials/instalacao-ia/).
Primeiro inspecione o projeto. Faça uma pergunta por vez, com opções curtas e
uma recomendação explicada. Não peça chaves no chat: oriente como cadastrá-las.
As regras e os analistas NÃO ficam neste serviço: o time de segurança mantém
$DONO/$NOME_SEC (gate, regra IA-001, ignore, exceções) e o de engenharia mantém
$DONO/$NOME_DEV (skills PAD-001 e QA-001, analistas padroes e qa). Aponte o
serviço para os dois: no workflow, policy_repository: $DONO/$NOME_SEC com
policy_ref: $sec_sha e standards_repository: $DONO/$NOME_DEV com
standards_ref: $dev_sha. No .aurumcode/config.yml só o que é do serviço
(review.language: pt-BR e review.profiles: [seguranca, padroes, qa]); nada de
gate, ignore nem cópia de skill aqui.
O servidor MCP "aurum" já está registrado neste projeto e já recebe as duas
camadas (clones em $SEC_DIR e $DEV_DIR): depois de cada commit, verifique com
aurum_gate (base main). Para PRs, fixe o workflow reutilizável em
Mpaape/AurumCode/.github/workflows/review.yml@$REF (a tag v2.0.0 passa a ser a
referência fixa quando for publicada). Revisão em português (pt-BR).
Este é um repositório de demonstração: você pode commitar e fazer push na main
(a revisão da PR lê a configuração da main) e abrir PR quando eu pedir.
Comece pelo diagnóstico e pela primeira pergunta.
PEDIDO
  printf '\n%sPróximos passos%s\n' "$B" "$N"
  if [ "$MODO" = real ]; then
    ui_diz "1. Neste terminal (LLM_API_KEY e LLM_BASE_URL exportadas: o servidor MCP usa o modelo daqui), abra o agente no projeto (claude ou codex):"
  else
    ui_diz "1. Abra o agente no projeto (claude ou codex):"
  fi
  ui_copia "cd $TUT_WORK && claude"
  ui_diz "2. Cole o pedido (o mesmo texto está no arquivo: cat $pedido):"
  sed 's/^/      │ /' "$pedido"
  ui_diz "3. Com o Aurum configurado e commitado na main, rode a varredura inicial: o código que já existia nunca passa por uma PR, então o repositório inteiro é revisado uma vez, com as duas camadas (o histórico só é varrido por segredos com o gitleaks em quality_gates.scanners):"
  ui_copia "$(ia_cmd_completo)"
  ui_diz "4. Quando o Aurum estiver configurado, commitado e (no GitHub) na main, simule o dev com pressa, em outro terminal:"
  ui_copia "bash $HERE/run.sh ia-defeito --destino $TUT_WORK"
  if [ "$MODO" = real ]; then
    ui_diz "5. Peça a PR com o defeito (a main fica protegida pelo ia-defeito: merge só com o Aurum aprovando):"
    printf '      │ %s\n' "Faça push da branch feature e abra a PR para a main, sem corrigir nada ainda."
    ui_diz "   O workflow leva alguns minutos (gh pr checks --watch): o parecer chega \"Bloqueado: 2 problemas\", com os achados da engenharia marcados [padrões de engenharia], e o botão de merge fica travado."
    ui_diz "6. Peça a correção guiada pelo parecer:"
    printf '      │ %s\n' "Leia o parecer do AurumCode na PR, confirme com o Aurum (aurum_gate, base main), explique cada problema em uma frase e de que repositório veio a regra, corrija, consulte de novo e faça push."
    ui_diz "   O mesmo parecer é editado para \"Aprovado\" e o merge é liberado."
    ui_diz "Os secrets do serviço o agente só orienta; você digita (cada um pede o valor):"
    ui_copia "gh secret set LLM_API_KEY && gh secret set LLM_BASE_URL && gh variable set LLM_MODEL"
    [ "$PRIVADO" = 0 ] || ui_copia "gh secret set AURUMCODE_POLICY_TOKEN && gh secret set AURUMCODE_STANDARDS_TOKEN   # leitura dos dois repositórios privados"
  else
    ui_diz "5. Peça ao agente:"
    printf '      │ %s\n' "Mudei o assistente na branch feature. Antes de abrir a PR, pergunte ao Aurum (aurum_gate, base main), explique o que ele bloqueou e de que repositório veio cada regra, e corrija; depois consulte de novo."
  fi
  ui_diz "Sem tempo para o agente corrigir:"
  ui_copia "bash $HERE/run.sh ia-correcao --destino $TUT_WORK"
}
ia_defeito() {
  [ -d "$TUT_WORK/.git" ] || { echo "projeto não encontrado em $TUT_WORK (rode run.sh ia antes)" >&2; exit 2; }
  if ! tgit cat-file -e main:.aurumcode/config.yml 2>/dev/null; then
    ui_aviso "a main ainda não tem .aurumcode/config.yml: peça ao agente para commitar a configuração na main antes."
    exit 2
  fi
  if ! tgit show main:.github/workflows/code-review.yml 2>/dev/null | grep -qE '^ *(policy|standards)_repository:'; then
    ui_aviso "o workflow da main não aponta para os repositórios centrais (policy_repository e standards_repository): sem eles a PR é revisada sem a política e sem os padrões."
  fi
  if tgit remote get-url origin >/dev/null 2>&1; then
    tgit fetch -q origin main 2>/dev/null || true
    if ! tgit cat-file -e origin/main:.aurumcode/config.yml 2>/dev/null; then
      ui_aviso "o GitHub ainda não tem a configuração na main (a PR lê a config de lá): peça ao agente o push da main antes."
      exit 2
    fi
  fi
  tgit checkout -q main
  ia_protege_main
  if tgit rev-parse -q --verify feature >/dev/null; then tgit checkout -q feature; else tgit checkout -q -b feature; fi
  az_aplica defeito
  tgit add assistente.py; tgit commit -q -m "assistente: --executar roda o código sugerido"
  ui_ok "branch feature com a mudança (chave fixa na linha 10, exec da resposta do modelo na linha 36):"
  tgit diff main feature -- assistente.py | sed -E 's/sk-demo-[0-9a-f]+/sk-demo-…/' | grep -E '^[-+][^-+]' | ui_saida
}
# ia_protege_main: no GitHub real, a main passa a exigir o status
# aurumcode/policy-gate (inclusive para administradores): o botao de merge fica
# bloqueado enquanto o Aurum nao aprovar. Aplicada so depois que a configuracao
# esta na main, porque a protecao tambem recusa push direto sem o status.
ia_protege_main() {
  local url alvo
  url="$(tgit remote get-url origin 2>/dev/null || true)"
  alvo="${url#https://github.com/}"; alvo="${alvo#git@github.com:}"
  [ "$alvo" != "$url" ] || return 0
  alvo="${alvo%.git}"
  printf '{"required_status_checks":{"strict":false,"contexts":["aurumcode/policy-gate"]},"enforce_admins":true,"required_pull_request_reviews":null,"restrictions":null}' \
    | gh api -X PUT "repos/$alvo/branches/main/protection" --input - >/dev/null 2>&1 \
    && ui_ok "main de $alvo protegida: merge só com aurumcode/policy-gate aprovado" \
    || ui_aviso "não consegui proteger a main de $alvo (conta sem permissão de administrador?): o merge não fica bloqueado"
}
ia_correcao() {
  [ -d "$TUT_WORK/.git" ] || { echo "projeto não encontrado em $TUT_WORK" >&2; exit 2; }
  tgit checkout -q feature
  az_aplica correcao
  tgit add assistente.py; tgit commit -q -m "assistente: chave pelo ambiente e --executar só mostra"
  ui_ok "correção commitada na feature:"; tgit log --oneline -1 | ui_saida
}

# az_roteiro: os doze passos. O servidor falso da PR morre com o roteiro, mesmo
# que um passo falhe no meio (senao ele seguraria a gravacao aberta).
az_roteiro() {
  trap 'tut_pr_parar' EXIT
  passo_1; passo_2; passo_3; passo_4; passo_5; passo_6; passo_7; passo_8; passo_9; passo_10; passo_11; passo_12
}
# az_normaliza: na gravacao, os valores volateis viram a forma (como
# _lib/normaliza.sed faz nos tutoriais): o caminho absoluto desta copia, a
# chave da demonstracao, SHAs de commit e ids de achado. O resto fica literal.
az_normaliza() {
  local aqui
  aqui="$(printf '%s' "$HERE" | sed 's/[][\.*^$/]/\\&/g')"
  sed -E -e 's/\x1b\[[0-9;]*m//g' -e "s#$aqui#demo/do-zero#g" \
    -e 's/sk-demo-[0-9a-f]{6,}/sk-demo-…/g' -e 's/\b[0-9a-f]{40}\b/<sha>/g' \
    -e 's/(bloqueia|aurum_explain): [0-9a-f]{12} /\1: <id> /g' -e 's/^([[:space:]]*│ )([0-9a-f]{7}) /\1<sha7> /'
}
# az_roteiro_gravado: o roteiro mock sem pausas e com a saida gravada em
# out/roteiro-mock.log (normalizada) e a imagem em out/.imagem: a evidencia
# que run.sh --check e tests/acceptance/AUR-614.sh conferem.
az_roteiro_gravado() {
  local bruto="$STATE/roteiro-mock.bruto.log"
  mkdir -p "$OUT"
  az_roteiro 2>&1 | tee "$bruto"
  az_normaliza < "$bruto" > "$OUT/roteiro-mock.log"
  tut_grava_imagem
  echo "gravado: out/roteiro-mock.log e out/.imagem (run.sh --check confere)"
}

if [ "$ACAO" = ia ]; then ia_preparar
elif [ "$ACAO" = ia-defeito ]; then ia_defeito
elif [ "$ACAO" = ia-correcao ]; then ia_correcao
elif [ "$MODO" = mock ] && [ "$AUTO" = 1 ]; then az_roteiro_gravado
else az_roteiro
fi
