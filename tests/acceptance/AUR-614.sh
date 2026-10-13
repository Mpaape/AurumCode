#!/usr/bin/env bash
# AUR-614 acceptance (offline): the layered POC (demo/do-zero: security-policy,
# devops-policy and the service) and the `camadas` tutorial are complete and
# coherent with their recorded runs, and the real mode is mounted without
# touching the network.
#
# Selectors:
#   all      every check below, then both mutations
#   AC-001   demo/do-zero/run.sh --check passes on the versioned
#            out/roteiro-mock.log (last real run of `--modo mock --auto`): the
#            three repositories are created, the PR is blocked with a finding of
#            each layer identified (security rule and analyst, engineering rules
#            with the layer label and their analysts, deterministic check), the
#            legend names the repository of each rule, and the correction is
#            approved by the same layers; the README says who maintains what
#   AC-002   the recorded run shows the service's looser gate and ignore
#            discarded with a warning each and the same block (exit 3) right
#            after: the service cannot loosen the security policy
#   AC-003   `run.sh --modo real --mostrar` (no network, no gh, no docker) prints
#            the three repository names derived from --repo, the gh commands, the
#            secrets and, with --privado, the two read-only tokens; the service's
#            workflow template points to policy_repository/standards_repository
#            with SHA placeholders and to the reusable workflow by ref
#   AC-004   `run.sh limpar` removes .estado/ (the three repositories and the fake
#            GitHub) and the ia-mode project it created, and nothing else
#   AC-005   the camadas tutorial: run.sh --check passes on its out/, every
#            <!-- saida --> block is a literal excerpt of out/, every
#            <!-- arquivo --> block is byte-identical to its file, the page has
#            >= 3 use cases and a failure case, it is indexed, mapa.tsv has its
#            four cases, every `aurumcode` command and flag it cites exists in
#            --help, only reserved domains, and the spec records the real run
#   MUT-001  the service workflow without standards_repository loses the
#            engineering findings in the PR review: with docker and the recorded
#            image, the mutated POC runs and the engineering findings leave the
#            published review (AC-001 red); without them (sealed run) the same
#            proof on the evidence: removing from out/ the lines that prove the
#            standards layer makes --check diverge
#   MUT-002  layers inverted in the tutorial's case 3 (the engineering version of
#            the skill "nomes" placed in the policy and vice versa): with docker
#            the case runs, the gate no longer blocks (exit 0) and out/ diverges
#            from expected/; without docker, removing the proving line from
#            out/mesmo-nome.log makes --check diverge
# Unknown selectors exit 64; infrastructure failures 79; failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-614'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
poc="$repo_root/demo/do-zero"
tut="$repo_root/demo/tutoriais/camadas"
lib="$repo_root/demo/tutoriais/_lib"
doc="$repo_root/docs/tutorials/camadas.md"
spec="$repo_root/docs/specs/AUR-614.md"
log="$poc/out/roteiro-mock.log"

for f in "$poc/run.sh" "$poc/README.md" "$log" "$poc/expected/roteiro-mock.txt" "$poc/out/.imagem" \
  "$poc/camadas/servico/code-review.yml.modelo" "$poc/camadas/servico/afrouxa.yml" \
  "$tut/run.sh" "$tut/out/.imagem" "$doc" "$spec" "$lib/tutorial.sh" "$lib/pr.sh" "$lib/normaliza.sed" \
  "$lib/dominios-permitidos.txt" "$repo_root/docs/tutorials/README.md" "$repo_root/docs/index.md" \
  "$repo_root/tests/fixtures/tutorials/mapa.tsv"; do
  [[ -f "$f" ]] || infra "missing:${f#"$repo_root"/}"
done
for tool in awk grep sed diff cp mktemp find sort; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-tool:$tool"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a614.XXXXXX")" || infra mktemp
trap 'chmod -R u+rwX "${work:?}" 2>/dev/null || true; rm -rf "${work:?}"' EXIT

