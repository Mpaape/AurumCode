#!/usr/bin/env bash
# AUR-564 acceptance (offline): the three executable tutorials (qualquer-linguagem,
# benchmark, operacao) are complete, coherent with their demonstrations, and
# every command and flag they cite exists in the product.
#
# Selectors:
#   all              every check below, then the mutation
#   AC-001           the three tutorials exist, are indexed, are in the nav
#                    (mkdocs.yml, or the nav entries the spec registers), and
#                    each has >= 3 numbered use cases plus a failure case
#   AC-002           run.sh --check passes on the versioned out/ of each
#                    tutorial, every expected output block of the text is a
#                    literal excerpt of out/, and the spec records the real run
#   AC-003           every `aurumcode <sub> --flag` in the code blocks (and in
#                    each run.sh) exists in the product's --help
#   AC-004           every configuration block marked "<!-- arquivo: PATH -->"
#                    is byte-identical to PATH; only reserved domains; no secret
#   AC-002-MUT-001   removing from out/ the line that proves a case makes
#                    --check diverge (every case of every tutorial)
# Unknown selectors exit 64; infrastructure failures 79; failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-564'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
tutorials=(qualquer-linguagem benchmark operacao)
spec="$repo_root/docs/specs/AUR-564.md"

for t in "${tutorials[@]}"; do
  for f in "$repo_root/docs/tutorials/$t.md" "$repo_root/demo/tutoriais/$t/run.sh"; do
    [[ -f "$f" ]] || infra "missing:${f#"$repo_root"/}"
  done
done
for f in "$repo_root/docs/tutorials/README.md" "$repo_root/demo/tutoriais/README.md" "$repo_root/demo/tutoriais/_lib/tutorial.sh" "$spec"; do
  [[ -f "$f" ]] || infra "missing:${f#"$repo_root"/}"
done
for tool in awk grep sed diff cp mktemp find sort; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-tool:$tool"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a564.XXXXXX")" || infra mktemp
trap 'chmod -R u+rwX "${work:?}" 2>/dev/null || true; rm -rf "${work:?}"' EXIT

casos_of() { sed -n 's/^CASOS=(\(.*\))$/\1/p' "$repo_root/demo/tutoriais/$1/run.sh"; }

ac001() {
  local t n c doc demo
  for t in "${tutorials[@]}"; do
    doc="$repo_root/docs/tutorials/$t.md"; demo="$repo_root/demo/tutoriais/$t"
    grep -q "^## Objetivo" "$doc" || fail "AC-001/$t/sem-objetivo"
    grep -q "^## Pré-requisitos" "$doc" || fail "AC-001/$t/sem-prerequisitos"
    grep -q "^## Problemas comuns" "$doc" || fail "AC-001/$t/sem-problemas-comuns"
    grep -q "^## Quando falha" "$doc" || fail "AC-001/$t/sem-caso-de-falha"
    n="$(grep -c '^## Caso [0-9]' "$doc" || true)"
    (( n >= 3 )) || fail "AC-001/$t/casos-insuficientes:$n"
    # cada caso tem comando, saida esperada e "o que observar"
    (( $(grep -c '^O que observar' "$doc" || true) >= n )) || fail "AC-001/$t/sem-o-que-observar"
    (( $(grep -c '^<!-- saida: ' "$doc" || true) >= n )) || fail "AC-001/$t/saidas-insuficientes"
    grep -q "docs/tutorials/$t.md\|$t.md" "$repo_root/docs/tutorials/README.md" || fail "AC-001/$t/fora-do-indice"
    # fases executaveis: pelo menos 4 (3 casos de uso + 1 de falha), cada uma com expected/ e out/
    local k=0
    for c in $(casos_of "$t"); do
      k=$((k + 1))
      [[ -s "$demo/expected/$c.txt" ]] || fail "AC-001/$t/expected-ausente:$c"
      [[ -s "$demo/out/$c.log" ]] || fail "AC-001/$t/out-ausente:$c"
      grep -q "^caso_${c//-/_}()" "$demo/run.sh" || fail "AC-001/$t/fase-ausente:$c"
    done
    (( k >= 4 )) || fail "AC-001/$t/fases-insuficientes:$k"
    # o nav: mkdocs.yml quando existe; senao, as entradas registradas na spec
    if [[ -f "$repo_root/mkdocs.yml" ]]; then
      # o hook do site injeta a aba Tutoriais de docs/tutorials/README.md + *.md
      grep -q "tutorials" "$repo_root/scripts/docs/hooks.py" || fail "AC-001/$t/hook-sem-tutoriais"
      grep -q "tutorials/$t.md\|$t.md" "$repo_root/docs/tutorials/README.md" || fail "AC-001/$t/fora-do-nav"
    else
      grep -q "tutorials/$t.md" "$spec" || fail "AC-001/$t/nav-nao-registrado-na-spec"
    fi
  done
  printf '%s/AC-001/ok (3 tutoriais, casos de uso e falha, indice e nav)\n' "$card"
}

