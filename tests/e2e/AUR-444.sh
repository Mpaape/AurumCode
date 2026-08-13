#!/usr/bin/env bash
# AUR-444 E2E: run the REAL scripts/action-entrypoint.sh, as a process, and
# prove its control flow end to end -- not just that its text looks right.
#
# WHY A STAND-IN BINARY IS STILL USED FOR SCENARIOS 1-7, EVEN THOUGH THE REAL
# BINARY IS NOW BUILDABLE HERE
#   AUR-444's read_paths now carries cmd/aurumcode's full compile closure
#   (internal/analyzer, internal/prompt, internal/review, internal/llm,
#   internal/security/redaction, internal/git/githubclient, pkg/types, and
#   the internal/documentation/... + internal/pipeline packages the docs
#   subcommand reuses), so `go build ./cmd/aurumcode` succeeds inside the
#   sealed `bootstrap-readonly-v1` acceptance container -- this was NOT true
#   of an earlier revision of this card, whose read_paths listed only
#   cmd/aurumcode itself; a plain `go build` failed there on "finding module
#   for package .../internal/analyzer" et al., confirmed empirically before
#   read_paths was corrected.
#
#   Scenarios 1-7 below still use a tiny stand-in script (recording exactly
#   the argv it was invoked with, exiting 0) rather than the real binary, on
#   purpose: they test the ENTRYPOINT'S OWN control flow -- require_binary,
#   the AURUMCODE_BASE_REF gate, the exact argv it builds, GITHUB_OUTPUT
#   writing, determinism, the qa regression fix -- which is a property of
#   the shell script, not of cmd/aurumcode's flag parsing. Conflating the two
#   under one binary would leave a real review's provider/git/network
#   behavior able to mask a shell-script regression, or vice versa. Section 8
#   below, and tests/integration/AUR-444.go, use the REAL binary to prove the
#   other half: that every flag the entrypoint sends is one cmd/aurumcode
#   actually registers -- now REQUIRED, not best-effort, exactly because the
#   closure is now a materialized precondition rather than an occasional
#   convenience.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md): 0 pass, 1 behavioral
# RED, 64 unknown selector, 79 inconclusive/infrastructure.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR444}" == E2EAUR444 ]] || { printf 'AUR-444/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-444/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-444/AC-001/infrastructure/%s\n' "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
entrypoint="$repo_root/scripts/action-entrypoint.sh"
[[ -f "$entrypoint" ]] || infra entrypoint_missing
command -v bash >/dev/null 2>&1 || infra missing_bash

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e444.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir"' EXIT INT TERM HUP

workspace="$run_dir/workspace"
mkdir -p "$workspace"

# Unique per run so a coincidental substring match is not mistaken for a
# leak, and unique enough that it collides with nothing this script itself
# prints.
canary="AURUM_SECRET_CANARY_$(date +%s)_$$_e444"

fake_bin="$run_dir/fake-aurumcode"
args_capture="$run_dir/fake-args.txt"
cat >"$fake_bin" <<FAKEEOF
#!/bin/sh
: >"$args_capture"
for a in "\$@"; do printf '%s\n' "\$a" >>"$args_capture"; done
exit 0
FAKEEOF
chmod +x "$fake_bin"

# run_entrypoint <logfile> VAR=val...  -- runs the real entrypoint with the
# given env additions (plus the canary, always), captures combined
# stdout+stderr to logfile, and prints the raw exit code. AURUM_SECRET_CANARY
# is exported on every call, unconditionally, so its absence from every log
# this script writes is a property of ALL of them, not just one path.
run_entrypoint() {
  local logfile="$1"; shift
  local rc
  set +e
  env "$@" AURUM_SECRET_CANARY="$canary" bash "$entrypoint" >"$logfile" 2>&1
  rc=$?
  set -e
  printf '%s' "$rc"
}

# --- 1. No mode at all: usage error, exit 64. ---
log="$run_dir/log-nomode"
rc="$(run_entrypoint "$log" AURUMCODE_MODE='' GITHUB_WORKSPACE="$workspace")"
[[ "$rc" == 64 ]] || fail "nomode-wrong-exit:$rc"
grep -Fq 'AURUMCODE_MODE is not set' "$log" || fail 'nomode-wrong-message'

# --- 2. review mode without AURUMCODE_BASE_REF: usage error, exit 64. The
# binary must exist (fake_bin) so require_binary passes and execution
# actually reaches the AURUMCODE_BASE_REF gate this card added. ---
log="$run_dir/log-nobase"
rc="$(run_entrypoint "$log" AURUMCODE_MODE=review GITHUB_WORKSPACE="$workspace" AURUMCODE_CLI="$fake_bin")"
[[ "$rc" == 64 ]] || fail "nobase-wrong-exit:$rc"
grep -Fq 'AURUMCODE_BASE_REF' "$log" || fail 'nobase-wrong-message'
[[ ! -f "$args_capture" ]] || fail 'nobase-invoked-binary-anyway'

# --- 3. review mode, binary absent: environment failure, exit 3, and the
# message must name the real source (cmd/aurumcode) and must NOT carry the
# card's defect-2 false claim. ---
log="$run_dir/log-nobinary"
rc="$(run_entrypoint "$log" AURUMCODE_MODE=review AURUMCODE_BASE_REF=deadbeef GITHUB_WORKSPACE="$workspace" AURUMCODE_CLI="$run_dir/does-not-exist")"
[[ "$rc" == 3 ]] || fail "nobinary-wrong-exit:$rc"
if grep -Fq 'there is no cmd/cli in the source tree' "$log"; then
  fail 'nobinary-false-claim-survives'
