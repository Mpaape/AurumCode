#!/usr/bin/env bash
#
# Acceptance program for card AUR-541.
#
# WHAT THIS PROVES
#
#   AUR-540 found that tests/e2e/AUR-467.sh's offline model fixture
#   predated the scope-and-evidence gate (internal/review/scope.go's
#   filterModelIssues) and so had its only finding silently discarded,
#   letting the e2e run on "no findings" instead of what it promised.
#   AUR-541 audited every tests/e2e/*.sh that feeds AURUMCODE_LLM_FIXTURE a
#   real value and found the same defect in several more scripts (see
#   docs/specs/AUR-541.md for the full measured table). This program is
#   the automated guard: it runs every such e2e script with a `go` shim on
#   PATH that wraps the built `aurumcode` binary so the scope gate's
#   stderr line survives even when a script redirects the binary's own
#   stderr into a temp file it deletes on exit, and fails if any script
#   reports a discard it has not declared it expects via a
#   "AUR-541: expect-gate-discard" comment in its own source.
#
# KNOWN LIMITATION (documented, not hidden): the declaration check is
# file-level (any such comment anywhere in the script silences every
# discard line from that script's run), not per-reason or per-fixture.
# tests/e2e/AUR-434.sh and tests/e2e/AUR-448.sh each carry such a comment
# for fixtures that mix a corrected (evidence-bearing) finding with
# deliberately-ungrounded ones; AC-001-MUT-001 therefore exercises the
# detector against tests/e2e/AUR-461.sh instead, which carries no such
# comment at all.
#
# SELECTORS
#   all             run every scenario below
#   AC-001          every e2e script that sets AURUMCODE_LLM_FIXTURE to a
#                   real value is run once; any undeclared scope-gate
#                   discard line on the binary's stderr is a failure
#   AC-002          the scripts this card corrected (tests/e2e/AUR-431.sh,
#                   AUR-432.sh, AUR-433.sh, AUR-434.sh, AUR-435.sh,
#                   AUR-436.sh, AUR-441.sh, AUR-461.sh, AUR-475.sh) each
#                   still exit 0 -- the measured "continues to prove what
#                   it promised" half of the card's outcome
#   AC-001-MUT-001  a staged copy of tests/e2e/AUR-461.sh with the
#                   evidence/impact/verification fields stripped back out
#                   of its catalog-id.json fixture must make AC-001's
#                   detector report an undeclared discard
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-541'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
real_go="$(command -v go)" || infra missing_go

