#!/usr/bin/env bash
# AUR-425 E2E: run the real generator over a project with no supported source
# file and prove the whole promised contract at the process boundary:
#
#   1. The run exits 1 (not 0, not something else) - the card's Outcome.
#   2. The output states the reason and names the scanned source directory.
#   3. Repeating the run over the same input produces the same output
#      (AC-001's repeat clause), modulo the log timestamp prefix and the
#      run-scoped temporary paths, which are normalized before comparison.
#   4. The AURUM_SECRET_CANARY value handed to the process never appears in
#      its output (the card's security clause).
#   5. A positive control: the same binary over a real Go project still
#      documents it and exits 0, so the fix is specific to the zero-page
#      outcome instead of failing every run.
#
# `aurumcode docs --source <dir>` from the card's AC-001 is realized here the
# same way every shipped consumer invokes this generator (action.yml,
# scripts/action-entrypoint.sh, tests/e2e/smoke_test.go): the
# cmd/regenerate-docs binary with AURUMCODE_SOURCE_DIR pointing at <dir>.
# No `aurumcode docs` subcommand exists in this repository (cmd/aurumcode
# publishes only `review`, owned by AUR-430), and cmd/aurumcode is outside
# this card's paths, so the binary IS the declared command's only real shape.
#
# Compiling the binary needs cmd/regenerate-docs's full local import closure
# (internal/documentation/..., internal/llm/..., internal/pipeline) plus the
# yaml.v3 module in an offline module cache. When those inputs are not
# materialized this exits 69: blocked/inconclusive per the card's Review
# clause, never a pass and never a behavioural failure.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR425}" == E2EAUR425 ]] || { printf 'AUR-425/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-425/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-425/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# The compile closure this binary needs; missing pieces are infrastructure.
for input in \
  go.mod go.sum \
  cmd/regenerate-docs/main.go \
  internal/pipeline \
  internal/documentation/extractors \
  internal/documentation/site \
  internal/llm
do
  [[ -e "$repo_root/$input" ]] || infra "closure_not_materialized:$input"
done

# Resolved BEFORE HOME is redirected: the default module cache derives from
# HOME, and this run is offline (GOPROXY=off), so yaml.v3 must already be
# cached.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e425.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp" "$run_dir/home" "$run_dir/emptypath" "$run_dir/work"

# A caller (the acceptance) may hand over a warm GOCACHE so the closure is
# compiled once per accept instead of once per lane; a bare run gets its own.
gocache="${GOCACHE:-$run_dir/cache}"
mkdir -p "$gocache"

binary="$run_dir/regenerate-docs"
build_out="$run_dir/build.log"
if ! ( ulimit -v 8388608
  cd "$repo_root" && \
  HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
  GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
  GOCACHE="$gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
  go build -o "$binary" ./cmd/regenerate-docs ) >"$build_out" 2>&1; then
  cat "$build_out" >&2
  infra build_failed
fi

# The card's fixture: a project whose every file is outside the supported
# language set (no extension detectLanguageFromFile maps). It is generated
# here because no fixture directory is declared in this card's paths.
src="$run_dir/projeto-sem-suporte"
mkdir -p "$src"
printf 'plain text, not a supported language\n' >"$src/notes.txt"
printf 'reStructuredText, not a supported language\n' >"$src/manual.rst"

readonly canary='AUR425-E2E-CANARY-b7f3'

# run_generator <output-dir> <log-file>; returns the raw exit code.
run_generator() {
  local out="$1" log="$2" rc=0
  ( cd "$run_dir/work" && \
    env -i \
      PATH="$run_dir/emptypath" \
      HOME="$run_dir/home" \
      AURUMCODE_SOURCE_DIR="$src" \
      AURUMCODE_OUTPUT_DIR="$out" \
      AURUM_SECRET_CANARY="$canary" \
      "$binary" ) >"$log" 2>&1 || rc=$?
  return "$rc"
}