casos_of() { sed -n 's/^CASOS=(\(.*\))$/\1/p' "$tut/run.sh"; }

# copia_demo DEST: a copy of the demo directories a run.sh reads (the POC, the
# tutorial, the shared library and the MCP test client); .estado/ left out.
copia_demo() {
  local dest="$1"
  mkdir -p "$dest/demo/tutoriais"
  cp -R "$poc" "$dest/demo/do-zero"
  cp -R "$tut" "$dest/demo/tutoriais/camadas"
  cp -R "$lib" "$dest/demo/tutoriais/_lib"
  [[ ! -d "$repo_root/demo/tutoriais/agente" ]] || cp -R "$repo_root/demo/tutoriais/agente" "$dest/demo/tutoriais/agente"
  rm -rf "$dest/demo/do-zero/.estado" "$dest/demo/tutoriais/camadas/.estado"
  chmod -R u+rwX "$dest"
}

# imagem_gravada: the product image that recorded the tutorial's out/ (by id).
imagem_gravada() { sed -n 's/^image=//p' "$tut/out/.imagem"; }

# docker_ok: the behavioural tier of a mutation needs docker, python3 (fake
# GitHub and MCP client) and the exact image that recorded the evidence.
docker_ok() {
  command -v docker >/dev/null 2>&1 && command -v python3 >/dev/null 2>&1 &&
    docker info >/dev/null 2>&1 && [[ -n "$(imagem_gravada)" ]] &&
    docker image inspect "$(imagem_gravada)" >/dev/null 2>&1 &&
    [[ -f "$repo_root/demo/tutoriais/agente/cliente-mcp.py" ]]
}

# linha_de TRECHO ARQUIVO: number of the first line holding TRECHO.
linha_de() { grep -nF -- "$1" "$2" | head -n 1 | cut -d: -f1; }

# achatado: the roteiro folds its prose at 78 columns; joining the lines and
# squeezing blanks lets a sentence be searched whole (commands are one line).
achatado() { tr '\n' ' ' | tr -s ' '; }
# tem TRECHO TEXTO: TRECHO (blanks squeezed) occurs in TEXTO (already flat).
tem() { grep -qF -- "$(tr -s ' ' <<<"$1")" <<<"$2"; }