required_inputs=(
  go.mod go.sum cmd/aurumcode internal/review tests/e2e
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a541.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

discard_log="$run_dir/discard.log"
: >"$discard_log"

# Every e2e script this program drives falls back to its OWN per-run
# GOCACHE/GOTMPDIR only when neither is already set (`: "${GOCACHE:=...}"`).
# Exporting a single shared pair here, before any script runs, means every
# `go build` after the first one for the same source is a cache hit instead
# of a cold recompile -- this is what keeps ~20 sequential e2e builds well
# under the go-unit-offline-v1 profile's 600s budget.
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"

# A `go` shim placed first on PATH: it passes every invocation through to
# the real `go` unchanged, except for `go build -o X ...`. There it first
# checks a shared binary cache (keyed by the build arguments with the -o
# path itself excluded, plus the current GOFLAGS) and, on a hit, copies
# the already-built binary into place instead of invoking the compiler at
# all -- a second, cheaper-than-GOCACHE layer of reuse across the ~20
# e2e scripts, every one of which builds the identical ./cmd/aurumcode
# package. On a miss it builds once, seeds the shared cache, then -- hit
# or miss -- moves the binary to X.aur541real and writes X as a wrapper
# that runs it, replays its stdout and stderr untouched to its own
# caller, and ALSO appends any scope-gate discard line to a fixed log path
# baked into the wrapper's text -- so the line survives even when the
# calling e2e script redirects the binary's stderr into a temp file it
# deletes before this program ever gets to look at it.
shim_dir="$run_dir/shimbin"
bin_share_dir="$run_dir/binshare"
mkdir -p "$shim_dir" "$bin_share_dir"
cat >"$shim_dir/go" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "build" ]]; then
  out=""
  prev=""
  key_args=()
  for a in "$@"; do
    if [[ "$prev" == "-o" ]]; then out="$a"; key_args+=("-o" "__OUT__"); prev="$a"; continue; fi
    key_args+=("$a")
    prev="$a"
  done
  key="$(printf '%s\0' "${key_args[@]}" "GOFLAGS=${GOFLAGS:-}" | sha256sum | awk '{print $1}')"
  cached="__BIN_SHARE__/$key"
  rc=0
  if [[ -n "$out" && -x "$cached" ]]; then
    cp -f -- "$cached" "$out"
    chmod +x -- "$out"
  else
    "__REAL_GO__" "$@"
    rc=$?
    if [[ $rc -eq 0 && -n "$out" && -x "$out" ]]; then
      cp -f -- "$out" "$cached.tmp" && mv -f -- "$cached.tmp" "$cached"
    fi
  fi
  if [[ $rc -eq 0 && -n "$out" && -x "$out" ]]; then
    mv -f -- "$out" "$out.aur541real"
    {
      printf '#!/usr/bin/env bash\n'
      printf 'errtmp="$(mktemp)"\n'
      printf '"%s" "$@" 2>"$errtmp"\n' "$out.aur541real"
      printf 'rc=$?\n'
      printf 'cat "$errtmp" >&2\n'
      printf "grep -E 'sem evidencia concreta|sem impacto explicado|sem verificacao proposta' \"\$errtmp\" >> '%s' || true\n" "__DISCARD_LOG__"
      printf 'rm -f "$errtmp"\n'
      printf 'exit "$rc"\n'
    } >"$out"
    chmod +x -- "$out"
  fi
  exit "$rc"
else
  exec "__REAL_GO__" "$@"
fi
SHIM
sed -i "s|__REAL_GO__|$real_go|g; s|__DISCARD_LOG__|$discard_log|g; s|__BIN_SHARE__|$bin_share_dir|g" "$shim_dir/go"
chmod +x "$shim_dir/go"

