#!/usr/bin/env bash
# Tutorial executavel: Aurum no seu agente de codigo. Veja ../README.md e docs/tutorials/agente.md.
#
#   run.sh gate-consultado|skill-do-repo|hook-pre-commit|gate-inconclusivo
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(gate-consultado skill-do-repo hook-pre-commit gate-inconclusivo)

INICIO='{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"cliente-de-teste","version":"1"}}}'
INICIADO='{"jsonrpc":"2.0","method":"notifications/initialized"}'

# lista ID: a requisicao tools/list.
lista() { printf '{"jsonrpc":"2.0","id":%s,"method":"tools/list"}' "$1"; }
# chama ID FERRAMENTA ARGUMENTOS: a requisicao tools/call.
chama() { printf '{"jsonrpc":"2.0","id":%s,"method":"tools/call","params":{"name":"%s","arguments":%s}}' "$1" "$2" "$3"; }

# mcp REQUISICAO...: o cliente de teste. Escreve initialize, a notificacao
# initialized e as requisicoes no stdin de `aurumcode mcp` (stdio, na imagem do
# produto, sem rede), grava as respostas e as resume (cliente-mcp.py).
mcp() {
  local reqs="$TUT_WORK/.mcp-requisicoes.jsonl" resp="$TUT_WORK/.mcp-respostas.jsonl" r
  { printf '%s\n' "$INICIO" "$INICIADO"; for r in "$@"; do printf '%s\n' "$r"; done; } > "$reqs"
  printf '$ aurumcode mcp   # cliente de teste: initialize + %d requisicao(oes) por stdio\n' "$#"
  set +e
  aurum_raw "${TUT_ENVS[@]}" --interactive=true --init=false -- mcp < "$reqs" > "$resp" 2> "$TUT_WORK/.mcp-stderr.txt"
  LAST_RC=$?
  set -e
  echo "exit_code=$LAST_RC"
  python3 -I "$HERE/cliente-mcp.py" resume "$resp" "$reqs"
}

# id_de REGRA: o id do achado que cita REGRA na ultima resposta.
id_de() {
  python3 -I -c 'import json,sys
for l in open(sys.argv[1]):
    m = json.loads(l)
    sc = m.get("result", {}).get("structuredContent", {})
    for f in sc.get("blocking_findings", []) + sc.get("findings", []):
        if f["rule_id"] == sys.argv[2]:
            print(f["id"]); sys.exit(0)
sys.exit(1)' "$TUT_WORK/.mcp-respostas.jsonl" "$1"
}

# confere TRECHO MENSAGEM: o resumo do cliente tem o trecho (RESULTADO) ou o caso falha.
confere() {
  if python3 -I "$HERE/cliente-mcp.py" resume "$TUT_WORK/.mcp-respostas.jsonl" "$TUT_WORK/.mcp-requisicoes.jsonl" | grep -qF -- "$1"; then
    echo "RESULTADO: $2"
  else
    echo "ERRO: ausente: $1 ($2)"; return 1
  fi
}

