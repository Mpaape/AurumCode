#!/usr/bin/env bash
# AUR-588 acceptance: the documentation is visual and reproducible. Every
# capability page (tutorials, extension guide, architecture, configuration,
# corporate guide, index) has desktop and mobile screenshots, every tutorial
# case has the render of what the user sees (terminal, PR comment, status
# checks) in a "Como fica" section, and docs/assets/capturas/capturas.json
# records, per image, the digest of its input, the digest-pinned Playwright
# image and the command (scripts/docs/capturas.sh).
#
# The sealed profile has no docker, browser or python: `all` checks the
# manifest, the referenced images, the input digests and the "Como fica"
# sections offline (scripts/docs/capturas-check.sh), plus the static wiring of
# the generator, the browser test and the workflows. `full` (host with docker)
# regenerates the screenshots twice in a copy and runs the Chromium test.
#
# Selectors:
#   all      AC-001 AC-002 AC-003 AC-004 MUT-001 MUT-002 (offline)
#   AC-001   generator pinned by digest, manifest covers every page and case,
#            carries no date and no absolute path
#   AC-002   every tutorial has "Como fica" with a capture per case; index and
#            extension guide reference captures
#   AC-003   browser test asserts nav, the six search terms, captures vs
#            manifest, console and mobile scroll; CI runs it with the pinned image
#   AC-004   offline check of manifest, presence and input digests
#   MUT-001  removing a referenced capture turns AC-004 red (full: AC-003 too)
#   MUT-002  changing an out/ without regenerating turns AC-004 red
#   full     host only: capturas.sh twice in a copy (same manifest as the
#            committed one, same file set), Chromium test, MUT-001 in Chromium
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-588'
selector="${1:-all}"
case "$selector" in
  all|full|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
readonly manifesto='docs/assets/capturas/capturas.json'
readonly checker='scripts/docs/capturas-check.sh'
for input in "$manifesto" "$checker" scripts/docs/capturas.sh scripts/docs/capturas.cjs scripts/docs/png-sem-perda.cjs \
  scripts/docs/playwright.lock tests/docs/mkdocs.test.cjs .github/workflows/ci.yml .github/workflows/docs.yml \
  mkdocs.yml docs/index.md docs/extensao.md demo/tutoriais; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a588.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "${run_dir:?}" >/dev/null 2>&1 || true; rm -rf -- "${run_dir:?}" >/dev/null 2>&1 || true' EXIT INT TERM HUP

# stage copies what the screenshots depend on (docs, tutorials, generator,
# site configuration, workflows and tests) to a fresh root.
stage() {
  local root="${1:?}" source
  mkdir -p "$root/demo" "$root/scripts" "$root/tests" "$root/.github"
  for source in docs mkdocs.yml scripts/docs demo/tutoriais tests/docs .github/workflows; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  find "$root/demo/tutoriais" -name .estado -prune -exec rm -rf {} + 2>/dev/null || true
  rm -rf "${root:?}/site"
  chmod -R u+w -- "$root"
}

# check root: the offline checker; output in $run_dir/check.log, rc returned.
check() { bash "$1/$checker" "$1" >"$run_dir/check.log" 2>&1; }

lock="$(sed -n '1p' "$repo_root/scripts/docs/playwright.lock")"

ac_001() {
  [[ "$lock" =~ ^mcr\.microsoft\.com/playwright:v[0-9.]+-noble@sha256:[0-9a-f]{64}$ ]] || fail AC-001/playwright-not-by-digest
  grep -Fq 'scripts/docs/playwright.lock' "$repo_root/scripts/docs/capturas.sh" || fail AC-001/generator-ignores-lock
  grep -Fq 'scripts/docs/build.sh' "$repo_root/scripts/docs/capturas.sh" || fail AC-001/generator-does-not-build
  grep -Fq 'reducedMotion: "reduce"' "$repo_root/scripts/docs/capturas.cjs" || fail AC-001/no-reduced-motion
  grep -Fq 'r.abort()' "$repo_root/scripts/docs/capturas.cjs" || fail AC-001/external-requests-allowed
  # Reproducible manifest: no date and no absolute path of the machine that ran it.
  if grep -Eq '"(data|date|gerado_em|timestamp)"' "$repo_root/$manifesto"; then fail AC-001/manifest-has-date; fi
  if grep -Eq '"(/home|/tmp|/src|/Users)' "$repo_root/$manifesto"; then fail AC-001/manifest-has-absolute-path; fi
  local n
  n="$(grep -c '^ *{"imagem":' "$repo_root/$manifesto" || true)"
  (( n >= 100 )) || fail "AC-001/manifest-too-small:$n"
  printf 'AC-001: generator pinned (%s), %s images in the manifest, no date, no absolute path\n' "${lock##*@}" "$n"
}