# Every tests/e2e/*.sh that feeds AURUMCODE_LLM_FIXTURE a real value (a
# path, not an unset/empty assignment): the scope gate can only discard a
# finding that fixture actually proposes.
fixture_scripts() {
  grep -lE 'AURUMCODE_LLM_FIXTURE=("\$[A-Za-z_0-9]+"|\$[A-Za-z_0-9]+|"\$\{[A-Za-z_0-9]+[:=-][^}]*\}")' \
    "$repo_root"/tests/e2e/*.sh 2>/dev/null \
    | xargs -n1 basename | sed 's/\.sh$//' | sort -u
}

declares_expected_discard() {
  grep -q 'AUR-541: expect-gate-discard' "$repo_root/tests/e2e/$1.sh" 2>/dev/null
}

# run_e2e runs one e2e script, from repo_root, with the shim first on
# PATH, discarding neither stdout nor stderr (both land in $1.out so a
# caller can inspect the failure tag on a RED run).
run_e2e() {
  local name="$1" out
  out="$run_dir/e2e-$name.out"
  set +e
  ( cd "$repo_root" && PATH="$shim_dir:$PATH" bash "tests/e2e/$name.sh" ) >"$out" 2>&1
  rc=$?
  set -e
  printf '%s\n' "$out"
  return 0
}

# AC-001: no fixture-bearing e2e script may report an undeclared
# scope-gate discard.
run_ac001() {
  local any_bad=0 name out
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    : >"$discard_log"
    out="$(run_e2e "$name")"
    rc=$?
    if [[ "$rc" -eq 79 || "$rc" -eq 69 ]]; then
      printf '%s/%s/infra-skip/%s:%s\n' "$card" "$selector" "$name" "$rc" >&2
      continue
    fi
    if [[ -s "$discard_log" ]]; then
      if declares_expected_discard "$name"; then
        continue
      fi
      cat "$discard_log" >&2
      cat "$out" >&2
      printf '%s/%s/undeclared-discard:%s\n' "$card" "$selector" "$name" >&2
      any_bad=1
    fi
  done < <(fixture_scripts)
  [[ "$any_bad" -eq 0 ]]
}

# AC-002: the scripts this card corrected still exit 0 -- the measured
# "continues to prove what it promised" half of the outcome. AUR-438 and
# AUR-448 are deliberately excluded: both still go RED, but for reasons
# this card's non-goals forbid touching (AUR-438's general-comment finding
# is discarded by the scope gate's OutsideAddedLines rule, not by missing
# evidence; AUR-448's no-provider scenario fails before it ever reaches
# its gate-relevant fixtures). See docs/specs/AUR-541.md.
readonly -a corrected_scripts=(
  AUR-431 AUR-432 AUR-433 AUR-434 AUR-435 AUR-436 AUR-441 AUR-461 AUR-475
)

run_ac002() {
  local any_bad=0 name out rc
  for name in "${corrected_scripts[@]}"; do
    out="$(run_e2e "$name")"
    rc=$?
    if [[ "$rc" -ne 0 ]]; then
      cat "$out" >&2
      printf '%s/%s/corrected-script-not-green:%s:%s\n' "$card" "$selector" "$name" "$rc" >&2
      any_bad=1
    fi
  done
  [[ "$any_bad" -eq 0 ]]
}

# AC-001-MUT-001: stage a copy of tests/e2e/AUR-461.sh (a corrected script
# with no "expect-gate-discard" comment of its own) and strip this card's
# evidence/impact/verification fields back out of its catalog-id.json
# fixture, reproducing the pre-fix shape. AC-001's detector must then
# report that script as an undeclared discard.
run_mut001() {
  local root="$run_dir/root-mut001"
  mkdir -p "$root"
  local top
  for top in go.mod go.sum cmd internal pkg tests; do
    [[ -e "$repo_root/$top" ]] || continue
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"

  local target="$root/tests/e2e/AUR-461.sh"
  [[ -f "$target" ]] || infra 'MUT-001/stage-missing'
  local before after
  before="$(sha256sum "$target" | awk '{print $1}')"

  local tmp="$root/AUR-461.mutated.sh"
  sed \
    -e '/"evidence": "linha 4 concatena/d' \
    -e '/"impact": "um valor hostil na variavel vira comando arbitrario no shell",$/d' \
    -e '/"verification": "reexecutar com um valor contendo ; e confirmar/d' \
    -e '0,/catalog-id.json/{s/"suggestion": "pass an argument vector",$/"suggestion": "pass an argument vector"/}' \
    "$target" >"$tmp" || infra 'MUT-001/rewrite'
  mv "$tmp" "$target"

  after="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$before" != "$after" ]] || infra 'MUT-001/no-change'

  # Run the mutated copy through the same shim + detector this program's
  # AC-001 uses, scoped to just this one script.
  : >"$discard_log"
  local out="$root/mut001.out"
  set +e
  ( cd "$root" && PATH="$shim_dir:$PATH" bash tests/e2e/AUR-461.sh ) >"$out" 2>&1
  local rc=$?
  set -e

  if [[ "$rc" -eq 79 || "$rc" -eq 69 ]]; then cat "$out" >&2; infra 'MUT-001/e2e-infra'; fi
  if [[ ! -s "$discard_log" ]]; then
    cat "$out" >&2
    return 1 # mutation survived: stripping evidence should have revived the discard
  fi
  grep -q 'AUR-541: expect-gate-discard' "$target" && {
    cat "$out" >&2
    infra 'MUT-001/mutated-script-declares-discard'
  }
  return 0
}

case "$selector" in
  AC-001)
    run_ac001 || fail AC-001
    ;;
  AC-002)
    run_ac002 || fail AC-002
    ;;
  AC-001-MUT-001)
    run_mut001 || fail AC-001-MUT-001
    ;;
  all)
    run_ac001 || fail AC-001
    run_ac002 || fail AC-002
    run_mut001 || fail AC-001-MUT-001
    ;;
esac

printf '%s/%s/pass\n' "$card" "$selector"
exit 0