# 1. O agente consulta o gate antes do commit, le o achado, corrige e consulta de novo.
caso_gate_consultado() {
  tut_repo gate-consultado repo-exemplo/base repo-exemplo/mudanca
  TUT_FIXTURE=fixture-skill.json
  echo "--- o agente pergunta ao gate (aurum_gate, base main) e pede o relatorio (aurum_review)"
  mcp "$(lista 1)" "$(chama 2 aurum_gate '{"base":"main"}')" "$(chama 3 aurum_review '{"base":"main"}')"
  confere 'aurum_gate: decisao=fail exit_code=3' "o gate reprova a mudanca, como o CI reprovaria"
  echo "--- a mesma pergunta pela CLI: review --base com as mesmas opcoes do servidor"
  set +e
  aurum_raw "${TUT_ENVS[@]}" -- review --base main --seguranca --exigir-qualidade > "$TUT_WORK/.cli-stdout.txt" 2>/dev/null
  local cli_rc=$?
  set -e
  echo '$ aurumcode review --base main --seguranca --exigir-qualidade'
  echo "exit_code=$cli_rc"
  python3 -I "$HERE/cliente-mcp.py" compara "$TUT_WORK/.mcp-respostas.jsonl" 3 "$TUT_WORK/.cli-stdout.txt" "$cli_rc" | tee "$TUT_WORK/.compara.txt"
  if ! grep -qF 'stdout de review --base: igual' "$TUT_WORK/.compara.txt" || ! grep -qF 'exit_code de review --base: igual' "$TUT_WORK/.compara.txt"; then
    echo "ERRO: o servidor MCP e a CLI divergiram"; return 1
  fi
  echo "RESULTADO: o servidor MCP e a CLI deram o mesmo relatorio e a mesma decisao"
  echo "--- o agente pede a explicacao do achado bloqueante"
  local id
  id="$(id_de 'erros#err-001-erro-nunca-ignorado')"
  mcp "$(chama 4 aurum_gate '{"base":"main"}')" "$(chama 5 aurum_explain "{\"finding_id\":\"$id\"}")"
  confere 'aurum_explain:' "a explicacao traz a regra, a sugestao e como corrigir"
  echo "--- o agente corrige (novo commit) e consulta de novo"
  cp -R "$HERE/repo-exemplo/correcao/." "$TUT_WORK/"
  tgit add -A; tgit commit -q -m "corrige: devolve o erro de os.RemoveAll"
  TUT_FIXTURE=fixture-vazia.json
  mcp "$(chama 6 aurum_gate '{"base":"main"}')"
  confere 'aurum_gate: decisao=pass exit_code=0' "depois da correcao o gate passa: o agente pode commitar e abrir o PR"
}

# 2. A skill do repositorio decide; um achado do modelo que so cita o catalogo orienta.
caso_skill_do_repo() {
  tut_repo skill-do-repo repo-exemplo/base repo-exemplo/mudanca
  echo "--- antes de escrever, o agente pergunta quais regras valem para app.go"
  mcp "$(chama 1 aurum_rules '{"paths":["app.go"]}')"
  confere 'regra: erros#err-001-erro-nunca-ignorado [error]' "a secao da skill do repositorio e uma regra citavel"
  echo "--- o modelo cita so a regra do catalogo (quality/missing-error-handling): o achado aparece, o gate nao reprova"
  TUT_FIXTURE=fixture-llm.json
  mcp "$(chama 2 aurum_review '{"base":"main"}')"
  confere 'aurum_review: decisao=pass exit_code=0' "achado do modelo sem regra de skill orienta e nao reprova"
  echo "--- o modelo cita a skill do repositorio (disse warning; a skill diz error): o gate reprova"
  TUT_FIXTURE=fixture-skill.json
  mcp "$(chama 3 aurum_gate '{"base":"main"}')"
  confere 'erros#err-001-erro-nunca-ignorado (origem skills)' "a violacao da skill do repo reprova o gate"
  echo "--- sob politica central (AURUMCODE_POLICY): as skills da politica e as do repositorio"
  TUT_POLICY=politica
  TUT_ENVS=(-e AURUMCODE_POLICY=/policy)
  mcp "$(chama 4 aurum_rules '{"paths":["app.go"]}')" "$(chama 5 aurum_gate '{"base":"main","politica":"/tmp/outra"}')"
  confere 'skill (policy): policy/.aurumcode/skills/org/SKILL.md' "a skill da politica aparece com a camada policy"
  confere 'skill (repository): .aurumcode/skills/erros/SKILL.md' "a skill do repositorio aparece com a camada repository"
  confere 'aurum_gate: recusado antes de executar: erro -32602' "nenhum argumento aponta outra politica ou desliga regra"
}

