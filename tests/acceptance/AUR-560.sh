#!/usr/bin/env bash
#
# Acceptance program for card AUR-560 (searchable documentation site built in
# a digest-pinned container and published by workflow). Offline, bash + awk
# + grep + sed only: the sealed profile has no python and no network. The
# real MkDocs build runs outside the sandbox (scripts/docs/build.sh); its
# --strict log and the search index hash are recorded in docs/specs/AUR-560.md
# and checked here.
#
# SELECTORS
#   all             AC-001, AC-002, AC-003, AC-004 and AC-002-MUT-001
#   AC-001          mkdocs.yml/scripts declare the strict, searchable build
#                   and the spec records a warning-free real build
#   AC-002          every docs/**/*.md (specs included) is in the nav, every
#                   internal link and anchor resolves, mkdocs.yml cites only
#                   files that exist
#   AC-003          image by digest, actions by commit SHA, Pages permissions
#                   only on the deploy job
#   AC-004          docs/index.md cites each configuration.md section with a
#                   link and each capability with its tutorial
#   AC-002-MUT-001  removing a page (or a spec) from the nav in a copy turns
#                   AC-002 red
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = holds, 1 = behavioral RED, 64 = unknown selector, 79 = infrastructure
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-560'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
for input in mkdocs.yml docs/index.md docs/configuration.md docs/specs/AUR-560.md \
  scripts/docs/build.sh scripts/docs/serve.sh scripts/docs/image.lock \
  .github/workflows/docs.yml; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a560.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$work"' EXIT INT TERM HUP

# slug: the python-markdown toc slug MkDocs uses, for Portuguese text.
slug() {
  printf '%s' "$1" | sed -e 's/á/a/g;s/à/a/g;s/â/a/g;s/ã/a/g;s/ä/a/g;s/é/e/g;s/ê/e/g;s/í/i/g;s/ó/o/g;s/ô/o/g;s/õ/o/g;s/ú/u/g;s/ü/u/g;s/ç/c/g' \
    -e 's/Á/A/g;s/É/E/g;s/Í/I/g;s/Ó/O/g;s/Ú/U/g;s/Ç/C/g' \
    | tr 'A-Z' 'a-z' | sed -e 's/[^a-z0-9_ -]//g' -e 's/^[[:space:]]*//;s/[[:space:]]*$//' -e 's/[[:space:]-][[:space:]-]*/-/g'
}

# headings <file>: heading text of each ATX heading outside code fences.
headings() {
  awk '/^```/{f=!f;next} !f && /^#+ /{sub(/^#+ +/,"");sub(/ +#+ *$/,"");print}' "$1"
}

# nav_files <mkdocs.yml>: every .md cited under the top-level nav key.
nav_files() {
  awk '/^nav:/{n=1;next} /^[^ #]/{n=0} n && /\.md[[:space:]]*$/{
         s=$0; sub(/[[:space:]]*$/,"",s); sub(/^.*[ :]-?[[:space:]]*/,"",s); sub(/^.*: /,"",s); print s}' "$1" | sort
}

# ac002_check <root>: prints the first violation and returns 1, or returns 0.
ac002_check() {
  local root="$1" slugs h f t base dir target anchor line problems=0
  local docs="$root/docs"
  nav_files "$root/mkdocs.yml" >"$work/nav.txt"
  (cd "$docs" && find . -name '*.md' -not -path './site/*' | sed 's#^\./##' | sort) >"$work/pages.txt"
  [[ -s "$work/nav.txt" && -s "$work/pages.txt" ]] || { echo "empty-nav-or-docs"; return 1; }
  if line="$(comm -13 "$work/nav.txt" "$work/pages.txt" | head -n1)" && [[ -n "$line" ]]; then
    echo "orphan-page-not-in-nav:$line"; return 1
  fi
  if line="$(comm -23 "$work/nav.txt" "$work/pages.txt" | head -n1)" && [[ -n "$line" ]]; then
    echo "nav-cites-missing-file:$line"; return 1
  fi
  if [[ "$(uniq -d "$work/nav.txt" | head -n1)" != '' ]]; then echo "nav-duplicate:$(uniq -d "$work/nav.txt" | head -n1)"; return 1; fi
  # other files mkdocs.yml cites (extra_css under docs_dir, hooks from root)
  for t in $(awk '/^extra_css:/{c=1;next} /^[^ #]/{c=0} c && /^ +- /{sub(/^ +- /,"");print}' "$root/mkdocs.yml"); do
    [[ -f "$docs/$t" ]] || { echo "mkdocs-cites-missing-file:$t"; return 1; }
  done
  for t in $(awk '/^hooks:/{c=1;next} /^[^ #]/{c=0} c && /^ +- /{sub(/^ +- /,"");print}' "$root/mkdocs.yml"); do
    [[ -f "$root/$t" ]] || { echo "mkdocs-cites-missing-file:$t"; return 1; }
  done
  # internal links and anchors
  while IFS= read -r f; do
    dir="$(dirname "$f")"
    while IFS= read -r target; do
      case "$target" in
        http:*|https:*|mailto:*|ftp:*|'') continue ;;
      esac
      anchor=''; [[ "$target" == *'#'* ]] && anchor="${target#*#}"
      t="${target%%#*}"
      if [[ -z "$t" ]]; then base="$f"; else
        base="$(realpath -m --relative-to="$docs" "$docs/$dir/$t")"
        [[ -e "$docs/$base" ]] || { echo "broken-link:$f -> $target"; return 1; }
      fi
      if [[ -n "$anchor" && "$base" == *.md ]]; then
        slugs=''
        while IFS= read -r h; do slugs+="$(slug "$h")"$'\n'; done < <(headings "$docs/$base")
        grep -qxF -- "$anchor" <<<"$slugs" || { echo "broken-anchor:$f -> $target"; return 1; }
      fi
    done < <(awk '/^```/{f=!f;next} !f' "$docs/$f" | sed -e 's/`[^`]*`//g' | grep -oE '\]\([^) ]+\)' | sed -e 's/^](//;s/)$//' || true)
  done <"$work/pages.txt"
  return 0
}

