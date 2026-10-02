#!/usr/bin/env bash
# Tutorial executavel: skills de convencao (AUR-561). Veja ../README.md e docs/tutorials/skills.md.
#
#   run.sh skill-do-repo|seletor-por-caminho|selecao-por-linguagem|repo-vs-politica|regra-citavel|falha-skill-inexistente
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(skill-do-repo seletor-por-caminho selecao-por-linguagem repo-vs-politica regra-citavel falha-skill-inexistente)

# Mostra as linhas do prompt que o modelo recebeu (AURUMCODE_PROMPT_CAPTURE)
# que casam com cada trecho literal pedido.
mostra_prompt() {
  local t
  for t in "$@"; do
    if grep -qF -- "$t" "$TUT_WORK/prompt.txt"; then echo "prompt: $t"; else echo "prompt: AUSENTE: $t"; fi
  done
}

# 1. A skill de convencao do repositorio chega ao modelo, e cada secao vira uma regra citavel.
caso_skill_do_repo() {
  tut_repo skill-do-repo repo-exemplo/base-skill repo-exemplo/segredo
  TUT_FIXTURE=fixture-repo.json
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main
  expect_rc 0 "o achado cita a secao da skill como regra; sem gate o exit e 0"
  mostra_prompt '- `convencoes#sem-segredos-no-codigo`' '- `convencoes#erros-com-contexto`' \
    '- review skill (.aurumcode/skills/convencoes.md)' '## Sem segredos no codigo' 'Credenciais vem do ambiente, nunca de literais.'
}

# 2. Seletor por caminho: .aurumcode/instructions/*.md com applyTo. So a do arquivo alterado chega.
caso_seletor_por_caminho() {
  tut_repo seletor-por-caminho repo-exemplo/base-instrucoes repo-exemplo/segredo
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main
  expect_rc 0 "revisao feita"
  mostra_prompt 'go.md (applyTo: **/*.go)' 'Em Go, trate todo erro retornado e envolva-o com %w.'
  if grep -qF 'ts.md (applyTo' "$TUT_WORK/prompt.txt"; then echo "ERRO: ts.md chegou ao modelo"; return 1; fi
  echo "prompt: ts.md (applyTo: **/*.ts) NAO chegou: a mudanca nao toca .ts"
}

# 3. Selecao por linguagem e apelido: o que a CLI faz hoje.
caso_selecao_por_linguagem() {
  tut_repo selecao-por-linguagem repo-exemplo/base-instrucoes repo-exemplo/segredo
  TUT_ENVS=(-e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  echo "skill de diretorio: .aurumcode/skills/estilo-ts/SKILL.md com languages: [ts]"
  aurum review --base main
  expect_rc 0 "revisao feita"
  if grep -qF 'MARCADOR-SKILL-DIRETORIO' "$TUT_WORK/prompt.txt"; then echo "ERRO: a CLI leu a skill de diretorio"; return 1; fi
  echo "prompt: a skill de diretorio SKILL.md NAO foi lida pelo review (selecao por linguagem e apelidos ainda sao biblioteca, nao CLI)"
  if grep -qF 'Skill selection warnings' "$TUT_WORK/prompt.txt"; then echo "ERRO: ha bloco de avisos de selecao"; return 1; fi
  echo "prompt: nenhum bloco 'Skill selection warnings': o aviso de apelido desconhecido so existe na biblioteca"
}

# 4. Skill do repositorio vs skill da politica: sob politica central, so a da politica reprova.
caso_repo_vs_politica() {
  tut_repo repo-vs-politica repo-exemplo/base-politica repo-exemplo/segredo
  TUT_POLICY=politica
  echo "--- o achado cita a skill do REPOSITORIO (convencoes#...)"
  TUT_FIXTURE=fixture-repo.json
  aurum review --base main --politica /policy
  expect_rc 0 "sob politica, a secao da skill do repo nao entra no gate"
  echo "--- o achado cita a skill da POLITICA (seguranca#...)"
  TUT_FIXTURE=fixture-politica.json
  aurum review --base main --politica /policy
  expect_rc 3 "a secao da skill da politica reprova"
}

# 5. Uma regra de skill vira regra citavel do gate: o piso de severidade e a secao nova.
caso_regra_citavel() {
  tut_repo regra-citavel repo-exemplo/base-skill-gate repo-exemplo/segredo
  TUT_FIXTURE=fixture-repo.json
  echo "--- o modelo diz warning; a secao declara severity: error"
  aurum review --base main
  expect_rc 3 "o gate compara o MAIOR entre a severidade do modelo e a da secao"
  echo "--- uma secao nova na skill vale na execucao seguinte"
  cat >> "$TUT_WORK/.aurumcode/skills/convencoes.md" <<'EOF'

## Logs sem dados pessoais
severity: error
Nunca registre senha, token ou documento em log.
EOF
  sed 's/convencoes#sem-segredos-no-codigo/convencoes#logs-sem-dados-pessoais/' "$HERE/fixture-repo.json" > "$TUT_WORK/fixture-nova.json"
  TUT_ENVS=(-e AURUMCODE_LLM_FIXTURE=/work/fixture-nova.json -e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  TUT_FIXTURE=none
  aurum review --base main
  expect_rc 3 "a secao nova ja e citavel e reprova"
  mostra_prompt '- `convencoes#logs-sem-dados-pessoais`'
}

# Falha: skill listada que nao existe.
caso_falha_skill_inexistente() {
  tut_repo falha-skill-inexistente repo-exemplo/base-faltante repo-exemplo/segredo
  TUT_FIXTURE=fixture-repo.json
  echo "--- no repositorio: aviso, a revisao segue sem esse contexto"
  aurum review --base main
  expect_rc 0 "localmente a skill ausente e avisada e a revisao continua"
  echo "--- na politica central: erro de carga, antes de qualquer chamada ao modelo"
  tut_repo falha-skill-inexistente-politica repo-exemplo/base-skill-gate repo-exemplo/segredo
  TUT_POLICY=politica-faltante
  aurum review --base main --politica /policy
  expect_rc 1 "politica com skill inexistente falha o comando"
}

tut_main "$@"
