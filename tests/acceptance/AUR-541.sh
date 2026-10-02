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
#   reports an undeclared scope/evidence-gate discard.
#
# No per-script exemption mechanism exists any more: every fixture this
# card corrected (including the ones mixed with deliberately-ungrounded
# entries in AUR-434.sh and AUR-448.sh) now carries evidence/impact/
# verification on every entry, so none of them can legitimately trip the
# scope/evidence gate at all -- any occurrence is an unconditional defect.
# tests/e2e/AUR-459.sh is excluded from the scan below: its fixture uses
# the `line_comments` wire format, whose `lineComment` struct (internal/
# prompt/parser.go) carries no evidence/impact/verification fields at
# all, so a converted finding can never pass the gate without a parser
# change this card's non-goals forbid; the script is also permanently
# infra-blocked in any clean checkout (it requires cmd/regenerate-docs,
# which git no longer tracks -- `git ls-files cmd/regenerate-docs` is
# empty on main). That gap is tracked by a separate card (AUR-542), not
# this one's AC-001.
#
# SELECTORS
#   all             run every scenario below
#   AC-001          every e2e script that sets AURUMCODE_LLM_FIXTURE to a
#                   real value (except AUR-459, see above) is run once;
#                   any scope-gate discard line on the binary's stderr is
#                   a failure, and any script that itself returns
#                   infrastructure (69/79) makes this selector inconclusive
#                   (79) rather than silently continuing
#   AC-002          the scripts this card corrected (tests/e2e/AUR-431.sh,
#                   AUR-432.sh, AUR-433.sh, AUR-434.sh, AUR-435.sh,
#                   AUR-436.sh, AUR-441.sh, AUR-461.sh, AUR-475.sh) each
#                   still exit 0 -- the measured "continues to prove what
#                   it promised" half of the card's outcome
#   AC-001-MUT-001  a staged copy of tests/e2e/AUR-461.sh with the
#                   evidence/impact/verification fields stripped back out
#                   of its catalog-id.json fixture must make AC-001's
#                   detector report a discard
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
invoked_log="$run_dir/invoked.log"
: >"$discard_log"
: >"$invoked_log"

# Every e2e script this program drives falls back to its OWN per-run
# GOCACHE/GOTMPDIR only when neither is already set (`: "${GOCACHE:=...}"`).
# Exporting a single shared pair here, before any script runs, means every
# `go build` after the first one for the same source is a cache hit instead
# of a cold recompile -- this is what keeps ~20 sequential e2e builds well
# under the go-unit-offline-v1 profile's 600s budget.
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"

# A `go` shim placed first on PATH: it passes every invocation through to
# the real `go` unchanged, EXCEPT `go build -o X ./cmd/aurumcode` -- an
# exact package match, checked against the literal last argument, not
# just "any -o". A prior version wrapped every `go build -o`, including
# the throwaway fakegithub loopback servers tests/e2e/AUR-438.sh,
# AUR-439.sh and AUR-451.sh each build and then kill by PID: those
# scripts captured the WRAPPER's pid and killed it, orphaning the real
# server child underneath (wrapped in a plain foreground `"$bin" "$@"`,
# nothing reaped it), leaking one process per AC-001 run. 11 scripts x
# repeated runs pushed the shared container to 1006/1024 PIDs, `go build`
# itself started failing with "resource temporarily unavailable", and
# the resulting build_failed exit (1, zero discard-log content) was
# silently read as "clean" by the old AC-001. Narrowing the match removes
# the leak at its source for every binary this program does not need to
# wrap; the wrapper below is ALSO made exec-safe regardless, in case any
# future script kills it directly: the real binary runs backgrounded, a
# trap kills and reaps it on TERM/INT/EXIT, and `wait` propagates its real
# exit code rather than racing it.
#
# The wrapper also appends one line to a fixed "invoked" log every time it
# actually runs the real binary -- AC-001 below uses this to tell "ran
# clean" apart from "never got far enough to run at all" (a build or
# environment failure), which is the other half of the defect above: a
# script that fails without ever invoking the binary has zero discard-log
# content for a reason that proves nothing about the gate.
shim_dir="$run_dir/shimbin"
mkdir -p "$shim_dir"
cat >"$shim_dir/go" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "build" ]]; then
  out=""
  prev=""
  last=""
  for a in "$@"; do
    if [[ "$prev" == "-o" ]]; then out="$a"; fi
    prev="$a"
    last="$a"
  done
  "__REAL_GO__" "$@"
  rc=$?
  if [[ $rc -eq 0 && -n "$out" && -x "$out" && "$last" == "./cmd/aurumcode" ]]; then
    mv -f -- "$out" "$out.aur541real"
    cat >"$out" <<'WRAP'