ac001() {
  local spec="$repo_root/docs/specs/AUR-560.md" y="$repo_root/mkdocs.yml" lock log h
  for needle in 'docs_dir: docs' 'name: material' 'language: pt-BR' 'navigation.tabs' 'navigation.indexes' \
    'content.code.copy' 'strict: true' 'lang: [pt]' '- search:' 'nav:'; do
    grep -qF -- "$needle" "$y" || fail "mkdocs.yml-missing:$needle"
  done
  grep -qE '^plugins:' "$y" || fail mkdocs.yml-plugins
  grep -q 'build --strict' "$repo_root/scripts/docs/build.sh" || fail build.sh-not-strict
  grep -q 'scripts/docs/image.lock' "$repo_root/scripts/docs/build.sh" || fail build.sh-not-using-lock
  grep -q 'scripts/docs/image.lock' "$repo_root/scripts/docs/serve.sh" || fail serve.sh-not-using-lock
  grep -q '127.0.0.1:8000' "$repo_root/scripts/docs/serve.sh" || fail serve.sh-not-local
  lock="$(head -n1 "$repo_root/scripts/docs/image.lock")"
  grep -qF -- "$lock" "$spec" || fail spec-missing-image-digest
  # the recorded real build: a fenced log with the success line and no warning
  log="$(awk '/^```text build-strict-log/{f=1;next} /^```/{f=0} f' "$spec")"
  [[ -n "$log" ]] || fail spec-missing-build-log
  grep -q 'Documentation built in' <<<"$log" || fail spec-log-without-success-line
  if grep -qE '^(WARNING|ERROR)|Aborted' <<<"$log"; then fail spec-log-has-warning; fi
  h="$(sed -n 's/^search_index_sha256: \([0-9a-f]\{64\}\)$/\1/p' "$spec" | head -n1)"
  [[ -n "$h" ]] || fail spec-missing-search-index-sha256
  grep -qE '^search_index_docs: [1-9][0-9]*$' "$spec" || fail spec-missing-search-index-docs
  # when a local build exists, it must have a real index and tab navigation
  if [[ -f "$repo_root/site/search/search_index.json" ]]; then
    grep -q '"docs"' "$repo_root/site/search/search_index.json" || fail site-search-index-invalid
    grep -q 'md-tabs' "$repo_root/site/index.html" || fail site-without-tabs
  fi
}

ac002() {
  local out
  out="$(ac002_check "$repo_root")" || fail "$out"
}

