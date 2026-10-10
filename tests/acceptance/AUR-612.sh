#!/usr/bin/env bash
# AUR-612 acceptance: the integration and architecture pages promise only
# what the code does (docs/specs/AUR-612.md cites the code for each finding).
#
# Selectors:
#   all      AC-001, then MUT-001
#   AC-001   every corrected statement is on its page and every wrong one is
#            gone (findings 1-12 of the spec)
#   MUT-001  a copy of the pages with "cada seção de quality_gates do
#            repositório é ignorada" put back must fail AC-001
# AC-002 (strict documentation build) is scripts/docs/build.sh, not this file.
# Unknown selectors exit 64.
set -euo pipefail
export LC_ALL=C

repo_root="$(cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
pages=(
  docs/gate-corporativo.md
  docs/getting-started.md
  docs/provedores.md
  docs/architecture.md
  docs/tutorials/politica-central.md
  docs/README.md
  demo/do-zero/README.md
)
workflow=.github/workflows/review.yml

fail() { printf 'AUR-612/%s\n' "$1" >&2; exit 1; }

# flat ROOT PAGE: the page as one line, without backticks or asterisks and
# with runs of blanks collapsed, so a sentence matches across line breaks.
flat() {
  [[ -s "$1/$2" ]] || fail "AC-001/pagina-ausente:$2"
  tr '\n' ' ' <"$1/$2" | sed -e 's/`//g' -e 's/\*//g' -e 's/[[:space:]][[:space:]]*/ /g'
}

# has ID TEXT PHRASE: PHRASE must be in TEXT.
has() { grep -qF -- "$3" <<<"$2" || fail "AC-001/$1/ausente:$3"; }
# lacks ID TEXT PHRASE: PHRASE must not be in TEXT.
lacks() { if grep -qF -- "$3" <<<"$2"; then fail "AC-001/$1/frase-errada:$3"; fi; }