ac002() {
  local t demo c nb=0
  for t in "${tutorials[@]}"; do
    demo="$repo_root/demo/tutoriais/$t"
    bash "$demo/run.sh" --check >"$work/check.out" 2>&1 || { cat "$work/check.out" >&2; fail "AC-002/$t/check-falhou"; }
    grep -q '^CHECK OK$' "$work/check.out" || fail "AC-002/$t/check-sem-ok"
    # cada bloco "saida" do texto e um trecho literal do out/ do caso
    awk -v dir="$demo/out" -v t="$t" '
      /^<!-- saida: / { c = $3; armed = 1; next }
      armed && !open && /^```/ { open = 1; next }
      open && /^```/ { open = 0; armed = 0; next }
      open { print c "\t" $0 }
    ' "$repo_root/docs/tutorials/$t.md" > "$work/saidas.tsv"
    [[ -s "$work/saidas.tsv" ]] || fail "AC-002/$t/sem-saidas-no-texto"
    while IFS=$'\t' read -r c line; do
      [[ -n "$line" ]] || continue
      grep -qF -- "$line" "$demo/out/$c.log" || fail "AC-002/$t/saida-do-texto-nao-esta-no-out:$c:$line"
      nb=$((nb + 1))
    done < "$work/saidas.tsv"
    # nenhuma fase terminou em ERRO
    if grep -l '^ERRO:' "$demo"/out/*.log | grep -q .; then fail "AC-002/$t/out-com-erro"; fi
  done
  grep -qiE 'execu(c|ç)(a|ã)o real' "$spec" || fail "AC-002/spec-sem-execucao-real"
  for t in "${tutorials[@]}"; do
    for c in $(casos_of "$t"); do
      grep -qF "$t/$c" "$spec" || fail "AC-002/spec-sem-registro:$t/$c"
    done
  done
  printf '%s/AC-002/ok (--check dos 3 tutoriais; %d linhas de saida do texto presentes no out/; execucao real na spec)\n' "$card" "$nb"
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

ac003() {
  local bin t n=0
  bin="$(aurum_bin)"
  "$bin" --help >"$work/help.txt" 2>&1 || true
  [[ -s "$work/help.txt" ]] || infra "help-vazio"
  # comandos citados: blocos de codigo dos tutoriais ("aurumcode ..." ou "$ aurumcode ...")
  # e as chamadas "aurum ..." de cada run.sh
  : > "$work/cmds.txt"
  for t in "${tutorials[@]}"; do
    awk '/^```/ { if (open) { open = 0 } else { open = ($0 == "```bash") } next } open && /^(\$ )?aurumcode / { sub(/^\$ /, ""); print }' "$repo_root/docs/tutorials/$t.md" >> "$work/cmds.txt"
    sed -n 's/^[[:space:]]*aurum \(review\|fix\) /aurumcode \1 /p' "$repo_root/demo/tutoriais/$t/run.sh" >> "$work/cmds.txt"
    sed -n 's/.*aurum_raw -- \(review\|fix\) /aurumcode \1 /p' "$repo_root/demo/tutoriais/$t/run.sh" >> "$work/cmds.txt"
  done
  [[ -s "$work/cmds.txt" ]] || fail "AC-003/nenhum-comando-extraido"
  local line sub tok name
  while IFS= read -r line; do
    line="${line%%#*}"; line="${line%%>*}"
    # shellcheck disable=SC2206
    local toks=($line)
    sub="${toks[1]:-}"
    grep -qE "^  $sub[[:space:]]" "$work/help.txt" || fail "AC-003/comando-inexistente:$sub ($line)"
    "$bin" "$sub" --help >"$work/help-$sub.txt" 2>&1 || true
    for tok in "${toks[@]:2}"; do
      case "$tok" in
        --*|-[a-z]*)
          name="${tok#--}"; name="${name#-}"; name="${name%%=*}"
          grep -qE "^  -{1,2}$name( |\$)" "$work/help-$sub.txt" || fail "AC-003/flag-inexistente:$sub --$name ($line)"
          n=$((n + 1)) ;;
      esac
    done
  done < <(sort -u "$work/cmds.txt")
  (( n >= 3 )) || fail "AC-003/poucas-flags-conferidas:$n"
  # as flags de politica e de gate citadas existem de fato
  for name in base fail-on; do
    grep -qE "^  -{1,2}$name( |\$)" "$work/help-review.txt" || fail "AC-003/review-sem-flag:$name"
  done
  # comandos dos blocos bash que nao sao do produto: scripts do board, go-shared, oci-run, go test
  local doc ln s sub name2 k=0
  for t in "${tutorials[@]}"; do
    doc="$repo_root/docs/tutorials/$t.md"
    awk '/^```/ { if (open) { open = 0 } else { open = ($0 == "```bash") } next } open { print }' "$doc" > "$work/blocks.txt"
    while IFS= read -r ln; do
      for s in $(grep -oE '\.board/bin/[a-z.-]+' <<<"$ln" || true); do
        [[ -f "$repo_root/$s" ]] || fail "AC-003/$t/script-inexistente:$s"; k=$((k + 1))
      done
      if [[ "$ln" =~ go-shared[[:space:]]+(up|exec|status|down)($|[[:space:]]) ]]; then
        sub="${BASH_REMATCH[1]}"; grep -qE "^[[:space:]]+$sub\)" "$repo_root/.board/bin/go-shared" || fail "AC-003/$t/go-shared-sem-subcomando:$sub"; k=$((k + 1))
      elif [[ "$ln" == *go-shared* ]]; then fail "AC-003/$t/go-shared-subcomando-invalido:$ln"; fi
      if [[ "$ln" == *oci-run* ]]; then
        for s in $(grep -oE -- '--[a-z-]+' <<<"$ln"); do
          grep -qF -- "'$s'" "$repo_root/.board/bin/oci-run" || grep -qE -- "^[[:space:]]+$s\)" "$repo_root/.board/bin/oci-run" || fail "AC-003/$t/oci-run-sem-flag:$s"; k=$((k + 1))
        done
      fi
      if [[ "$ln" == *"go test"* ]]; then
        for s in $(grep -oE -- '-run [A-Za-z0-9_]+' <<<"$ln" | cut -d' ' -f2); do
          grep -rqE "^func $s" "$repo_root/tests/benchmark" "$repo_root/internal/grammar" || fail "AC-003/$t/teste-inexistente:$s"; k=$((k + 1))
        done
        for s in $(grep -oE -- '-update-[a-z0-9]+' <<<"$ln" | cut -c2-); do
          grep -rqF "\"$s\"" "$repo_root/tests/benchmark" || fail "AC-003/$t/flag-de-teste-inexistente:$s"; k=$((k + 1))
        done
      fi
    done < "$work/blocks.txt"
  done
  (( k >= 12 )) || fail "AC-003/poucas-conferencias-do-board:$k"
  printf '%s/AC-003/ok (%d comandos do produto, %d flags conferidas contra --help; %d conferencias de scripts do board, go-shared, oci-run e go test)\n' "$card" "$(sort -u "$work/cmds.txt" | wc -l)" "$n" "$k"
}

ac004() {
  local t doc path n total=0
  for t in "${tutorials[@]}"; do
    doc="$repo_root/docs/tutorials/$t.md"; n=0
    while IFS= read -r path; do
      n=$((n + 1))
      [[ -f "$repo_root/$path" ]] || fail "AC-004/$t/arquivo-inexistente:$path"
      awk -v want="<!-- arquivo: $path -->" '
        $0 == want { armed = 1; next }
        armed && !open && /^```/ { open = 1; next }
        open && /^```/ { exit }
        open { print }
      ' "$doc" > "$work/bloco"
      [[ -s "$work/bloco" ]] || fail "AC-004/$t/bloco-ausente:$path"
      diff -u "$repo_root/$path" "$work/bloco" >/dev/null || fail "AC-004/$t/bloco-diverge:$path"
    done < <(sed -n 's/^<!-- arquivo: \(.*\) -->$/\1/p' "$doc")
    (( n >= 3 )) || fail "AC-004/$t/blocos-insuficientes:$n"
    total=$((total + n))
  done
  # AC-004 do card: cada script de .board/bin e cada profile registrado e citado no tutorial de operacao
  local op="$repo_root/docs/tutorials/operacao.md" key
  for f in "$repo_root"/.board/bin/*; do
    name="${f##*/}"
    grep -qF "\`$name\`" "$op" || fail "AC-004/operacao-nao-cita-script:$name"
  done
  while IFS= read -r key; do
    grep -qF "\`$key\`" "$op" || fail "AC-004/operacao-nao-cita-profile:$key"
  done < <(grep -oE '"key"[[:space:]]*:[[:space:]]*"[^"]+"' "$repo_root/.board/oci/profiles/registry.v1.json" | sed -E 's/.*"([^"]+)"$/\1/')
  (( $(grep -oE '"key"[[:space:]]*:' "$repo_root/.board/oci/profiles/registry.v1.json" | wc -l) >= 11 )) || fail "AC-004/registry-com-menos-de-11-profiles"
  grep -qF '`trust-root-docker-v1`' "$op" || fail "AC-004/operacao-nao-cita-profile-fora-do-registro"
  # so dominios reservados; nenhum segredo
  local files=() f host bad=''
  while IFS= read -r f; do files+=("$f"); done < <(find "$repo_root/demo/tutoriais" "$repo_root/docs/tutorials" "$spec" -type f ! -path '*/.estado/*')
  local allowed dl="$repo_root/demo/tutoriais/_lib/dominios-permitidos.txt" dline alt=''
  [[ -f "$dl" ]] || infra "missing:demo/tutoriais/_lib/dominios-permitidos.txt"
  while IFS= read -r dline || [[ -n "$dline" ]]; do
    case "$dline" in ''|'#'*) continue ;; esac
    dline="${dline//./\\.}"
    if [[ "$dline" == \\.* ]]; then alt="$alt|([A-Za-z0-9-]+\\.)*${dline#\\.}"; else alt="$alt|$dline"; fi
  done < "$dl"
  allowed="^(${alt#|})\$"
  while IFS= read -r host; do
    host="${host#*://}"; host="${host%%[:/]*}"
    [[ "$host" =~ $allowed ]] || bad="$bad $host"
  done < <(grep -rhoE 'https?://[^/[:space:]"'"'"')`>]+' "${files[@]}" || true)
  [[ -z "$bad" ]] || fail "AC-004/url-real:$bad"
  bad=''
  while IFS= read -r host; do
    [[ "$host" =~ $allowed ]] || bad="$bad $host"
  done < <(grep -rhoE '\b([A-Za-z0-9-]+\.)+(com|org|net|io|dev|app|cloud|co|ai|local|internal|corp|lan|br|gov|edu)\b' "${files[@]}" || true)
  [[ -z "$bad" ]] || fail "AC-004/dominio-real:$bad"
  bad="$(grep -rhoE '[A-Za-z0-9._-]+@([A-Za-z0-9-]+\.)+[A-Za-z]{2,}' "${files[@]}" | grep -vE '@(example\.(com|org|net)|[A-Za-z0-9-]+\.invalid|[A-Za-z0-9-]+\.test)$' || true)"
  [[ -z "$bad" ]] || fail "AC-004/email-real:$bad"
  if grep -rIlE 'BEGIN [A-Z ]*PRIVATE KEY|ghp_[A-Za-z0-9]{20}|github_pat_|sk-[A-Za-z0-9]{20}|xox[bp]-|AKIA[0-9A-Z]{16}' "${files[@]}" | grep -q .; then
    fail "AC-004/segredo-versionado"
  fi
  if grep -rIlE 'LLM_API_KEY=[^ ]+|api[_-]?key[\"'"'"']?[:=][ ]*[\"'"'"'][A-Za-z0-9]{16,}' "${files[@]}" | grep -q .; then
    fail "AC-004/credencial-literal"
  fi
  printf '%s/AC-004/ok (%d blocos identicos aos arquivos da demo; so dominios reservados; sem segredo)\n' "$card" "$total"
}