ac001() {
  bash "$poc/run.sh" --check >"$work/check.out" 2>&1 || { cat "$work/check.out" >&2; fail "AC-001/check-falhou"; }
  grep -q '^CHECK OK$' "$work/check.out" || fail "AC-001/check-sem-ok"
  local l
  for l in remotos/security-policy.git remotos/devops-policy.git remotos/assistente.git; do
    grep -qF -- "$l" "$log" || fail "AC-001/repositorio-nao-criado:$l"
  done
  # a PR bloqueada: um achado de cada camada, identificado no parecer publicado
  grep -qF -- '[perfil seguranca] -- publicado na linha' "$log" || fail "AC-001/sem-achado-da-seguranca-na-pr"
  grep -qF -- 'ia#ia-001-resposta-do-modelo-nunca-e-executada' "$log" || fail "AC-001/sem-regra-ia-001"
  grep -qF -- '[perfil padroes] [padrões de engenharia] -- no parecer' "$log" || fail "AC-001/sem-achado-padroes-rotulado-na-pr"
  grep -qF -- '[perfil qa] [padrões de engenharia] -- no parecer' "$log" || fail "AC-001/sem-achado-qa-rotulado-na-pr"
  grep -qF -- 'analysis/hardcoded-secret' "$log" || fail "AC-001/sem-achado-deterministico"
  grep -qF -- '**Bloqueado: 2 problemas' "$log" || fail "AC-001/pr-nao-bloqueada"
  # a legenda do roteiro nomeia o repositorio de cada regra
  grep -qF -- '→ security-policy (política de segurança), analista seguranca' "$log" || fail "AC-001/legenda-sem-seguranca"
  grep -qF -- '→ devops-policy (padrões de engenharia), analista padroes' "$log" || fail "AC-001/legenda-sem-padroes"
  grep -qF -- '→ devops-policy (padrões de engenharia), analista qa' "$log" || fail "AC-001/legenda-sem-qa"
  grep -qF -- '→ verificação determinística do AurumCode, cobrada pelo gate do security-policy' "$log" || fail "AC-001/legenda-sem-deterministico"
  # a correcao e aprovada depois do bloqueio, pelas mesmas camadas (as observacoes seguem)
  grep -qF -- '**Aprovado com 2 observações que não bloqueiam o merge.**' "$log" || fail "AC-001/correcao-nao-aprovada"
  grep -qF -- 'status aurumcode/policy-gate: success' "$log" || fail "AC-001/gate-nao-aprovou"
  (( $(linha_de '**Bloqueado: 2 problemas' "$log") < $(linha_de '**Aprovado com 2 observações' "$log") )) || fail "AC-001/aprovado-antes-do-bloqueio"
  (( $(grep -cF -- '[padrões de engenharia] -- no parecer' "$log") >= 4 )) || fail "AC-001/engenharia-fora-do-parecer-aprovado"
  # o README diz quem mantem cada repositorio
  grep -q 'security-policy' "$poc/README.md" && grep -q 'devops-policy' "$poc/README.md" || fail "AC-001/readme-sem-repositorios"
  grep -qi 'time de segurança' "$poc/README.md" && grep -qi 'time de engenharia' "$poc/README.md" || fail "AC-001/readme-sem-quem-mantem"
  printf '%s/AC-001/ok (tres repositorios, PR bloqueada com as tres camadas identificadas, correcao aprovada)\n' "$card"
}

ac002() {
  local aviso prox
  grep -qF -- 'gate do config do repositório foi ignorado: a política central decide sozinha' "$log" || fail "AC-002/gate-do-servico-nao-descartado"
  grep -qF -- 'padrão de ignore "assistente.py" do repositório não foi aplicado: a política central decide sozinha' "$log" || fail "AC-002/ignore-do-servico-nao-descartado"
  aviso="$(linha_de 'gate do config do repositório foi ignorado' "$log")"
  prox="$(tail -n +"$aviso" "$log" | grep -m1 -E '^ *exit_code=' | sed 's/^ *//')"
  [[ "$prox" == 'exit_code=3' ]] || fail "AC-002/bloqueio-diferente-depois-do-aviso:$prox"
  grep -qF -- 'nada mudou: o serviço não consegue afrouxar a segurança.' "$log" || fail "AC-002/sem-conclusao"
  grep -q '^gate:' "$poc/camadas/servico/afrouxa.yml" && grep -q '^ignore:' "$poc/camadas/servico/afrouxa.yml" || fail "AC-002/afrouxa-sem-gate-ou-ignore"
  grep -qi 'afrouxar' "$poc/README.md" || fail "AC-002/readme-sem-por-que"
  printf '%s/AC-002/ok (gate e ignore do servico descartados com aviso; bloqueio igual)\n' "$card"
}