rc1=0; run_generator "$run_dir/out1" "$run_dir/run1.log" || rc1=$?
rc2=0; run_generator "$run_dir/out2" "$run_dir/run2.log" || rc2=$?

if (( rc1 == 0 || rc2 == 0 )); then
  cat "$run_dir/run1.log" >&2
  printf 'AUR-425/AC-001/behavior-missing\n' >&2
  printf 'AUR-425/AC-001/MUT-001\n' >&2
  fail "zero-pages-exited-zero:rc1=$rc1:rc2=$rc2"
fi
(( rc1 == 1 && rc2 == 1 )) || { cat "$run_dir/run1.log" >&2; fail "wrong-exit:rc1=$rc1:rc2=$rc2"; }

# The stated reason, with the real directory named.
grep -Fq 'no documentation page was produced' "$run_dir/run1.log" \
  || { cat "$run_dir/run1.log" >&2; fail reason-missing:verdict; }
grep -Fq "no supported source file was documented under $src" "$run_dir/run1.log" \
  || { cat "$run_dir/run1.log" >&2; fail reason-missing:source-dir; }
grep -Fq 'aurumcode: result=empty' "$run_dir/run1.log" \
  || { cat "$run_dir/run1.log" >&2; fail summary-missing; }
! grep -Fq 'result=ok' "$run_dir/run1.log" || fail false-success-marker

# Security: the canary value must never surface in any run output.
! grep -Fq "$canary" "$run_dir/run1.log" || fail canary-leak:run1
! grep -Fq "$canary" "$run_dir/run2.log" || fail canary-leak:run2

# Determinism: identical input, identical output. The log timestamp prefix
# ("2026/08/13 10:11:12 main.go:383: ") and the per-run output directory are
# the only run-scoped bytes, so both are normalized away before diffing.
normalize() {
  sed -e 's|^[0-9][0-9/]* [0-9:]* ||' \
      -e "s|$run_dir/out1|OUTDIR|g" \
      -e "s|$run_dir/out2|OUTDIR|g" \
      "$1"
}
normalize "$run_dir/run1.log" >"$run_dir/run1.norm"
normalize "$run_dir/run2.log" >"$run_dir/run2.norm"
if ! diff -u "$run_dir/run1.norm" "$run_dir/run2.norm" >&2; then
  fail nondeterministic-output
fi

# Positive control: a documentable project must still document and exit 0.
ctl="$run_dir/projeto-go"
mkdir -p "$ctl"
printf 'module example.com/aur425ctl\n\ngo 1.21\n' >"$ctl/go.mod"
printf '// Package aur425ctl is the positive control.\npackage aur425ctl\n\n// Answer returns the documented answer.\nfunc Answer() int { return 42 }\n' \
  >"$ctl/doc.go"

ctl_rc=0
( cd "$run_dir/work" && \
  env -i \
    PATH="$run_dir/emptypath" \
    HOME="$run_dir/home" \
    AURUMCODE_SOURCE_DIR="$ctl" \
    AURUMCODE_OUTPUT_DIR="$run_dir/outctl" \
    AURUM_SECRET_CANARY="$canary" \
    "$binary" ) >"$run_dir/ctl.log" 2>&1 || ctl_rc=$?
if (( ctl_rc != 0 )); then
  cat "$run_dir/ctl.log" >&2
  fail "overshoot:documentable-project-exited-$ctl_rc"
fi
grep -Fq 'aurumcode: result=ok' "$run_dir/ctl.log" || { cat "$run_dir/ctl.log" >&2; fail overshoot:no-ok-verdict; }
control_page="$(find "$run_dir/outctl" -name '*.md' ! -name 'index.md' -type f | head -n 1)"
[[ -n "$control_page" ]] || { cat "$run_dir/ctl.log" >&2; fail overshoot:no-page-generated; }
! grep -Fq "$canary" "$run_dir/ctl.log" || fail canary-leak:control

printf 'AUR-425/AC-001/E2E: empty run exit=1 with stated reason, deterministic; control run exit=0 with %s\n' \
  "${control_page#"$run_dir/"}"
