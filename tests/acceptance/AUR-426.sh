#!/usr/bin/env bash
#
# Acceptance program for card AUR-426, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode docs [--source <dir>] [--output <dir>] [--languages <list>]`
#   generates project documentation through discoverable command-line flags
#   -- `aurumcode docs --help` documents every one -- instead of requiring
#   the operator to already know cmd/regenerate-docs's AURUMCODE_-prefixed
#   environment variables. It reuses internal/pipeline and
#   internal/documentation/extractors -- the exact engine cmd/regenerate-docs
#   already wires -- rather than reimplementing extraction; cmd/regenerate-docs
#   itself is untouched and keeps working exactly as published, and
#   `aurumcode review`'s published contract stays byte-for-byte identical.
#   See docs/specs/AUR-426.md.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure: an input this card does not own was
#        never materialized, a required tool is missing. Never valid red
#        evidence, never a pass.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-426'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR426|IntegrationAUR426|E2EAUR426|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Offline module cache holding gopkg.in/yaml.v3 (internal/pipeline's
# _config.yml exclude-list reader): resolved BEFORE HOME is redirected,
# since the default cache location derives from HOME.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a426.XXXXXX")" || infra mktemp
# The staged copies below preserve the read-only modes of the materialized
# input tree, so force write permission back before removing, and never let
# a residual removal error decide the exit code (see AUR-430.sh).
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 (single build/test process) for every go
# invocation in this file.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache"
export HOME="$run_dir/home"
export TMPDIR="$run_dir"

# run_go <dir> <go-args...> runs `go` inside dir with the memory ceiling and
# GOMEMLIMIT the card requires, isolated to a subshell so the ulimit does not
# leak into the rest of this script (e.g. the `cp -R` staging below).
run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" )
}