fi
grep -Fq 'cmd/aurumcode' "$log" || fail 'nobinary-message-missing-real-source'

# --- 4. GREEN: the full review path, driven by the stand-in binary. Proves
# the entrypoint really invokes the binary with EXACTLY the flags this card
# fixed -- "review" and "--base=<ref>", nothing else, no --provider/--model/
# --api-key/--github-token/--post-comments. ---
log1="$run_dir/log-green1"
out1="$run_dir/gh-output-1"
: >"$out1"
rc="$(run_entrypoint "$log1" AURUMCODE_MODE=review AURUMCODE_BASE_REF=deadbeef GITHUB_WORKSPACE="$workspace" AURUMCODE_CLI="$fake_bin" GITHUB_OUTPUT="$out1")"
[[ "$rc" == 0 ]] || fail "green-wrong-exit:$rc"
grep -Fq 'review_result=passed' "$out1" || fail 'green-missing-output'
[[ -f "$args_capture" ]] || fail 'green-fake-not-invoked'
mapfile -t got_args <"$args_capture"
[[ "${#got_args[@]}" -eq 2 ]] || fail "green-wrong-argc:${#got_args[*]}"
[[ "${got_args[0]}" == "review" ]] || fail "green-wrong-arg0:${got_args[0]}"
[[ "${got_args[1]}" == "--base=deadbeef" ]] || fail "green-wrong-arg1:${got_args[1]}"

# --- 5. Determinism: repeating the exact same input reproduces the exact
# same output (AC-001's repeat clause). ---
log2="$run_dir/log-green2"
out2="$run_dir/gh-output-2"
: >"$out2"
rc2="$(run_entrypoint "$log2" AURUMCODE_MODE=review AURUMCODE_BASE_REF=deadbeef GITHUB_WORKSPACE="$workspace" AURUMCODE_CLI="$fake_bin" GITHUB_OUTPUT="$out2")"
[[ "$rc2" == 0 ]] || fail "repeat-wrong-exit:$rc2"
diff "$log1" "$log2" >/dev/null || fail 'repeat-output-differs'
diff "$out1" "$out2" >/dev/null || fail 'repeat-github-output-differs'

# --- 6. qa mode: the regression this fix must not introduce. Before this
# card, once AURUMCODE_CLI resolved to an existing binary, run_qa's
# unguarded invocation would hit cmd/aurumcode's "unknown command" usage
# exit (2) and kill the script under `set -e` -- a code this file's own
# EXIT CODES table never documents. Proven here with the SAME existing
# fake_bin (which "exists", so require_binary alone can no longer be what
# stops this): the fix must fail closed at exit 3, one of this file's own
# documented codes, before ever invoking the binary. ---
log="$run_dir/log-qa"
rc="$(run_entrypoint "$log" AURUMCODE_MODE=qa GITHUB_WORKSPACE="$workspace" AURUMCODE_CLI="$fake_bin")"
[[ "$rc" == 3 ]] || fail "qa-regression:wrong-exit:$rc"
grep -Fq 'does not publish' "$log" || fail 'qa-wrong-message'

# --- 7. Security: the canary handed to every run above never appears in any
# log or GITHUB_OUTPUT file this script wrote. ---
for f in "$run_dir"/log-* "$out1" "$out2"; do
  [[ -f "$f" ]] || continue
  if grep -Fq "$canary" "$f"; then
    fail "canary-leaked:$f"
  fi
done

# --- 8. REQUIRED, against the REAL binary: AUR-444's read_paths now carries
# cmd/aurumcode's full compile closure (internal/analyzer, internal/prompt,
# internal/review, internal/llm, internal/security/redaction,
# internal/git/githubclient, pkg/types, and the internal/documentation/...
# + internal/pipeline packages the docs subcommand reuses), so `go build
# ./cmd/aurumcode` is expected to succeed here too, inside the sealed
# acceptance sandbox -- this is no longer a "when available" bonus. Its
# absence is reported as an infrastructure gap (this closure is a
# precondition this card's own read_paths declares, not something this
# script can repair), but the one behavioral assertion once it builds --
# that the real binary does not reject the entrypoint's flags as
# unrecognized, the exact failure mode this card fixes -- is a hard
# requirement.
command -v go >/dev/null 2>&1 || infra missing_go_for_real_binary
[[ -d "$repo_root/internal/analyzer" ]] || infra "closure_not_materialized:internal/analyzer"

real_bin="$run_dir/aurumcode-real"
build_log="$run_dir/build-real.log"
if ! ( cd "$repo_root" && ulimit -v 8388608 2>/dev/null
    GOFLAGS='-mod=mod -p=1' GOMEMLIMIT=2GiB GOMAXPROCS=1 GOTOOLCHAIN=local \
      go build -o "$real_bin" ./cmd/aurumcode ) >"$build_log" 2>&1; then
  cat "$build_log" >&2
  infra "real_build_failed"
fi

reallog="$run_dir/log-real"
realout="$run_dir/gh-output-real"
: >"$realout"
run_entrypoint "$reallog" AURUMCODE_MODE=review AURUMCODE_BASE_REF=HEAD \
  AURUMCODE_CLI="$real_bin" GITHUB_WORKSPACE="$repo_root" GITHUB_OUTPUT="$realout" >/dev/null
if grep -Eq 'flag provided but not defined|flag needs an argument' "$reallog"; then
  fail "real-binary-rejected-flag:$(tr '\n' ' ' <"$reallog")"
fi
printf 'AUR-444/AC-001/real-binary-accepted-flags\n'

printf 'AUR-444/AC-001/ok\n'