# 3. Sem agente: o hook de pre-commit opcional faz a mesma pergunta antes do commit existir.
caso_hook_pre_commit() {
  tut_repo hook-pre-commit repo-exemplo/base
  tgit checkout -q -b feature
  local wrapper="$STATE/aurumcode-docker.sh"
  cat > "$wrapper" <<WRAP
#!/usr/bin/env bash
# o comando aurumcode do hook neste tutorial: a imagem do produto, sem rede,
# com o repositorio e a worktree temporaria montados nos mesmos caminhos.
exec docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \\
  -e AURUMCODE_LLM_FIXTURE="/fixtures/\${AURUM_TUT_FIXTURE}" -v "$HERE:/fixtures:ro" \\
  -v "$TUT_WORK:$TUT_WORK" -v "\$PWD:\$PWD" -w "\$PWD" --entrypoint /app/aurumcode "$TUT_IMAGE" "\$@"
WRAP
  chmod +x "$wrapper"
  cp "$REPO_ROOT/.agents/skills/aurum-review/hooks/pre-commit" "$TUT_WORK/.git/hooks/pre-commit"
  chmod +x "$TUT_WORK/.git/hooks/pre-commit"
  echo "hook instalado: .git/hooks/pre-commit (copia de .agents/skills/aurum-review/hooks/pre-commit)"
  cp -R "$HERE/repo-exemplo/mudanca/." "$TUT_WORK/"
  tgit add -A
  echo '$ git commit -m "limpa o diretorio"   # mudanca que descarta o erro'
  set +e
  AURUMCODE="$wrapper" AURUM_TUT_FIXTURE=fixture-skill.json TMPDIR="$STATE" tgit commit -q -m "limpa o diretorio" > "$TUT_WORK/.hook.txt" 2>&1
  LAST_RC=$?
  set -e
  grep -E '^app\.go:|aurum pre-commit' "$TUT_WORK/.hook.txt" || true
  echo "exit_code=$LAST_RC"
  [ "$LAST_RC" -ne 0 ] && [ "$(tgit rev-list --count main..feature)" = 0 ] || { cat "$TUT_WORK/.hook.txt"; echo "ERRO: o commit passou"; return 1; }
  echo "RESULTADO: o hook barrou o commit; nenhum commit novo existe"
  cp -R "$HERE/repo-exemplo/correcao/." "$TUT_WORK/"
  tgit add -A
  echo '$ git commit -m "limpa o diretorio e devolve o erro"'
  set +e
  AURUMCODE="$wrapper" AURUM_TUT_FIXTURE=fixture-vazia.json TMPDIR="$STATE" tgit commit -q -m "limpa o diretorio e devolve o erro" > "$TUT_WORK/.hook.txt" 2>&1
  LAST_RC=$?
  set -e
  echo "exit_code=$LAST_RC"
  [ "$LAST_RC" -eq 0 ] && [ "$(tgit rev-list --count main..feature)" = 1 ] || { cat "$TUT_WORK/.hook.txt"; echo "ERRO: o commit corrigido nao passou"; return 1; }
  echo "RESULTADO: com a correcao o gate passa e o commit existe"
  if [ -n "$(tgit worktree list --porcelain | sed -n '/aurum-pre-commit/p')" ]; then echo "ERRO: a worktree temporaria ficou"; return 1; fi
  echo "RESULTADO: a worktree temporaria do hook foi removida"
}

# Falha: sem provedor, ou com o provedor falhando, o gate e inconclusivo, nunca "passa".
caso_gate_inconclusivo() {
  tut_repo gate-inconclusivo repo-exemplo/base repo-exemplo/correcao
  echo "--- sem provedor de modelo configurado"
  TUT_FIXTURE=none
  mcp "$(chama 1 aurum_gate '{"base":"main"}')" "$(chama 2 aurum_gate '{"base":"--output=/tmp/x"}')"
  confere 'aurum_gate: decisao=inconclusive exit_code=1 motivo=provider_failure' "sem provedor a revisao nao aconteceu: inconclusivo, nunca pass"
  confere 'aurum_gate: recusado antes de executar: erro -32602' "argumento invalido e recusado antes de qualquer revisao"
  echo "--- com o provedor falhando (fixture inexistente)"
  TUT_FIXTURE=fixture-inexistente.json
  mcp "$(chama 3 aurum_gate '{"base":"main"}')"
  confere 'aurum_gate: decisao=inconclusive exit_code=1' "provedor que falha vira inconclusivo"
  echo "--- o agente pergunta antes de commitar: base HEAD, nada mudou"
  TUT_FIXTURE=fixture-llm.json
  mcp "$(chama 4 aurum_gate '{"base":"HEAD"}')"
  confere 'aurum_gate: decisao=inconclusive exit_code=0 motivo=empty_change' "mudanca vazia nao foi revisada: inconclusivo, nunca pass"
}

tut_main "$@"