ac003() {
  local out l wf="$poc/camadas/servico/code-review.yml.modelo"
  # gh e docker falsos: --mostrar nao pode chamar nenhum dos dois
  mkdir -p "$work/bin"
  printf '#!/usr/bin/env bash\n: > "%s/chamou-$(basename "$0")"\nexit 99\n' "$work" > "$work/bin/gh"
  cp "$work/bin/gh" "$work/bin/docker"; chmod +x "$work/bin/gh" "$work/bin/docker"
  out="$(cd "$poc" && PATH="$work/bin:$PATH" bash ./run.sh --modo real --mostrar --repo loja --privado 2>&1 | achatado)" || fail "AC-003/mostrar-falhou"
  [[ ! -e "$work/chamou-gh" && ! -e "$work/chamou-docker" ]] || fail "AC-003/mostrar-chamou-gh-ou-docker"
  for l in '<seu-usuario>/loja-security-policy' '<seu-usuario>/loja-devops-policy' \
    'gh repo create <seu-usuario>/loja-security-policy --private --source=security-policy --remote=origin --push' \
    'gh repo create <seu-usuario>/loja-devops-policy --private --source=devops-policy --remote=origin --push' \
    'gh repo create <seu-usuario>/loja --private --source=loja --remote=origin --push' \
    'gh secret set LLM_API_KEY --repo <seu-usuario>/loja --body "$LLM_API_KEY"' \
    'gh secret set LLM_BASE_URL --repo <seu-usuario>/loja --body "$LLM_BASE_URL"' \
    'gh variable set LLM_MODEL --repo <seu-usuario>/loja --body "$LLM_MODEL"' \
    'gh secret set AURUMCODE_POLICY_TOKEN --repo <seu-usuario>/loja --body "$AURUMCODE_POLICY_TOKEN"' \
    'gh secret set AURUMCODE_STANDARDS_TOKEN --repo <seu-usuario>/loja --body "$AURUMCODE_STANDARDS_TOKEN"' \
    'Contents: read' 'policy_repository: <seu-usuario>/loja-security-policy   policy_ref: <SHA da main de loja-security-policy>' \
    'standards_repository: <seu-usuario>/loja-devops-policy   standards_ref: <SHA da main de loja-devops-policy>' \
    'uses: Mpaape/AurumCode/.github/workflows/review.yml@' 'nada é executado'; do
    tem "$l" "$out" || fail "AC-003/mostrar-privado-sem:$l"
  done
  out="$(cd "$poc" && PATH="$work/bin:$PATH" bash ./run.sh --modo real --mostrar --repo acme/loja 2>&1 | achatado)" || fail "AC-003/mostrar-publico-falhou"
  tem 'gh repo create acme/loja-security-policy --public' "$out" || fail "AC-003/dono-de-repo-nao-usado"
  tem 'só são necessários com --privado' "$out" || fail "AC-003/publico-sem-aviso-de-token"
  ! tem 'gh secret set AURUMCODE_POLICY_TOKEN' "$out" || fail "AC-003/publico-cadastra-token"
  [[ ! -e "$work/chamou-gh" && ! -e "$work/chamou-docker" ]] || fail "AC-003/mostrar-chamou-gh-ou-docker"
  for l in 'policy_repository: __POLICY_REPO__' 'policy_ref: __POLICY_REF__' 'standards_repository: __STANDARDS_REPO__' \
    'standards_ref: __STANDARDS_REF__' 'uses: Mpaape/AurumCode/.github/workflows/review.yml@__REF__' \
    'AURUMCODE_POLICY_TOKEN: ${{ secrets.AURUMCODE_POLICY_TOKEN }}' 'AURUMCODE_STANDARDS_TOKEN: ${{ secrets.AURUMCODE_STANDARDS_TOKEN }}'; do
    grep -qF -- "$l" "$wf" || fail "AC-003/modelo-do-workflow-sem:$l"
  done
  # o roteiro fixa as duas camadas na SHA da main de cada repositorio central
  grep -q 'rev-parse main' "$poc/run.sh" && grep -q '__POLICY_REF__|\$sec_sha' "$poc/run.sh" && grep -q '__STANDARDS_REF__|\$dev_sha' "$poc/run.sh" || fail "AC-003/refs-nao-fixadas-em-sha"
  printf '%s/AC-003/ok (modo real montado sem rede: tres repositorios, comandos gh, secrets e tokens)\n' "$card"
}

