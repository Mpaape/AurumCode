#!/usr/bin/env bash
# POC interativa: do zero ao primeiro parecer do AurumCode, passo a passo, no
# terminal. Veja README.md.
#
#   run.sh [--modo mock|real] [--auto] [--repo NOME] [--privado] [--ref REF]
#       roteiro guiado: os 9 passos, cada comando mostrado e executado no ⏎
#   run.sh ia [--modo mock|real] [--destino DIR] [--repo NOME] [--privado] [--ref REF]
#       so prepara o projeto (boilerplate + git) e registra o AurumCode como
#       servidor MCP (.mcp.json); imprime o pedido para o seu agente de IA
#       configurar o Aurum por conversa (docs/tutorials/instalacao-ia.md)
#   run.sh ia-defeito [--destino DIR]   a mudanca com defeito, numa branch
#   run.sh ia-correcao [--destino DIR]  a correcao, na mesma branch
#   run.sh limpar
#
# mock (padrao): tudo local. O "GitHub" e um repositorio bare em .estado/ e um
#   servidor falso da API em 127.0.0.1 (demo/tutoriais/_lib/github-falso.py); o
#   modelo e o fixture deterministico fixture-llm.json; sem rede, sem segredo.
# real: gh autenticado na sua conta, repositorio NOVO no GitHub (--repo NOME),
#   secrets reais lidos do ambiente (LLM_API_KEY, LLM_BASE_URL e, opcional,
#   LLM_MODEL), PR real revisada pelo workflow; o gate local e o agente usam o
#   mesmo modelo. --ref fixa a versao do AurumCode no workflow (padrao: v2.0.0
#   se a tag existir no GitHub, senao main). --privado cria o repositorio privado.
#
# No host: bash, git, docker, python3 (e gh no modo real). O aurumcode roda na
# imagem do produto, construida do Dockerfile da raiz na primeira execucao.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../tutoriais/_lib/tutorial.sh
. "$HERE/../tutoriais/_lib/tutorial.sh"
# shellcheck source=../tutoriais/_lib/pr.sh
. "$HERE/../tutoriais/_lib/pr.sh"

MODO=mock AUTO=0 NOME="" PRIVADO=0 REF="${AZ_REF:-}" ACAO=roteiro DESTINO=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    ia|ia-defeito|ia-correcao) ACAO="$1"; shift ;;
    --destino) DESTINO="$2"; shift 2 ;;
    --modo) MODO="$2"; shift 2 ;;
    --auto) AUTO=1; shift ;;
    --repo) NOME="$2"; shift 2 ;;
    --privado) PRIVADO=1; shift ;;
    --ref) REF="$2"; shift 2 ;;
    limpar) ACAO=limpar; shift ;;
    *) echo "uso: run.sh [ia|ia-defeito|ia-correcao] [--modo mock|real] [--auto] [--destino DIR] [--repo NOME] [--privado] [--ref REF] | limpar" >&2; exit 2 ;;
  esac
done
if [ "$MODO" != mock ] && [ "$MODO" != real ]; then echo "--modo: mock ou real" >&2; exit 2; fi
NOME_DIR="${NOME#*/}"; [ -n "$DESTINO" ] || DESTINO="$HOME/aurum-poc/${NOME_DIR:-assistente}"
if [ "$ACAO" = limpar ]; then
  rm -rf "$STATE"; echo "apagado: $STATE"
  if [ -f "$DESTINO/.git/aurum-poc" ]; then rm -rf "$DESTINO"; echo "apagado: $DESTINO"; fi
  exit 0
fi

# ---------------------------------------------------------------- interface
ESC="$(printf '\033')"
if [ -t 1 ]; then B="$ESC[1m" D="$ESC[2m" C="$ESC[36m" G="$ESC[32m" Y="$ESC[33m" N="$ESC[0m"; else B="" D="" C="" G="" Y="" N=""; fi
PASSOS=("Projeto: o boilerplate" "Git e o remoto" "AurumCode no repositório" "Secrets, fora do código"
  "Uma mudança com defeito" "O agente pergunta antes do PR" "PR: parecer bloqueado" "Correção: parecer aprovado" "Resumo")