ac001() {
  local root="$1" gc gs pv ar pc rd dz
  gc="$(flat "$root" docs/gate-corporativo.md)"
  gs="$(flat "$root" docs/getting-started.md)"
  pv="$(flat "$root" docs/provedores.md)"
  ar="$(flat "$root" docs/architecture.md)"
  pc="$(flat "$root" docs/tutorials/politica-central.md)"
  rd="$(flat "$root" docs/README.md)"
  dz="$(flat "$root" demo/do-zero/README.md)"

  # 1. camada 2: padrões de engenharia centrais não são nativos.
  has F01 "$gc" '### Padrões de engenharia centrais (hoje)'
  has F01 "$gc" 'O workflow aceita um policy_repository. Não existe um segundo repositório de política'
  has F01 "$gc" 'nem sincronização automática de skills entre repositórios'
  has F01 "$gc" 'a política não governa review.profiles'
  has F01 "$gc" 'skills/padroes/'
  has F01 "$gc" 'CODEOWNERS'
  has F01 "$gc" 'Copie as skills do time para .aurumcode/skills/ de cada repositório'

  # 2. tabela de precedência: gate sempre da política; seções novas.
  has F02 "$pc" '| rules, ignore, gate, exceptions | sempre a política, declare ou não |'
  has F02 "$pc" '| quality_gates.scanners | engine por engine |'
  has F02 "$pc" 'required: true vence: o repositório não a troca nem a desliga'
  has F02 "$pc" '| review.profiles, review.verification, review.memory'
  has F02 "$pc" '| nunca a política | sempre do repositório |'
  has F02 "$pc" '| analysis_data, deliberation, batches, changelog_check, dependencies | a política, quando declara |'
  lacks F02 "$pc" '| gate | sim |'
  lacks F02 "$pc" 'A política governa só o que declara'
  lacks F02 "$pc" '(a política governa só o que declara)'

  # 3. quality_gates: vale a seção que a política declara; scanners por engine.
  if grep -qE 'cada se(ção|cao) de quality_gates do reposit(ó|o)rio (é|e|são|sao) ignorad' <<<"$gc"; then
    fail "AC-001/F03/frase-errada:cada seção de quality_gates do repositório é ignorada"
  fi
  has F03 "$gc" 'Em quality_gates, vale a seção que a política declara'
  has F03 "$gc" 'Os scanners (sast e scanners) se resolvem engine por engine'

  # 4. versão: sem versão mínima na política; Dependabot só em uses:;
  #    tag no repositório avulso, SHA no workflow obrigatório; ruleset.
  has F04 "$gc" 'A política não fixa uma versão mínima do AurumCode'
  has F04 "$gc" 'O Dependabot atualiza só a referência de uses:, nunca o policy_ref'
  has F04 "$gc" 'Num repositório avulso, a tag de uma release basta'
  has F04 "$gc" 'Require workflows to pass before merging'
  has F04 "$gc" 'depende do plano da organização'
  has F04 "$gs" 'A tag basta para um repositório avulso. Numa organização com workflow obrigatório'
  has F04 "$gs" 'fixe a SHA de 40 hex com a versão num comentário'

  # 5. o status só impede o merge com a proteção da branch.
  has F05 "$gs" 'Com gate: declarado em .aurumcode/config.yml'
  has F05 "$gs" 'faça o status impedir o merge: em Settings → Branches'
  has F05 "$gs" 'o status aurumcode/policy-gate não é publicado: exigi-lo na proteção da branch ou num ruleset deixaria toda PR pendente'
  lacks F05 "$gs" '6. Faça o status impedir o merge'
  has F05 "$gs" 'Require status checks to pass before merging e escolha aurumcode/policy-gate'

  # 6. PR do Dependabot só recebe Dependabot secrets.
  has F06 "$gs" 'A PR aberta pelo Dependabot roda o workflow só com os Dependabot secrets'
  has F06 "$gs" 'Settings → Secrets and variables → Dependabot'
  lacks F06 "$gs" 'com as notas da versão, que o próprio AurumCode revisa.'
  has F06 "$gc" 'Cadastre os mesmos nomes da tabela também como Dependabot secrets da organização'

  # 7. falha do AURUMCODE_POLICY_TOKEN é o erro do checkout.
  has F07 "$gc" 'O AURUMCODE_POLICY_TOKEN vai direto ao actions/checkout'
  has F07 "$gc" 'o job para nesse passo, com o erro do próprio actions/checkout, antes de qualquer revisão'
  has F07 "$gc" 'nenhum status aurumcode/policy-gate é publicado'

  # 8. reserva bedrock/azure no Actions; LLM_API_KEY com valor para ollama.
  has F08 "$pv" 'uma reserva bedrock ou azure-openai exige LLM_FALLBACK_<n>_BASE_URL com a URL completa'
  has F08 "$pv" 'https://bedrock-runtime.<região>.amazonaws.com/openai/v1'
  has F08 "$pv" 'https://<recurso>.openai.azure.com/openai/deployments/<deployment>'
  has F08 "$pv" 'o secret LLM_API_KEY precisa ter algum valor mesmo para o Ollama'
  lacks F08 "$pv" 'não envia credencial (auth: none) e não exige LLM_API_KEY.'
  local id
  for id in claude-sonnet-5-5 gpt-5.4-mini 'fallback_2_model: gpt-5' 'LLM_FALLBACK_2_MODEL=gpt-5' us.anthropic.claude; do
    lacks F08 "$pv" "$id"
  done

  # 9. versões fixadas das ferramentas.
  has F09 "$gc" '### Versões fixadas das ferramentas'
  has F09 "$gc" '| gitleaks | v8.30.1 |'
  has F09 "$gc" '| Semgrep | 1.172.0 |'
  has F09 "$gc" '| osv-scanner | v2.0.0 |'
  has F09 "$gc" '| Trivy | 0.73.0 |'
  has F09 "$gc" '| Cosign | v3.1.3 |'
  has F09 "$gc" 'Nenhuma ferramenta roda como latest'
  [[ -s "$root/$workflow" ]] || fail "AC-001/F09/workflow-ausente"
  grep -qF "cosign-release: 'v3.1.3'" "$root/$workflow" || fail "AC-001/F09/cosign-diverge-do-workflow"

  # 10. o verificador adversarial no fluxo da arquitetura.
  has F10 "$ar" 'o verificador adversarial (review.verification, internal/review/verify) confere cada achado do modelo'
  has F10 "$ar" 'Achados de scanner nunca são enviados a ele'
  has F10 "$ar" 'Roda depois do modelo e antes da decisão do gate, no início deste passo'

  # 11. o analista seguranca é uma passada do modelo; parecer atualizado.
  has F11 "$dz" 'O analista embutido seguranca é uma passada do modelo'
  has F11 "$dz" 'não a varredura determinística de segredos'
  has F11 "$dz" 'o parecer é atualizado: aprovado'
  lacks F11 "$dz" 'o parecer é editado'

  # 12. o índice aponta para os tutoriais por capacidade logo no topo.
  head -n 8 "$root/docs/README.md" | grep -qF '(tutorials/README.md)' || fail "AC-001/F12/topo-sem-tutoriais"
  head -n 8 "$root/docs/README.md" | grep -qF '(index.md)' || fail "AC-001/F12/topo-sem-visao-geral"
  has F12 "$rd" 'índice de tutoriais executáveis'
}

mut001() {
  local copy p
  copy="$(mktemp -d)"
  trap 'rm -rf -- "$copy"' RETURN
  for p in "${pages[@]}" "$workflow"; do
    mkdir -p "$copy/${p%/*}"
    cp "$repo_root/$p" "$copy/$p"
  done
  printf '\nSob política central, cada seção de `quality_gates` do repositório é ignorada.\n' \
    >>"$copy/docs/gate-corporativo.md"
  local err
  if err="$( ( ac001 "$copy" ) 2>&1 )"; then
    fail "MUT-001/frase-errada-passou"
  fi
  # The failure must be the reintroduced sentence (F03), not another check.
  grep -q '^AUR-612/AC-001/F03/' <<<"$err" || fail "MUT-001/falhou-por-outro-motivo:$err"
  printf 'AUR-612/MUT-001/rejected (%s)\n' "$err"
}

case "${1:-all}" in
  all) ac001 "$repo_root"; mut001; printf 'AUR-612/all/pass\n' ;;
  AC-001) ac001 "$repo_root"; printf 'AUR-612/AC-001/pass\n' ;;
  MUT-001) mut001 ;;
  *) printf 'AUR-612/%s/unknown-selector\n' "$1" >&2; exit 64 ;;
esac