ac004() {
  local copy="$work/limpar" d out
  copia_demo "$copy"
  for d in security-policy devops-policy projeto remotos/assistente.git remotos/security-policy.git remotos/devops-policy.git github/assistente; do
    mkdir -p "$copy/demo/do-zero/.estado/$d"
  done
  mkdir -p "$work/projeto-ia/.git" "$work/projeto-alheio/.git"
  : > "$work/projeto-ia/.git/aurum-poc"
  out="$(bash "$copy/demo/do-zero/run.sh" limpar --destino "$work/projeto-ia" 2>&1)" || fail "AC-004/limpar-falhou"
  [[ ! -e "$copy/demo/do-zero/.estado" ]] || fail "AC-004/estado-nao-removido"
  [[ ! -e "$work/projeto-ia" ]] || fail "AC-004/projeto-ia-nao-removido"
  grep -qF 'security-policy, devops-policy e o serviço' <<<"$out" || fail "AC-004/limpar-nao-diz-o-que-apagou"
  for d in out/roteiro-mock.log expected/roteiro-mock.txt camadas/security-policy/.aurumcode/config.yml camadas/devops-policy/.aurumcode/profiles.yml README.md; do
    [[ -f "$copy/demo/do-zero/$d" ]] || fail "AC-004/limpar-apagou-demais:$d"
  done
  # um destino sem a marca do modo ia nunca e apagado
  bash "$copy/demo/do-zero/run.sh" limpar --destino "$work/projeto-alheio" >/dev/null 2>&1 || fail "AC-004/limpar-sem-estado-falhou"
  [[ -d "$work/projeto-alheio/.git" ]] || fail "AC-004/limpar-apagou-projeto-alheio"
  printf '%s/AC-004/ok (limpar remove .estado/ e o projeto do modo ia; nada alem)\n' "$card"
}

# O binario do produto: .bin/aurumcode, AURUMCODE_BIN, aurumcode no PATH, ou go build.
aurum_bin() {
  local b
  for b in "${AURUMCODE_BIN:-}" "$repo_root/.bin/aurumcode"; do
    [[ -n "$b" && -x "$b" ]] && { printf '%s' "$b"; return; }
  done
  if command -v aurumcode >/dev/null 2>&1; then command -v aurumcode; return; fi
  command -v go >/dev/null 2>&1 || infra "sem-binario-e-sem-go"
  (cd "$repo_root" && go build -o "$work/aurumcode" ./cmd/aurumcode) >"$work/build.log" 2>&1 || { cat "$work/build.log" >&2; infra "go-build"; }
  printf '%s' "$work/aurumcode"
}