ac_002() {
  local doc t n=0
  for doc in "$repo_root"/docs/tutorials/*.md; do
    t="$(basename "$doc" .md)"
    [[ "$t" == README ]] && continue
    [[ -d "$repo_root/demo/tutoriais/$t/out" ]] || continue
    grep -q '^## Como fica$' "$doc" || fail "AC-002/$t/sem-como-fica"
    n=$((n + 1))
  done
  (( n >= 15 )) || fail "AC-002/too-few-tutorials:$n"
  grep -Eq '!\[[^]]*\]\(assets/capturas/[^)]+\.png\)' "$repo_root/docs/index.md" || fail AC-002/index-sem-captura
  grep -Eq '!\[[^]]*\]\(assets/capturas/[^)]+\.png\)' "$repo_root/docs/extensao.md" || fail AC-002/extensao-sem-captura
  grep -Eq '^strict: true$' "$repo_root/mkdocs.yml" || fail AC-002/mkdocs-not-strict
  printf 'AC-002: %s tutorials with "Como fica"; index and extension guide reference captures\n' "$n"
}

ac_003() {
  local test="$repo_root/tests/docs/mkdocs.test.cjs" term needle ci="$repo_root/.github/workflows/ci.yml"
  for term in engine gitleaks deliberacao skill ContextProvider 'excecao proposta'; do
    grep -Fq "\"$term\"" "$test" || fail "AC-003/search-term-missing:$term"
  done
  for needle in 'md-nav--primary' 'capturas.json' 'scrollWidth' '"console"' 'requisicao externa'; do
    grep -Fq "$needle" "$test" || fail "AC-003/assertion-missing:$needle"
  done
  grep -Fq "$lock" "$ci" || fail AC-003/ci-image-differs-from-lock
  grep -Fq 'node tests/docs/mkdocs.test.cjs' "$ci" || fail AC-003/ci-does-not-run-mkdocs-test
  grep -Fq 'scripts/docs/build.sh' "$ci" || fail AC-003/ci-does-not-build-site
  grep -Fq 'scripts/docs/capturas-check.sh' "$repo_root/.github/workflows/docs.yml" || fail AC-003/docs-workflow-does-not-check
  printf 'AC-003: browser test asserts nav, six terms, captures vs manifest; CI runs it with %s\n' "${lock##*@}"
}

ac_004() {
  check "$repo_root" || { cat "$run_dir/check.log" >&2; fail AC-004/check-red; }
  printf 'AC-004: %s\n' "$(sed -n '$p' "$run_dir/check.log")"
}

# A referenced capture of the gate tutorial, removed in a copy.
readonly mut_img='docs/assets/capturas/gate/status-pr-status.png'
readonly mut_site_img='assets/capturas/gate/status-pr-status.png'
mut_001() {
  local root="$run_dir/mut1"
  stage "$root"
  check "$root" || { cat "$run_dir/check.log" >&2; infra MUT-001/baseline-red; }
  grep -Fq "$mut_site_img" "$root/docs/tutorials/gate.md" || infra MUT-001/capture-not-referenced
  rm -f "${root:?}/${mut_img:?}"
  if check "$root"; then fail MUT-001/removed-capture-still-green; fi
  grep -Fq "$mut_img: captura referenciada ausente" "$run_dir/check.log" || { cat "$run_dir/check.log" >&2; fail MUT-001/wrong-reason; }
  printf 'MUT-001: removing %s -> RED: %s\n' "$mut_img" "$(grep -F 'referenciada ausente' "$run_dir/check.log" | sed -n '1p')"
}

readonly mut_log='demo/tutoriais/gate/out/status-pr.log'
mut_002() {
  local root="$run_dir/mut2"
  stage "$root"
  check "$root" || { cat "$run_dir/check.log" >&2; infra MUT-002/baseline-red; }
  printf 'status publicado: context=aurumcode/policy-gate state=success\n' >>"$root/$mut_log"
  if check "$root"; then fail MUT-002/changed-out-still-green; fi
  grep -Fq "insumo $mut_log mudou sem regenerar" "$run_dir/check.log" || { cat "$run_dir/check.log" >&2; fail MUT-002/wrong-reason; }
  printf 'MUT-002: changing %s -> RED: %s\n' "$mut_log" "$(grep -F 'mudou sem regenerar' "$run_dir/check.log" | sed -n '1p')"
}

# ------------------------------------------------------------------ full (host)
browser_test() {
  local root="$1"
  docker run --rm --ipc=host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -v "$root:/src:ro" -v "$npm_cache:/qa" -w /src "$lock" \
    bash -c '[ -d /qa/node_modules/playwright ] || npm install --silent --no-audit --no-fund --prefix /qa playwright@1.58.2 >/dev/null; NODE_PATH=/qa/node_modules node tests/docs/mkdocs.test.cjs'
}

full() {
  command -v docker >/dev/null 2>&1 || infra missing_docker
  npm_cache="${AURUM_DOCS_NPM_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/aurumcode-docs}"
  mkdir -p "$npm_cache"
  local root="$run_dir/full" run
  stage "$root"
  for run in 1 2; do
    bash "$root/scripts/docs/capturas.sh" >"$run_dir/capturas-$run.log" 2>&1 || { tail -20 "$run_dir/capturas-$run.log" >&2; fail "full/capturas-run-$run"; }
    cp "$root/$manifesto" "$run_dir/manifesto-$run.json"
    ( cd "$root/docs/assets/capturas" && find . -type f | sort ) >"$run_dir/arquivos-$run.txt"
  done
  cmp -s "$run_dir/manifesto-1.json" "$run_dir/manifesto-2.json" || fail full/AC-001/manifest-differs-between-runs
  cmp -s "$run_dir/arquivos-1.txt" "$run_dir/arquivos-2.txt" || fail full/AC-001/file-set-differs-between-runs
  cmp -s "$run_dir/manifesto-1.json" "$repo_root/$manifesto" || { diff "$run_dir/manifesto-1.json" "$repo_root/$manifesto" | sed -n '1,10p' >&2; fail full/AC-001/committed-manifest-not-reproduced; }
  ( cd "$repo_root/docs/assets/capturas" && find . -type f | sort ) | cmp -s - "$run_dir/arquivos-1.txt" || fail full/AC-001/committed-file-set-not-reproduced
  if grep -q WARNING "$run_dir/capturas-1.log"; then fail full/AC-002/build-warnings; fi
  printf 'full AC-001: two runs, same manifest (= committed) and same %s files\n' "$(wc -l <"$run_dir/arquivos-1.txt" | tr -d ' ')"
  check "$root" || { cat "$run_dir/check.log" >&2; fail full/AC-004/check-red-after-regeneration; }
  browser_test "$root" >"$run_dir/browser.log" 2>&1 || { tail -20 "$run_dir/browser.log" >&2; fail full/AC-003/browser-red; }
  printf 'full AC-003: %s\n' "$(grep '^PASS' "$run_dir/browser.log")"
  # MUT-001 in Chromium: the built site without one referenced capture.
  rm -f "${root:?}/site/${mut_site_img:?}"
  if browser_test "$root" >"$run_dir/browser-mut.log" 2>&1; then fail full/MUT-001/browser-still-green; fi
  grep -Fq "capture does not load: /$mut_site_img" "$run_dir/browser-mut.log" || { tail -20 "$run_dir/browser-mut.log" >&2; fail full/MUT-001/wrong-reason; }
  printf 'full MUT-001: Chromium RED: %s\n' "$(grep -F 'capture does not load' "$run_dir/browser-mut.log" | sed -n '1p')"
}

case "$selector" in
  AC-001) ac_001 ;;
  AC-002) ac_002 ;;
  AC-003) ac_003 ;;
  AC-004) ac_004 ;;
  MUT-001) mut_001 ;;
  MUT-002) mut_002 ;;
  full) full ;;
  all) ac_001; ac_002; ac_003; ac_004; mut_001; mut_002 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