#!/usr/bin/env bash
printf '1\n' >>'__INVOKED_LOG__' 2>/dev/null || true
errtmp="$(mktemp)"
"$0.aur541real" "$@" 2>"$errtmp" &
child=$!
trap 'kill "$child" 2>/dev/null; wait "$child" 2>/dev/null' TERM INT EXIT
wait "$child"
rc=$?
trap - TERM INT EXIT
cat "$errtmp" >&2
grep -E 'sem evidencia concreta|sem impacto explicado|sem verificacao proposta' "$errtmp" >>'__DISCARD_LOG__' || true
rm -f "$errtmp"
exit "$rc"
WRAP
    chmod +x -- "$out"
  fi
  exit "$rc"
else
  exec "__REAL_GO__" "$@"
fi
SHIM
sed -i "s|__REAL_GO__|$real_go|g; s|__DISCARD_LOG__|$discard_log|g; s|__INVOKED_LOG__|$invoked_log|g" "$shim_dir/go"
chmod +x "$shim_dir/go"

# Every tests/e2e/*.sh that feeds AURUMCODE_LLM_FIXTURE a real value (a
# path, not an unset/empty assignment) -- except AUR-459, see the header
# comment above: the scope gate can only discard a finding that fixture
# actually proposes.
fixture_scripts() {
  grep -lE 'AURUMCODE_LLM_FIXTURE=("\$[A-Za-z_0-9]+"|\$[A-Za-z_0-9]+|"\$\{[A-Za-z_0-9]+[:=-][^}]*\}")' \
    "$repo_root"/tests/e2e/*.sh 2>/dev/null \
    | xargs -n1 basename | sed 's/\.sh$//' | sort -u | grep -Fvx 'AUR-459'
}

# run_e2e runs one e2e script, from repo_root, with the shim first on
# PATH. It sets globals e2e_rc and e2e_out rather than returning a status
# through a `$(...)` command substitution: a function's own `rc=$?`
# executes inside the subshell command substitution forks for its output,
# so it never reaches the caller -- the exit status every caller used to
# read back was always 0, the command substitution's own success,
# regardless of what the e2e script actually did (AUR-541 review,
# 97c4d39, B1). Call this directly, never as `x="$(run_e2e ...)"`.
run_e2e() {
  local name="$1"
  e2e_out="$run_dir/e2e-$name.out"
  set +e
  ( cd "$repo_root" && PATH="$shim_dir:$PATH" bash "tests/e2e/$name.sh" ) >"$e2e_out" 2>&1
  e2e_rc=$?
  set -e
}

# Scripts already red today for reasons this card's non-goals forbid
# touching, tracked by card AUR-542, NOT by this selector (measured this
# session, re-verify by running each one directly if this list is ever
# in doubt):
#   AUR-438  the "general" (non-inline) finding is removed by the scope
#            gate's OutsideAddedLines rule, not by missing evidence --
#            fixing it means changing internal/review/scope.go.
#   AUR-443  fails at help_missing_docs, unrelated to any model fixture.
#   AUR-448  the "no provider configured" scenario returns exit 0 instead
#            of the expected 1, before the script ever reaches its
#            gate-relevant mixed/all-discarded fixtures.
#   AUR-449  the same no-provider defect as AUR-448.
#   AUR-451  fails at seguranca_only_wrong_post_count; not confirmed
#            related to the gate either way.
readonly -a known_red_scripts=(AUR-438 AUR-443 AUR-448 AUR-449 AUR-451)

is_known_red() {
  local n
  for n in "${known_red_scripts[@]}"; do [[ "$n" == "$1" ]] && return 0; done
  return 1
}

# AC-001: no fixture-bearing e2e script may report a scope-gate discard.
# A script that itself cannot run (infra, 69/79) makes the WHOLE check
# inconclusive rather than being silently skipped -- an unresolved
# "can we even tell" is not the same claim as "no discard happened".
# Likewise an empty discard log proves nothing if the binary was never
# actually invoked (invoked_log): a build or environment failure that
# exits non-zero before ever running `aurumcode` must not be read as "ran
# clean" just because there is nothing in the discard log either.
#
# The discard check runs FIRST, for every script, unconditionally --
# before the known-red exemption or any other exit-code branch gets a
# chance to `continue`. A known-red script (AUR-438 et al.) still stays
# red for its own, already-tracked, unrelated reason, but it can ALSO
# regress its own corrected fixture, and the known-red exemption must
# never excuse that: it only ever excuses the exit code, never a
# discard. (A prior version continued on known-red before reaching this
# check, so stripping evidence back out of AUR-438's corrected inline
# finding passed AC-001 silently -- fixed here.)
run_ac001() {
  local any_bad=0 name
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    : >"$discard_log"
    : >"$invoked_log"
    run_e2e "$name"
    if [[ "$e2e_rc" -eq 79 || "$e2e_rc" -eq 69 ]]; then
      cat "$e2e_out" >&2
      infra "e2e-infra:$name:$e2e_rc"
    fi

    if [[ -s "$discard_log" ]]; then
      cat "$discard_log" >&2
      cat "$e2e_out" >&2
      printf '%s/%s/undeclared-discard:%s\n' "$card" "$selector" "$name" >&2
      any_bad=1
    fi

    if [[ "$e2e_rc" -ne 0 ]]; then
      if is_known_red "$name"; then
        continue
      fi
      if [[ ! -s "$invoked_log" ]]; then
        cat "$e2e_out" >&2
        if grep -Fqi 'resource temporarily unavailable' "$e2e_out" || grep -Fq 'build_failed' "$e2e_out"; then
          infra "build-or-env-error:$name"
        fi
        printf '%s/%s/red-without-invoking-binary:%s\n' "$card" "$selector" "$name" >&2
        any_bad=1
        continue
      fi
      printf '%s/%s/unexpected-red-but-invoked:%s\n' "$card" "$selector" "$name" >&2
      any_bad=1
      continue
    fi

    if [[ ! -s "$invoked_log" ]]; then
      cat "$e2e_out" >&2
      printf '%s/%s/passed-without-invoking-binary:%s\n' "$card" "$selector" "$name" >&2
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
  local any_bad=0 name
  for name in "${corrected_scripts[@]}"; do
    run_e2e "$name"
    if [[ "$e2e_rc" -eq 79 || "$e2e_rc" -eq 69 ]]; then
      cat "$e2e_out" >&2
      infra "e2e-infra:$name:$e2e_rc"
    fi
    if [[ "$e2e_rc" -ne 0 ]]; then
      cat "$e2e_out" >&2
      printf '%s/%s/corrected-script-not-green:%s:%s\n' "$card" "$selector" "$name" "$e2e_rc" >&2
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

  # Delete this card's three added fields, then a general post-pass:
  # whenever a line ends in a comma and the NEXT line (ignoring leading
  # whitespace) is a closing brace, strip that now-dangling trailing
  # comma -- a fixed field-name substitution (what this program used to
  # do) only works if the deleted field happened to be last; here
  # "verification" genuinely was last, but the fix no longer depends on
  # that staying true.
  local tmp="$root/AUR-461.mutated.sh"
  sed \
    -e '/"evidence": "linha 4 concatena/d' \
    -e '/"impact": "um valor hostil na variavel vira comando arbitrario no shell",$/d' \
    -e '/"verification": "reexecutar com um valor contendo ; e confirmar/d' \
    "$target" \
    | awk '
      { lines[NR] = $0 }
      END {
        for (i = 1; i <= NR; i++) {
          line = lines[i]
          if (i < NR) {
            nxt = lines[i + 1]
            gsub(/^[ \t]+/, "", nxt)
            if (line ~ /,[ \t]*$/ && substr(nxt, 1, 1) == "}") {
              sub(/,[ \t]*$/, "", line)
            }
          }
          print line
        }
      }' >"$tmp" || infra 'MUT-001/rewrite'
  mv "$tmp" "$target"

  after="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$before" != "$after" ]] || infra 'MUT-001/no-change'

  # Run the mutated copy through the same shim + detector this program's
  # AC-001 uses, scoped to just this one script.
  : >"$discard_log"
  run_e2e_in() {
    local root_dir="$1" name="$2"
    e2e_out="$root_dir/mut001.out"
    set +e
    ( cd "$root_dir" && PATH="$shim_dir:$PATH" bash "tests/e2e/$name.sh" ) >"$e2e_out" 2>&1
    e2e_rc=$?
    set -e
  }
  run_e2e_in "$root" "AUR-461"

  if [[ "$e2e_rc" -eq 79 || "$e2e_rc" -eq 69 ]]; then cat "$e2e_out" >&2; infra 'MUT-001/e2e-infra'; fi
  if [[ ! -s "$discard_log" ]]; then
    cat "$e2e_out" >&2
    return 1 # mutation survived: stripping evidence should have revived the discard
  fi
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