# copy materializes one repo path into a staged root. Every path it copies is
# either owned by this card (paths) or read by it (read_paths); a missing
# source is an input this card does not own that was never materialized -- an
# environment gap, never a verdict.
copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly this card's read-set: the two closures
# the "Nota de dispatch" names already proven -- documentation
# (internal/documentation/*, internal/pipeline, cmd/regenerate-docs,
# action.yml, the goproject fixture) and the engine cmd/aurumcode already
# carries (internal/analyzer, internal/prompt, internal/review, internal/llm,
# internal/security/redaction, pkg/types, the git-demo/review fixtures) --
# plus this card's own docs/specs/AUR-426.md, which its unit test reads.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" internal/pipeline
  copy "$root" internal/analyzer internal/prompt internal/review internal/llm internal/security/redaction pkg/types
  copy "$root" tests/fixtures/docs/goproject tests/fixtures/repos/git-demo tests/fixtures/review
  copy "$root" action.yml
  copy "$root" docs/specs/AUR-426.md
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# build_shared builds the binary once per acceptance run and reuses it for
# every lane; GOCACHE is shared process-wide, so the go test compiles and
# the mutation rebuild start warm instead of cold.
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! run_go "$shared_root" build -o "$shared_bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# nominal_case is AC-001's core behavioral proof, run directly against the
# real binary (not only through the Go test layers below): --help documents
# real, discoverable flags that agree with docs/specs/AUR-426.md; the
# command actually drives internal/pipeline to generate real documentation;
# the run is deterministic; invalid input and a zero-page run both fail
# loudly; a secret canary never reaches stdout or stderr; and
# `aurumcode review`'s published contract survives untouched.
nominal_case() {
  build_shared
  local rc

  # `aurumcode docs --help`: the declared command. Its promised result is
  # usage on stdout with the discoverable flags, exit 0. Before the
  # implementation exists this is exactly where RED fires (unknown command,
  # exit 2, nothing on stdout) -- never a compile or build failure, which
  # does not count as RED under this card's TDD proof.
  set +e
  "$shared_bin" docs --help >"$run_dir/help.stdout" 2>"$run_dir/help.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  [[ -s "$run_dir/help.stderr" ]] && fail behavior-missing
  grep -Fq 'usage: aurumcode docs' "$run_dir/help.stdout" || fail behavior-missing
  for flag in -source -output -languages; do
    grep -Fq -- "$flag" "$run_dir/help.stdout" || fail behavior-missing
  done

  # Cross-check the MUT-001 target directly at the acceptance level: the
  # flags --help documents must equal the flags docs/specs/AUR-426.md's
  # Command table documents. Both sides are read fresh from real artifacts
  # (the built binary's own output; the staged spec file), never a
  # hand-authored third list, so this cannot pass tautologically.
  local help_flags spec_flags
  help_flags="$(grep -oE '^  -[a-zA-Z][a-zA-Z0-9_-]*' "$run_dir/help.stdout" | sed 's/^  -//' | sort -u | tr '\n' ',')"
  spec_flags="$(grep -oE '^\| `--[a-zA-Z][a-zA-Z0-9_-]*' "$shared_root/docs/specs/AUR-426.md" | sed 's/^| `--//' | sort -u | tr '\n' ',')"
  [[ -n "$help_flags" ]] || fail help_flags_unparseable
  [[ -n "$spec_flags" ]] || fail spec_flags_unparseable
  [[ "$help_flags" == "$spec_flags" ]] || fail "flags_mismatch:help=$help_flags:spec=$spec_flags"

  # Real generation against the AUR-424 fixture project: proves this reuses
  # internal/pipeline (real symbols in a real generated page), not a stub
  # that only parses flags.
  local out_dir="$run_dir/site"
  set +e
  "$shared_bin" docs --source "$shared_root/tests/fixtures/docs/goproject" --output "$out_dir" \
    >"$run_dir/gen.stdout" 2>"$run_dir/gen.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail generation-failed
  [[ -f "$out_dir/go/root.md" ]] || fail missing-generated-page
  grep -Fq 'Greeting' "$out_dir/go/root.md" || fail generated-page-missing-symbol
  local first_stdout
  first_stdout="$(cat "$run_dir/gen.stdout")"

  # Determinism: repeating the run over the same input produces the same
  # output (AC-001's "repetir a execucao sobre a mesma entrada produz a
  # mesma saida").
  set +e
  "$shared_bin" docs --source "$shared_root/tests/fixtures/docs/goproject" --output "$out_dir" \
    >"$run_dir/gen2.stdout" 2>/dev/null
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  [[ "$(cat "$run_dir/gen2.stdout")" == "$first_stdout" ]] || fail non-deterministic

  # A --source that does not exist fails clearly, exit 1.
  set +e
  "$shared_bin" docs --source "$run_dir/does-not-exist" --output "$run_dir/site2" \
    >/dev/null 2>"$run_dir/badsrc.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail bad-source-wrong-exit
  grep -Fq 'source directory' "$run_dir/badsrc.stderr" || fail bad-source-message-missing

  # A --languages filter matching nothing in the fixture documents zero
  # pages: exit 1, never a silent green run with nothing on disk.
  set +e
  "$shared_bin" docs --source "$shared_root/tests/fixtures/docs/goproject" --output "$run_dir/site3" \
    --languages nonexistent-language >/dev/null 2>"$run_dir/empty.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail empty-run-wrong-exit
  grep -Fq 'no documentation page was generated' "$run_dir/empty.stderr" || fail empty-run-message-missing

  # A secret canary must never reach stdout or stderr (AUR-009/AUR-432),
  # even when embedded in a path this command itself prints (--output).
  local canary='aur426-nominal-canary-7f3c1a'
  set +e
  AURUM_SECRET_CANARY="$canary" "$shared_bin" docs \
    --source "$shared_root/tests/fixtures/docs/goproject" --output "$run_dir/${canary}-site" \
    >"$run_dir/canary.stdout" 2>"$run_dir/canary.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail canary-run-failed
  grep -Fq "$canary" "$run_dir/canary.stdout" && fail canary-leaked-stdout
  grep -Fq "$canary" "$run_dir/canary.stderr" && fail canary-leaked-stderr

  # aurumcode review's published contract (AUR-430/431/435/436) is
  # byte-for-byte unchanged by this card's addition.
  local review_fixture="$run_dir/response-clean.json"
  printf '{"issues":[],"summary":"Nothing to report."}' >"$review_fixture"
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$review_fixture" "$shared_bin" review --base HEAD~1) \
    >"$run_dir/review.stdout" 2>"$run_dir/review.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail review-contract-broken
  [[ "$(cat "$run_dir/review.stdout")" == "No issues found." ]] || fail review-contract-broken
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-426.go
  cat >"$root/tests/unit/aur426_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR426UnitBridge(t *testing.T) { TestAUR426(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/unit -run '^TestAUR426UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR426:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR426:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-426.go
  cat >"$root/tests/integration/aur426_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR426IntegrationBridge(t *testing.T) { IntegrationAUR426(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/integration -run '^TestAUR426IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR426:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR426:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-426.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-build its own copy. The
  # nested script's own exit-code vocabulary is preserved: its 79 is an
  # environment gap and must be re-emitted as infra here, never collapsed
  # into behavioral RED (see EXIT_CODE_CONVENTION.md).
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-426.sh E2EAUR426)
  rc=$?
  set -e
  ((rc != 79)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# regression_case proves the non-negotiable "cmd/regenerate-docs intocado e
# continuando a funcionar": its own pre-existing test suite -- unmodified by
# this card, staged from the same read-set that lets it build -- still
# passes.
regression_case() {
  local root="$run_dir/root-regression"
  stage_source "$root"
  local out rc
  set +e
  out="$(run_go "$root" test -timeout 300s ./cmd/regenerate-docs -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | tail -n 20
  ((rc == 0)) || fail "regression:exit:$rc"
  grep -q '^ok ' <<<"$out" || fail regression-zero-packages
  cleanup_root "$root"
}

# mutate_line/delete_line locate an EXACT, whole-line, fixed-string anchor
# (grep -Fx) and act on it by line number, so the replacement text never has
# to be regex/sed-escaped (the anchors below carry parentheses, commas and an
# asterisk). Both fail (return 1) if the anchor is absent or not unique.
mutate_line() {
  local file="$1" anchor="$2" replacement="$3" n
  n="$(grep -Fxn "$anchor" "$file" | cut -d: -f1)"
  [[ -n "$n" ]] || return 1
  [[ "$(wc -l <<<"$n")" -eq 1 ]] || return 1
  sed -i "${n}s#.*#${replacement}#" "$file"
}
delete_line() {
  local file="$1" anchor="$2" n
  n="$(grep -Fxn "$anchor" "$file" | cut -d: -f1)"
  [[ -n "$n" ]] || return 1
  [[ "$(wc -l <<<"$n")" -eq 1 ]] || return 1
  sed -i "${n}d" "$file"
}

# mutation_case is MUT-001: remove a flag documented in --help (the
# --languages registration in cmd/aurumcode/docs.go) and prove the same
# accept can never pass with the defect in place. The mutant must still
# compile -- a compile break is explicitly disqualified as RED/mutation
# evidence by this card's TDD proof -- so the flag's one use site is
# rewritten to a literal `nil` rather than left dangling. The committed
# source is never touched (only a scratch copy), so restoration is by
# construction: shared_bin still documents --languages afterward.
mutation_case() {
  build_shared

  local root="$run_dir/root-mut"
  stage_source "$root"
  local target="$root/cmd/aurumcode/docs.go"

  local anchor_decl=$'\tlanguages := fs.String("languages", "", "comma-separated language filter, exact lowercase names, e.g. go,python (default: all supported languages; an unrecognized or wrongly-cased name matches nothing)")'
  local anchor_use=$'\t\tLanguages:       splitDocsLanguages(*languages),'

  mutate_line "$target" "$anchor_use" $'\t\tLanguages:       nil,' || fail 'MUT-001/anchor-use-not-unique'
  delete_line "$target" "$anchor_decl" || fail 'MUT-001/anchor-decl-not-unique'
  grep -Fq "$anchor_decl" "$target" && fail 'MUT-001/mutation-not-applied'
  grep -Fxq $'\t\tLanguages:       nil,' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  # Direct behavioral evidence: --help silently stopped documenting
  # --languages -- the exact defect MUT-001 names.
  local mutant_help
  mutant_help="$("$bin" docs --help)"
  if grep -Fq -- '-languages' <<<"$mutant_help"; then
    fail 'MUT-001/mutation-had-no-observable-effect'
  fi

  # The card's own declared Unit selector independently catches it: rebuilt
  # against the mutated tree, TestAUR426's help-vs-spec cross-check must
  # fail -- this is the same accept the nominal lanes above must never pass
  # under, now proven to actually reject the defect rather than merely
  # asserting it should.
  copy "$root" tests/unit/AUR-426.go
  cat >"$root/tests/unit/aur426_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR426UnitBridge(t *testing.T) { TestAUR426(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -timeout 300s ./tests/unit -run '^TestAUR426UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail 'MUT-001/not-rejected'
  # Anchored on the failing SUBTEST NAME rather than its message text: this
  # survives a future rewording of the t.Fatalf string in
  # tests/unit/AUR-426.go without needing a matching edit here, while still
  # proving the mutation was caught by the specific check built for it
  # (not some unrelated assertion failing for an unrelated reason).
  grep -Fq -- '--- FAIL: TestAUR426UnitBridge/help_flags_match_the_documented_spec_table' <<<"$out" \
    || fail 'MUT-001/wrong-failure-reason'

  # Restoration: the unmutated shared binary still documents --languages --
  # the GREEN reproduces exactly.
  local restored
  restored="$("$shared_bin" docs --help)"
  grep -Fq -- '-languages' <<<"$restored" || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  regression_case
  mutation_case
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR426) unit_case ;;
  IntegrationAUR426) integration_case ;;
  E2EAUR426) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