ac003() {
  local wf="$repo_root/.github/workflows/docs.yml" lock line n
  lock="$(head -n1 "$repo_root/scripts/docs/image.lock")"
  [[ "$(wc -l <"$repo_root/scripts/docs/image.lock")" -eq 1 ]] || fail image.lock-not-one-line
  [[ "$lock" =~ ^squidfunk/mkdocs-material@sha256:[0-9a-f]{64}$ ]] || fail image-not-by-digest
  grep -q 'scripts/docs/build.sh' "$wf" || fail workflow-not-using-build.sh
  if grep -qE 'mkdocs-material(:|[[:space:]]|$)' "$wf"; then fail workflow-names-image-without-digest; fi
  grep -qE 'pip[3]? install' "$wf" && fail workflow-installs-with-pip
  n=0
  while IFS= read -r line; do
    n=$((n+1))
    [[ "$line" =~ uses:[[:space:]]*[A-Za-z0-9._/-]+@[0-9a-f]{40}([[:space:]]|$) ]] || fail "action-not-pinned-by-sha:$line"
  done < <(grep -E '^[[:space:]]*(-[[:space:]]*)?uses:' "$wf")
  [[ $n -ge 4 ]] || fail "too-few-actions:$n"
  for a in actions/configure-pages actions/upload-pages-artifact actions/deploy-pages; do
    grep -qE "uses: $a@" "$wf" || fail "workflow-missing-action:$a"
  done
  grep -qE '^[[:space:]]+- uses: actions/deploy-pages@|^[[:space:]]+uses: actions/deploy-pages@' "$wf" || fail deploy-pages-step
  grep -q -- '--strict\|scripts/docs/build.sh' "$wf" || fail workflow-not-strict
  grep -qE '^[[:space:]]+branches: \[main\]' "$wf" || fail workflow-not-on-main
  grep -q 'workflow_dispatch' "$wf" || fail workflow-no-dispatch
  # permissions: top level (if any) grants no Pages/OIDC; only the deploy job does
  awk '
    /^permissions:/{top=1;job="";next}
    /^[^ #]/{top=0}
    /^jobs:/{injobs=1}
    /^  [A-Za-z0-9_-]+:[[:space:]]*$/ && injobs{job=$1}
    /(pages|id-token):[[:space:]]*write/{ if (top) print "TOP"; else print job }
  ' "$wf" | sort -u >"$work/perm.txt"
  grep -qx 'deploy:' "$work/perm.txt" || fail deploy-job-lacks-pages-permission
  if [[ "$(grep -vx 'deploy:' "$work/perm.txt" | head -n1)" != '' ]]; then fail "pages-permission-outside-deploy:$(grep -vx 'deploy:' "$work/perm.txt" | head -n1)"; fi
  grep -qE 'needs:[[:space:]]*build' "$wf" || fail deploy-without-needs-build
  grep -qE 'name: github-pages' "$wf" || fail deploy-without-environment
  for p in pages id-token; do
    [[ "$(grep -cE "$p:[[:space:]]*write" "$wf")" -eq 1 ]] || fail "permission-$p-not-exactly-once"
  done
}

ac004() {
  local idx="$repo_root/docs/index.md" cfg="$repo_root/docs/configuration.md" h s count=0 sec name
  while IFS= read -r h; do
    s="$(slug "$h")"; count=$((count+1))
    grep -qF -- "](configuration.md#$s)" "$idx" || fail "index-missing-capability:configuration.md#$s"
  done < <(awk '/^```/{f=!f;next} !f && /^## /{sub(/^## +/,"");print}' "$cfg")
  [[ $count -ge 10 ]] || fail "too-few-config-sections:$count"
  # every capability section of the index names its tutorial: a link once the
  # file exists, "em breve (AUR-56x, `tutorials/x.md`)" until then
  awk '/^## /{sec=$0} /^Tutorial:/{print sec "\t" $0; seen[sec]=1} /^## /{order[++n]=$0} END{for(i=1;i<=n;i++) if(!(order[i] in seen) && order[i] !~ /Especifica/) print order[i] "\tMISSING"}' "$idx" >"$work/tut.txt"
  grep -q MISSING "$work/tut.txt" && fail "capability-without-tutorial-line:$(grep MISSING "$work/tut.txt" | head -n1 | cut -f1)"
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    if [[ -f "$repo_root/docs/tutorials/$name.md" ]]; then
      grep -qF -- "](tutorials/$name.md)" "$idx" || fail "tutorial-exists-but-not-linked:tutorials/$name.md"
    fi
  done < <(grep -oE 'em breve \(AUR-56[0-9], `tutorials/[a-z0-9-]+\.md`\)' "$idx" | sed -e 's#.*tutorials/##;s#\.md.*##')
  # a link to a tutorial must point at a file that exists
  while IFS= read -r name; do
    [[ -z "$name" || -f "$repo_root/docs/$name" ]] || fail "index-links-missing-tutorial:$name"
  done < <(grep -oE '\]\(tutorials/[^)]+\)' "$idx" | sed -e 's/^](//;s/)$//')
  [[ "$(grep -cE '^## ' "$idx")" -ge 6 ]] || fail index-too-few-sections
}

mut001() {
  local m="$work/mut" victim
  mkdir -p "$m"
  cp "$repo_root/mkdocs.yml" "$m/"
  cp -R "$repo_root/docs" "$m/docs"
  cp -R "$repo_root/scripts" "$m/scripts"
  ac002_check "$m" >/dev/null || fail "mutation-baseline-not-green"
  for victim in 'review-cache.md' 'specs/AUR-001.md'; do
    cp "$repo_root/mkdocs.yml" "$m/mkdocs.yml"
    grep -qE "^[[:space:]]+- $victim\$" "$m/mkdocs.yml" || fail "mutation-victim-absent:$victim"
    grep -vE "^[[:space:]]+- $victim\$" "$repo_root/mkdocs.yml" >"$m/mkdocs.yml"
    if ac002_check "$m" >"$work/mut.out"; then fail "mutation-survived:$victim"; fi
    grep -q "orphan-page-not-in-nav:$victim" "$work/mut.out" || fail "mutation-wrong-reason:$victim"
  done
  # a broken link must also redden AC-002
  cp "$repo_root/mkdocs.yml" "$m/mkdocs.yml"
  printf '\n[quebrado](nao-existe.md)\n' >>"$m/docs/index.md"
  if ac002_check "$m" >/dev/null; then fail mutation-survived:broken-link; fi
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  AC-002-MUT-001) mut001 ;;
  all) ac001; ac002; ac003; ac004; mut001 ;;
esac
printf '%s/%s/ok\n' "$card" "$selector"