PASSO=0
ui_passo() {
  PASSO="$1"
  local i
  printf '\n%s════════════════════════════════════════════════════════════════════════%s\n' "$D" "$N"
  printf '%sAurumCode do zero%s  %smodo %s%s\n' "$B" "$N" "$D" "$MODO" "$N"
  for i in "${!PASSOS[@]}"; do
    if [ "$((i + 1))" -lt "$PASSO" ]; then printf '  %s✔ %d. %s%s\n' "$G" "$((i + 1))" "${PASSOS[$i]}" "$N"
    elif [ "$((i + 1))" -eq "$PASSO" ]; then printf '  %s▶ %d. %s%s\n' "$B" "$((i + 1))" "${PASSOS[$i]}" "$N"
    else printf '  %s  %d. %s%s\n' "$D" "$((i + 1))" "${PASSOS[$i]}" "$N"; fi
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

# ---------------------------------------------------------------- produto
# az_aurum args...: o aurumcode sobre o projeto. mock: sem rede, modelo falso.
# real: com rede e o modelo do ambiente (LLM_API_KEY, LLM_BASE_URL, LLM_MODEL).
az_aurum() {
  ui_cmd "aurumcode $*"
  set +e
  if [ "$MODO" = mock ]; then
    LAST_OUT="$(aurum_raw -e AURUMCODE_CACHE_DIR=/tmp/cache -- "$@" 2>&1)"
  else
    LAST_OUT="$(docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp \
      -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL -e AURUMCODE_CACHE_DIR=/tmp/cache \
      -v "$TUT_WORK:/work" -w /work --entrypoint /app/aurumcode "$TUT_IMAGE" "$@" 2>&1)"
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
# az_mcp REQUISICAO...: o agente fala com `aurumcode mcp` por stdio e resume as respostas.
az_mcp() {
  local reqs="$STATE/mcp-requisicoes.jsonl" resp="$STATE/mcp-respostas.jsonl" r
  { printf '%s\n' "$INICIO" "$INICIADO"; for r in "$@"; do printf '%s\n' "$r"; done; } > "$reqs"
  ui_cmd "aurumcode mcp   # o agente manda initialize + $# requisição(ões) por stdio"
  set +e
  if [ "$MODO" = mock ]; then
    aurum_raw --interactive=true --init=false -- mcp < "$reqs" > "$resp" 2> "$STATE/mcp-stderr.txt"
  else
    docker run --rm -i --user "$(id -u):$(id -g)" -e HOME=/tmp -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL \
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
# az_pr args...: `review --pr` contra o GitHub falso. Sem GITHUB_SHA, a config
# (idioma, gate) vem do checkout do projeto, como num workflow do repositorio.
az_pr() {
  ui_cmd "aurumcode $*"
  set +e
  LAST_OUT="$(docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e "AURUMCODE_GITHUB_API_URL=http://127.0.0.1:$TUT_PR_PORT" -e GITHUB_TOKEN=token-falso-local \
    -e AURUMCODE_PR_PERMISSION_MODE=endpoint -e AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json -e AURUMCODE_CACHE_DIR=/tmp/cache \
    -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /app/aurumcode "$TUT_IMAGE" "$@" 2>&1)"
  LAST_RC=$?
  set -e
  printf '%s\n' "$LAST_OUT" | ui_saida
  echo "    exit_code=$LAST_RC"
}
# az_remoto_real: liga o projeto ao GitHub. Repositorio ja existente (criado por
# voce, vazio, com os secrets cadastrados): so adiciona o remoto e envia a main.
# Inexistente: cria com gh repo create. --repo aceita NOME ou DONO/NOME.
az_remoto_real() {
  local alvo="$NOME" vis=--public
  [ "${alvo#*/}" != "$alvo" ] || alvo="$DONO/$alvo"
  [ "$PRIVADO" = 1 ] && vis=--private
  if gh repo view "$alvo" >/dev/null 2>&1; then
    ui_cmd "git remote add origin https://github.com/$alvo.git && git push -u origin main"
    ui_pausa
    tgit remote add origin "https://github.com/$alvo.git"
    tgit push -q -u origin main 2>&1 | ui_saida
  else
    ui_cmd "gh repo create $alvo $vis --source=. --remote=origin --push"
    ui_pausa
    (cd "$TUT_WORK" && gh repo create "$alvo" "$vis" --source=. --remote=origin --push --description "POC: AurumCode do zero") 2>&1 | ui_saida
  fi
  DONO="${alvo%%/*}"; NOME="${alvo#*/}"
}
# az_exporta_main: a main do remoto falso exportada para o GitHub falso servir
# contents/ (GITHUB_FALSO_ARVORE), como o GitHub serve a base da PR.
az_exporta_main() {
  rm -rf "$STATE/remoto-main"; mkdir -p "$STATE/remoto-main"
  git --git-dir "$STATE/remoto.git" archive main | tar -x -C "$STATE/remoto-main"
  export GITHUB_FALSO_ARVORE="$STATE/remoto-main"
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
TUT_WORK="$STATE/projeto"; mkdir -p "$STATE"
if [ "${ACAO#ia}" != "$ACAO" ]; then TUT_WORK="$DESTINO"; fi
if [ "$MODO" = real ]; then
  command -v gh >/dev/null || { echo "modo real: gh ausente" >&2; exit 79; }
  gh auth status >/dev/null 2>&1 || { echo "modo real: gh não autenticado (gh auth login)" >&2; exit 79; }
  if [ "$ACAO" = roteiro ] || [ "$ACAO" = ia ]; then
    if [ -z "${LLM_API_KEY:-}" ] || [ -z "${LLM_BASE_URL:-}" ]; then
      echo "modo real: exporte LLM_API_KEY e LLM_BASE_URL (e LLM_MODEL se o serviço exigir)" >&2; exit 79
    fi
  fi
  DONO="$(gh api user --jq .login)"
  [ -n "$NOME" ] || NOME="aurum-do-zero-$(date +%Y%m%d-%H%M)"
  if [ -z "$REF" ]; then
    if gh api repos/Mpaape/AurumCode/git/ref/tags/v2.0.0 >/dev/null 2>&1; then REF=v2.0.0; else REF=main; fi
  fi
else
  DONO=voce; [ -n "$NOME" ] || NOME=assistente; [ -n "$REF" ] || REF=main
fi
if [ "$ACAO" = roteiro ]; then
printf '%sAurumCode do zero%s — um projeto novo, o Aurum configurado passo a passo e o primeiro parecer.\n' "$B" "$N"
ui_diz "Modo $MODO. Projeto: $DONO/$NOME. Tudo o que roda aparece como comando; ⏎ executa o próximo."
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
  ui_diz "O repositório nasce no git, com o primeiro commit, e ganha um remoto."
  ui_cmd "git init -b main && git add -A && git commit -m 'projeto: assistente'"; ui_pausa
  tgit init -q -b main; tgit add -A; tgit commit -q -m "projeto: assistente"
  tgit log --oneline | ui_saida
  if [ "$MODO" = mock ]; then
    ui_diz "Em mock, o \"GitHub\" é um repositório bare local."
    ui_cmd "git init --bare ../remoto.git && git remote add origin ../remoto.git && git push -u origin main"; ui_pausa
    rm -rf "$STATE/remoto.git"; git init -q --bare -b main "$STATE/remoto.git"
    tgit remote add origin "$STATE/remoto.git"; tgit push -q -u origin main
    tgit remote -v | head -1 | ui_saida
  else
    az_remoto_real
  fi
  ui_ok "main publicada no remoto."
}
passo_3() {
  ui_passo 3
  ui_diz "Três arquivos ligam o AurumCode: a config (idioma e gate), uma skill com as regras do time em texto (IA-001: resposta do modelo nunca é executada) e o workflow que chama a revisão em toda PR, fixado na versão $REF."
  ui_cmd "mkdir -p .aurumcode/skills/ia .github/workflows"
  ui_cmd "cp aurum/config.yml .aurumcode/config.yml"
  ui_cmd "cp aurum/skills/ia/SKILL.md .aurumcode/skills/ia/SKILL.md"
  ui_cmd "cp docs/site/workflow.yml .github/workflows/code-review.yml   # uses: ...review.yml@$REF"
  ui_pausa
  mkdir -p "$TUT_WORK/.aurumcode/skills/ia" "$TUT_WORK/.github/workflows"
  cp "$HERE/aurum/config.yml" "$TUT_WORK/.aurumcode/config.yml"
  cp "$HERE/aurum/skills/ia/SKILL.md" "$TUT_WORK/.aurumcode/skills/ia/SKILL.md"
  sed -E "s|(review\.yml)@[^[:space:]]+|\1@$REF|" "$REPO_ROOT/docs/site/workflow.yml" > "$TUT_WORK/.github/workflows/code-review.yml"
  { echo "# .aurumcode/config.yml"; cat "$TUT_WORK/.aurumcode/config.yml"; echo; echo "# .aurumcode/skills/ia/SKILL.md"; sed -n '/^## /,$p' "$TUT_WORK/.aurumcode/skills/ia/SKILL.md"; echo; echo "# .github/workflows/code-review.yml"; grep -E 'uses:|LLM_' "$TUT_WORK/.github/workflows/code-review.yml"; } | ui_saida
  ui_cmd "git add -A && git commit -m 'aurum: config, skill do time e workflow' && git push"; ui_pausa
  tgit add -A; tgit commit -q -m "aurum: config, skill do time e workflow"; tgit push -q
  tgit log --oneline -1 | ui_saida
  ui_ok "o Aurum está no repositório; nenhuma chave entrou em arquivo."
}
passo_4() {
  ui_passo 4
  ui_diz "As credenciais do modelo ficam no cofre de secrets do repositório, nunca em arquivo versionado. O workflow as recebe como \${{ secrets.LLM_API_KEY }} e \${{ secrets.LLM_BASE_URL }}; o modelo, pela variável LLM_MODEL."
  ui_cmd "gh secret set LLM_API_KEY --repo $DONO/$NOME --body \"\$LLM_API_KEY\""
  ui_cmd "gh secret set LLM_BASE_URL --repo $DONO/$NOME --body \"\$LLM_BASE_URL\""
  ui_cmd "gh variable set LLM_MODEL --repo $DONO/$NOME --body \"\$LLM_MODEL\""
  if [ "$MODO" = mock ]; then
    ui_aviso "não executado em mock: o modelo é um fixture local; em --modo real os três comandos rodam com os valores do seu ambiente."
  else
    ui_pausa
    gh secret set LLM_API_KEY --repo "$DONO/$NOME" --body "$LLM_API_KEY" 2>&1 | ui_saida
    gh secret set LLM_BASE_URL --repo "$DONO/$NOME" --body "$LLM_BASE_URL" 2>&1 | ui_saida
    [ -z "${LLM_MODEL:-}" ] || gh variable set LLM_MODEL --repo "$DONO/$NOME" --body "$LLM_MODEL" 2>&1 | ui_saida
    gh secret list --repo "$DONO/$NOME" 2>&1 | ui_saida
    ui_ok "secrets cadastrados; os valores nunca aparecem em log nem em arquivo."
  fi
}
passo_5() {
  ui_passo 5
  ui_diz "Numa branch, um desenvolvedor faz duas coisas com pressa: cola uma chave fixa \"para testar rápido\" e cria a opção --executar, que roda como Python o código que o modelo devolver."
  ui_cmd "git checkout -b feature"
  ui_cmd "cp etapas/defeito.py.modelo assistente.py   # os marcadores viram a chave fixa e a chamada exec"
  ui_cmd "git commit -am 'assistente: --executar roda o código sugerido'"; ui_pausa
  tgit checkout -q -b feature
  az_aplica defeito
  tgit commit -q -am "assistente: --executar roda o código sugerido"
  tgit diff main feature | sed -E 's/sk-demo-[0-9a-f]+/sk-demo-…/' | grep -E '^[-+][^-+]' | ui_saida
  ui_diz "Duas falhas de natureza diferente: a chave fixa (linha 10) qualquer ferramenta acha; executar a resposta do modelo (linha 36) nenhuma regra fixa reconhece — só quem lê o código e a regra IA-001 do time."
}
passo_6() {
  ui_passo 6
  ui_diz "Antes de abrir a PR, o agente de IA do terminal pergunta ao AurumCode pelo MCP (aurum_gate): a mesma decisão que o CI daria, antes de a PR existir. Reprovou? Ele pede a explicação (aurum_explain)."
  ui_pausa
  az_mcp "$(chama 1 aurum_gate '{"base":"main"}')"
  local id=""
  id="$(az_id_de 'ia#' 2>/dev/null || az_id_de 'analysis/hardcoded-secret' 2>/dev/null || true)"
  if [ -n "$id" ]; then
    ui_diz "Os ids valem dentro de uma sessão do servidor: o agente pergunta de novo e pede a explicação na mesma conversa."
    az_mcp "$(chama 2 aurum_gate '{"base":"main"}')" "$(chama 3 aurum_explain "{\"finding_id\":\"$id\"}")"
  fi
  ui_diz "A mesma pergunta pela linha de comando, para quem não usa agente:"
  az_aurum review --base main
  ui_ok "o gate reprovou antes do PR; a chave ainda não saiu da máquina do dev."
}
passo_7() {
  ui_passo 7
  ui_diz "O dev abre a PR mesmo assim. O workflow roda a revisão e publica o parecer: uma decisão no topo, o que corrigir antes do merge, e a correção aplicável na própria linha."
  ui_cmd "git push -u origin feature"; ui_pausa
  tgit push -q -u origin feature
  if [ "$MODO" = mock ]; then
    TUT_PR_SHA="$(tgit rev-parse feature)"
    ui_diz "Em mock, um servidor falso da API do GitHub serve o diff da PR #1 e grava o que o produto publica."
    az_exporta_main   # skills e config da PR vêm da main do remoto, como no GitHub
    tut_pr_servidor pr
    az_pr review --pr 1 --repo "$DONO/$NOME" --publicar --check --modo-publicacao comments
    ui_diz "O que foi publicado na PR #1:"
    az_publicado
  else
    ia_protege_main
    ui_run "abre a PR na sua conta" -- gh pr create --base main --head feature --title "assistente: chave fixa para testar rápido" --body "POC AurumCode do zero: a revisão deve reprovar esta mudança."
    NUM="$(gh pr view feature --repo "$DONO/$NOME" --json number --jq .number)"
    ui_diz "Esperando o workflow publicar o parecer na PR #$NUM…"
    ULTIMO_PARECER=""; az_espera_parecer "$NUM" "" || true
    gh pr checks "$NUM" --repo "$DONO/$NOME" 2>&1 | ui_saida || true
  fi
  ui_ok "bloqueado: nada disso entra no main. A chave, porém, já foi enviada ao GitHub na branch: num caso real, revogue-a."
}
passo_8() {
  ui_passo 8
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
  ui_ok "aprovado: a correção passou pelas mesmas camadas que reprovaram a versão anterior."
}
passo_9() {
  ui_passo 9
  ui_diz "O que ficou: um projeto com o AurumCode ligado (.aurumcode/config.yml, a skill do time, o workflow fixado em $REF), as credenciais fora do código, e uma PR que foi reprovada e depois aprovada pelas mesmas regras."
  if [ "$MODO" = mock ]; then
    ui_diz "Projeto em $TUT_WORK (remoto falso em $STATE/remoto.git). Para fazer de verdade na sua conta: exporte LLM_API_KEY e LLM_BASE_URL e rode  run.sh --modo real --repo NOME"
    ui_diz "Para apagar tudo: run.sh limpar"
  else
    ui_diz "Repositório: https://github.com/$DONO/$NOME  (PR #${NUM:-?}). Para apagar: gh repo delete $DONO/$NOME  (eu nunca apago por você)."
  fi
}

# ---------------------------------------------------------------- modo ia
# ia_mcp_json: .mcp.json do projeto com o AurumCode como servidor MCP, pela
# imagem do produto (mock: modelo = fixture, sem rede; real: o modelo do ambiente).
ia_mcp_json() {
  python3 -I - "$TUT_WORK/.mcp.json" "$MODO" "$TUT_IMAGE" "$TUT_WORK" "$HERE" "$(id -u):$(id -g)" <<'PY'
import json, sys
dest, modo, img, work, here, uid = sys.argv[1:]
args = ["run", "-i", "--rm", "--user", uid, "-e", "HOME=/tmp", "-e", "AURUMCODE_CACHE_DIR=/tmp/cache"]
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
ia_preparar() {
  if [ -e "$TUT_WORK" ] && [ -n "$(ls -A "$TUT_WORK" 2>/dev/null)" ]; then
    echo "o destino $TUT_WORK já existe e não está vazio; use --destino OUTRO ou: run.sh limpar --destino $TUT_WORK" >&2; exit 2
  fi
  printf '%sAurumCode com um agente de IA%s  %smodo %s · %s%s\n\n' "$B" "$N" "$D" "$MODO" "$TUT_WORK" "$N"
  mkdir -p "$TUT_WORK"; cp -R "$HERE/boilerplate/." "$TUT_WORK/"
  tgit init -q -b main; : > "$TUT_WORK/.git/aurum-poc"
  tgit add -A; tgit commit -q -m "projeto: assistente"
  ui_ok "projeto criado e commitado (sem AurumCode ainda):"
  (cd "$TUT_WORK" && git ls-files) | ui_saida
  if [ "$MODO" = real ]; then AUTO=1 az_remoto_real; fi
  ia_mcp_json
  ui_ok "AurumCode registrado como servidor MCP \"aurum\" em .mcp.json (Claude Code lê na abertura; fora do git):"
  sed 's/^/    /' "$TUT_WORK/.mcp.json" | head -40
  printf '\n  %sCodex:%s o mesmo servidor em ~/.codex/config.toml:\n' "$B" "$N"
  python3 -I -c 'import json,sys; s=json.load(open(sys.argv[1]))["mcpServers"]["aurum"]; print("    [mcp_servers.aurum]\n    command = \"docker\"\n    args = " + json.dumps(s["args"]))' "$TUT_WORK/.mcp.json"
  local pedido="$STATE/pedido-ia.txt"
  cat > "$pedido" <<PEDIDO
Quero instalar e configurar o AurumCode neste repositório, por conversa.
Siga o roteiro para o agente de $REPO_ROOT/docs/tutorials/instalacao-ia.md
(o mesmo guia está em https://mpaape.github.io/AurumCode/tutorials/instalacao-ia/).
Primeiro inspecione o projeto. Faça uma pergunta por vez, com opções curtas e
uma recomendação explicada. Não peça chaves no chat: oriente como cadastrá-las.
O servidor MCP "aurum" já está registrado neste projeto: depois de cada commit,
verifique com aurum_gate (base main). Para PRs, fixe o workflow reutilizável em
Mpaape/AurumCode/.github/workflows/review.yml@$REF (a tag v2.0.0 passa a ser a
referência fixa quando for publicada). Revisão em português (pt-BR).
Este é um repositório de demonstração: você pode commitar e fazer push na main
(a revisão da PR lê a configuração e as skills da main) e abrir PR quando eu pedir.
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
  ui_diz "3. Peça a regra do time (a segurança já a escreveu; o agente instala e explica):"
  printf '      │ %s\n' "A regra do time está em $HERE/aurum/skills/ia/SKILL.md. Instale-a como .aurumcode/skills/ia/SKILL.md, explique em uma frase cada regra e commite."
  ui_diz "4. Quando o Aurum estiver configurado, commitado e (no GitHub) na main, simule o dev com pressa, em outro terminal:"
  ui_copia "bash $HERE/run.sh ia-defeito --destino $TUT_WORK"
  if [ "$MODO" = real ]; then
    ui_diz "5. Peça a PR com o defeito (a main fica protegida pelo ia-defeito: merge só com o Aurum aprovando):"
    printf '      │ %s\n' "Faça push da branch feature e abra a PR para a main, sem corrigir nada ainda."
    ui_diz "   O workflow leva alguns minutos (gh pr checks --watch): o parecer chega \"Bloqueado: 2 problemas\" e o botão de merge fica travado."
    ui_diz "6. Peça a correção guiada pelo parecer:"
    printf '      │ %s\n' "Leia o parecer do AurumCode na PR, confirme com o Aurum (aurum_gate, base main), explique cada problema em uma frase, corrija, consulte de novo e faça push."
    ui_diz "   O mesmo parecer é editado para \"Aprovado\" e o merge é liberado."
    ui_diz "Os secrets do repositório o agente só orienta; você digita (cada um pede o valor):"
    ui_copia "gh secret set LLM_API_KEY && gh secret set LLM_BASE_URL && gh variable set LLM_MODEL"
  else
    ui_diz "5. Peça ao agente:"
    printf '      │ %s\n' "Mudei o assistente na branch feature. Antes de abrir a PR, pergunte ao Aurum (aurum_gate, base main), explique o que ele bloqueou e corrija; depois consulte de novo."
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
  if tgit remote get-url origin >/dev/null 2>&1; then
    tgit fetch -q origin main 2>/dev/null || true
    if ! tgit cat-file -e origin/main:.aurumcode/config.yml 2>/dev/null; then
      ui_aviso "o GitHub ainda não tem a configuração na main (a PR lê config e skills de lá): peça ao agente o push da main antes."
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

if [ "$ACAO" = ia ]; then ia_preparar
elif [ "$ACAO" = ia-defeito ]; then ia_defeito
elif [ "$ACAO" = ia-correcao ]; then ia_correcao
else passo_1; passo_2; passo_3; passo_4; passo_5; passo_6; passo_7; passo_8; passo_9
fi