ac005() {
  local c n k=0 line path nb=0 bin sub tok name nf=0
  grep -q "^## Objetivo" "$doc" || fail "AC-005/sem-objetivo"
  grep -q "^## Pré-requisitos" "$doc" || fail "AC-005/sem-prerequisitos"
  grep -q "^## Problemas comuns" "$doc" || fail "AC-005/sem-problemas-comuns"
  grep -q "^## Quando falha" "$doc" || fail "AC-005/sem-caso-de-falha"
  n="$(grep -c '^## Caso [0-9]' "$doc" || true)"
  (( n >= 3 )) || fail "AC-005/casos-insuficientes:$n"
  (( $(grep -c '^O que observar' "$doc" || true) >= n )) || fail "AC-005/sem-o-que-observar"
  for c in $(casos_of); do
    k=$((k + 1))
    [[ -s "$tut/expected/$c.txt" ]] || fail "AC-005/expected-ausente:$c"
    [[ -s "$tut/out/$c.log" ]] || fail "AC-005/out-ausente:$c"
    grep -q "^caso_${c//-/_}()" "$tut/run.sh" || fail "AC-005/fase-ausente:$c"
    grep -q "^<!-- saida: $c -->" "$doc" || fail "AC-005/saida-ausente-no-texto:$c"
    awk -F'\t' -v c="$c" '$2 == "camadas" && $3 == c && $NF == "entregue" { ok = 1 } END { exit !ok }' "$repo_root/tests/fixtures/tutorials/mapa.tsv" || fail "AC-005/mapa-sem-caso:$c"
    grep -qF "camadas/$c" "$spec" || fail "AC-005/spec-sem-registro:camadas/$c"
  done
  (( k >= 4 )) || fail "AC-005/fases-insuficientes:$k"
  grep -q 'camadas.md' "$repo_root/docs/tutorials/README.md" || fail "AC-005/fora-do-indice"
  grep -q 'tutorials/camadas.md' "$repo_root/docs/index.md" || fail "AC-005/fora-da-pagina-inicial"
  grep -qiE 'execu(c|ç)(a|ã)o real' "$spec" || fail "AC-005/spec-sem-execucao-real"
  bash "$tut/run.sh" --check >"$work/tcheck.out" 2>&1 || { cat "$work/tcheck.out" >&2; fail "AC-005/check-falhou"; }
  grep -q '^CHECK OK$' "$work/tcheck.out" || fail "AC-005/check-sem-ok"
  if grep -l '^ERRO:' "$tut"/out/*.log | awk 'END{exit NR==0}'; then fail "AC-005/out-com-erro"; fi
  # cada bloco "saida" do texto e um trecho literal do out/ do caso
  awk '
    /^<!-- saida: / { c = $3; armed = 1; next }
    armed && !open && /^```/ { open = 1; next }
    open && /^```/ { open = 0; armed = 0; next }
    open { print c "\t" $0 }
  ' "$doc" > "$work/saidas.tsv"
  [[ -s "$work/saidas.tsv" ]] || fail "AC-005/sem-saidas-no-texto"
  while IFS=$'\t' read -r c line; do
    [[ -n "$line" ]] || continue
    grep -qF -- "$line" "$tut/out/$c.log" || fail "AC-005/saida-do-texto-nao-esta-no-out:$c:$line"
    nb=$((nb + 1))
  done < "$work/saidas.tsv"
  # cada bloco "arquivo" e identico ao arquivo
  n=0
  while IFS= read -r path; do
    n=$((n + 1))
    [[ -f "$repo_root/$path" ]] || fail "AC-005/arquivo-inexistente:$path"
    awk -v want="<!-- arquivo: $path -->" '
      $0 == want { armed = 1; next }
      armed && !open && /^```/ { open = 1; next }
      open && /^```/ { exit }
      open { print }
    ' "$doc" > "$work/bloco"
    [[ -s "$work/bloco" ]] || fail "AC-005/bloco-ausente:$path"
    diff -u "$repo_root/$path" "$work/bloco" >/dev/null || fail "AC-005/bloco-diverge:$path"
  done < <(sed -n 's/^<!-- arquivo: \(.*\) -->$/\1/p' "$doc")
  (( n >= 3 )) || fail "AC-005/blocos-insuficientes:$n"
  # comandos e flags citados existem no produto
  bin="$(aurum_bin)"
  "$bin" --help >"$work/help.txt" 2>&1 || true
  [[ -s "$work/help.txt" ]] || infra "help-vazio"
  : > "$work/cmds.txt"
  awk '/^```/ { if (open) { open = 0 } else { open = ($0 == "```bash") } next } open && /^(\$ )?aurumcode / { sub(/^\$ /, ""); print }' "$doc" >> "$work/cmds.txt"
  sed -n 's/^[[:space:]]*aurum \(review\|fix\) /aurumcode \1 /p' "$tut/run.sh" >> "$work/cmds.txt"
  [[ -s "$work/cmds.txt" ]] || fail "AC-005/nenhum-comando-extraido"
  while IFS= read -r line; do
    line="${line%%#*}"; line="${line%%>*}"
    # shellcheck disable=SC2206
    local toks=($line)
    sub="${toks[1]:-}"
    grep -qE "^  $sub[[:space:]]" "$work/help.txt" || fail "AC-005/comando-inexistente:$sub ($line)"
    "$bin" "$sub" --help >"$work/help-$sub.txt" 2>&1 || true
    for tok in "${toks[@]:2}"; do
      case "$tok" in
        --*|-[a-z]*)
          name="${tok#--}"; name="${name#-}"; name="${name%%=*}"
          grep -qE "^  -{1,2}$name( |\$)" "$work/help-$sub.txt" || fail "AC-005/flag-inexistente:$sub --$name ($line)"
          nf=$((nf + 1)) ;;
      esac
    done
  done < <(sort -u "$work/cmds.txt")
  for name in politica padroes base; do
    grep -qE "^  -{1,2}$name( |\$)" "$work/help-review.txt" || fail "AC-005/review-sem-flag:$name"
  done
  # so dominios reservados; nenhum segredo (POC, tutorial, pagina e spec). O
  # aplicativo de exemplo da POC (boilerplate/ e etapas/) fica fora da regra de
  # dominio: ele fala com um servico compativel com OpenAI e o endereco padrao
  # publico faz parte do exemplo; a regra de segredo vale para ele tambem.
  local files=() f host bad='' allowed alt='' dline
  while IFS= read -r f; do files+=("$f"); done < <(find "$poc" "$tut" "$doc" "$spec" -type f ! -path '*/.estado/*')
  local dominio=()
  while IFS= read -r f; do dominio+=("$f"); done < <(find "$poc" "$tut" "$doc" "$spec" -type f ! -path '*/.estado/*' ! -path "$poc/boilerplate/*" ! -path "$poc/etapas/*")
  while IFS= read -r dline || [[ -n "$dline" ]]; do
    case "$dline" in ''|'#'*) continue ;; esac
    dline="${dline//./\\.}"
    if [[ "$dline" == \\.* ]]; then alt="$alt|([A-Za-z0-9-]+\\.)*${dline#\\.}"; else alt="$alt|$dline"; fi
  done < "$lib/dominios-permitidos.txt"
  allowed="^(${alt#|})\$"
  while IFS= read -r host; do
    host="${host#*://}"; host="${host%%[:/]*}"
    [[ "$host" =~ $allowed ]] || bad="$bad $host"
  done < <(grep -rhoE 'https?://[^/[:space:]"'"'"')`>]+' "${dominio[@]}" || true)
  [[ -z "$bad" ]] || fail "AC-005/url-real:$bad"
  if grep -rIlE 'BEGIN [A-Z ]*PRIVATE KEY|ghp_[A-Za-z0-9]{20}|github_pat_|sk-[A-Za-z0-9]{20}|xox[bp]-|AKIA[0-9A-Z]{16}' "${files[@]}" | awk 'END{exit NR==0}'; then
    fail "AC-005/segredo-versionado"
  fi
  printf '%s/AC-005/ok (%d casos; %d linhas de saida do texto no out/; %d blocos identicos; %d flags conferidas; execucao real na spec)\n' "$card" "$k" "$nb" "$n" "$nf"
}

mut001() {
  local copy="$work/m1" mlog
  copia_demo "$copy"
  grep -qF -- '[padrões de engenharia] -- no parecer' "$log" || fail "MUT-001/controle-sem-achado-de-padroes"
  if docker_ok; then
    sed -i '/standards_repository:/d; /standards_ref:/d' "$copy/demo/do-zero/camadas/servico/code-review.yml.modelo"
    ! grep -q 'standards_repository' "$copy/demo/do-zero/camadas/servico/code-review.yml.modelo" || infra "MUT-001/mutacao-nao-aplicada"
    AURUMCODE_TUT_IMAGE="$(imagem_gravada)" bash "$copy/demo/do-zero/run.sh" --modo mock --auto >"$work/m1.run" 2>&1 || true
    mlog="$copy/demo/do-zero/out/roteiro-mock.log"
    [[ -s "$mlog" ]] || { cat "$work/m1.run" >&2; infra "MUT-001/roteiro-mutado-sem-gravacao"; }
    if grep -qF -- '[padrões de engenharia] -- no parecer' "$mlog"; then fail "MUT-001/mutacao-sobreviveu (achados de padroes no parecer sem standards_repository)"; fi
    if bash "$copy/demo/do-zero/run.sh" --check >"$work/m1.check" 2>&1; then fail "MUT-001/check-passou-com-a-mutacao"; fi
    # a primeira divergencia e a camada que sumiu: a flag --padroes derivada do workflow ou o rotulo dos achados
    grep -qE -- 'padrões de engenharia|--padroes /github/standards' "$work/m1.check" || { cat "$work/m1.check" >&2; fail "MUT-001/divergencia-nao-nomeia-a-camada"; }
    printf '%s/MUT-001/rejected (sem standards_repository os achados de padroes somem do parecer; executado com docker)\n' "$card"
  else
    grep -vF 'padrões de engenharia' "$log" > "$copy/demo/do-zero/out/roteiro-mock.log"
    if bash "$copy/demo/do-zero/run.sh" --check >"$work/m1.check" 2>&1; then fail "MUT-001/mutacao-sobreviveu"; fi
    grep -qF 'padrões de engenharia' "$work/m1.check" || { cat "$work/m1.check" >&2; fail "MUT-001/divergencia-nao-nomeia-a-camada"; }
    printf '%s/MUT-001/rejected (sem docker: a evidencia sem as linhas da camada de padroes diverge no --check)\n' "$card"
  fi
}

mut002() {
  local copy="$work/m2" a b mlog
  copia_demo "$copy"
  grep -qF 'RESULTADO: a skill da politica venceu' "$tut/out/mesmo-nome.log" || fail "MUT-002/controle-sem-prova"
  if docker_ok; then
    a="$copy/demo/tutoriais/camadas/security-policy-mesmo-nome/.aurumcode/skills/nomes/SKILL.md"
    b="$copy/demo/tutoriais/camadas/devops-policy-mesmo-nome/.aurumcode/skills/nomes/SKILL.md"
    cp "$a" "$work/nomes.tmp"; cp "$b" "$a"; cp "$work/nomes.tmp" "$b"
    if AURUMCODE_TUT_IMAGE="$(imagem_gravada)" bash "$copy/demo/tutoriais/camadas/run.sh" mesmo-nome >"$work/m2.run" 2>&1; then
      fail "MUT-002/caso-passou-com-as-camadas-invertidas"
    fi
    mlog="$copy/demo/tutoriais/camadas/out/mesmo-nome.log"
    grep -q 'esperado exit_code=3, obtido 0' "$mlog" || { cat "$mlog" >&2; fail "MUT-002/falha-inesperada"; }
    if bash "$copy/demo/tutoriais/camadas/run.sh" --check >"$work/m2.check" 2>&1; then fail "MUT-002/check-passou-com-a-mutacao"; fi
    grep -q 'DIVERGENCIA caso=mesmo-nome' "$work/m2.check" || { cat "$work/m2.check" >&2; fail "MUT-002/divergencia-nao-nomeada"; }
    printf '%s/MUT-002/rejected (com a versao da engenharia na politica o gate passa; executado com docker)\n' "$card"
  else
    grep -vF 'RESULTADO: a skill da politica venceu' "$tut/out/mesmo-nome.log" > "$copy/demo/tutoriais/camadas/out/mesmo-nome.log"
    if bash "$copy/demo/tutoriais/camadas/run.sh" --check >"$work/m2.check" 2>&1; then fail "MUT-002/mutacao-sobreviveu"; fi
    grep -q 'DIVERGENCIA caso=mesmo-nome' "$work/m2.check" || { cat "$work/m2.check" >&2; fail "MUT-002/divergencia-nao-nomeada"; }
    printf '%s/MUT-002/rejected (sem docker: a evidencia sem a prova do caso 3 diverge no --check)\n' "$card"
  fi
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  AC-005) ac005 ;;
  MUT-001) mut001 ;;
  MUT-002) mut002 ;;
  all) ac001; ac002; ac003; ac004; ac005; mut001; mut002 ;;
esac