mut001() {
  local t c first copy
  for t in "${tutorials[@]}"; do
    copy="$work/mut-$t"
    mkdir -p "$copy"
    cp -R "$repo_root/demo/tutoriais/$t/." "$copy/"
    chmod -R u+rwX "$copy"
    bash "$repo_root/demo/tutoriais/$t/run.sh" --check >/dev/null 2>&1 || fail "AC-002-MUT-001/$t/controle-nao-passa"
    for c in $(casos_of "$t"); do
      # a linha que prova o caso: a primeira linha esperada que nao e o eco do comando
      first="$(grep -vE '^(#|$)' "$copy/expected/$c.txt" | grep -vF '$ aurumcode' | head -n1)"
      [[ -n "$first" ]] || first="$(grep -vE '^(#|$)' "$copy/expected/$c.txt" | head -n1)"
      grep -vF -- "$first" "$repo_root/demo/tutoriais/$t/out/$c.log" > "$copy/out/$c.log" || true
      if grep -qF -- "$first" "$copy/out/$c.log"; then fail "AC-002-MUT-001/$t/$c/linha-nao-removida"; fi
      # roda o --check da copia: o run.sh da copia usa o _lib relativo a ela
      mkdir -p "$work/m-$t/$c/_lib" "$work/m-$t/$c/$t"
      cp "$repo_root/demo/tutoriais/_lib/tutorial.sh" "$work/m-$t/$c/_lib/"
      cp -R "$copy/." "$work/m-$t/$c/$t/"
      if bash "$work/m-$t/$c/$t/run.sh" --check >"$work/mut.out" 2>&1; then
        fail "AC-002-MUT-001/$t/$c/mutacao-sobreviveu"
      fi
      grep -q "DIVERGENCIA caso=$c" "$work/mut.out" || fail "AC-002-MUT-001/$t/$c/divergencia-nao-nomeada"
      cp "$repo_root/demo/tutoriais/$t/out/$c.log" "$copy/out/$c.log"
    done
  done
  printf '%s/AC-002-MUT-001/ok (sem a linha que prova o caso, --check diverge em todos os casos)\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  AC-002-MUT-001) mut001 ;;
  all) ac001; ac002; ac003; ac004; mut001 ;;
esac
