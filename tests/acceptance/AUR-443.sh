#!/usr/bin/env bash
#
# Acceptance program for card AUR-443, scenario AC-001.
#
# WHAT THIS PROVES
#
#   A user who has never read the source code discovers what `aurumcode`
#   does and can run a first review: top-level `aurumcode --help` lists the
#   subcommands with a runnable example; `aurumcode --version` exists;
#   `review --help` and `docs --help` share one convention (an explicitly
#   requested --help is a fulfilled request: stdout, exit 0; a genuine
#   usage error is a refusal: stderr, exit 2); the provider-missing message
#   shows the FORM of a fixture file and points at a real, versioned
#   example; a repository that cannot be opened and a ref that does not
#   resolve each report one clean, actionable sentence instead of a
#   duplicated phrase or a leaked filesystem path. `review --base`'s
#   published findings output and `docs`'s published output are
#   byte-for-byte unchanged, including the deliberately-unprinted `summary`
#   field and the AUR-433 `--limite` exit code (see docs/specs/AUR-443.md's
#   "5." and "7." sections for why those two are investigated and left
#   unchanged rather than "fixed").
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

readonly card='AUR-443'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR443|IntegrationAUR443|E2EAUR443|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a443.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: the staged copies below
# preserve the read-only modes of the materialized input tree, so force
# write permission back before removing, and never let a residual removal
# error decide the exit code.
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 (single build/test process) for every go
# invocation in this file -- mirrors tests/acceptance/AUR-433.sh's own note:
# the shared cmd/aurumcode closure staged below (internal/documentation/*,
# internal/pipeline, cmd/regenerate-docs) is large enough that unbounded
# build parallelism gets the compiler OOM-killed under the sealed profile's
# memory ceiling.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

# run_go <dir> <go-args...> runs `go` inside dir with the memory ceiling and
# GOMEMLIMIT the card requires, isolated to a subshell so the ulimit does not
# leak into the rest of this script.
run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" )
}

# copy materializes one repo path into a staged root. Every path it copies
# is either owned by this card (paths) or read by it (read_paths); a
# missing source is an input this card does not own that was never
# materialized -- an environment gap, never a verdict.
copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes what `go build ./cmd/aurumcode` and every
# scenario below need: exactly this card's declared paths and read_paths
# (cmd/aurumcode is shared with concurrently-dispatched cards -- AUR-438
# added pr.go, importing internal/git/githubclient; AUR-426 added docs.go,
# importing internal/documentation/* and internal/pipeline -- both have to
# resolve for `go build ./cmd/aurumcode` to succeed against the integrated
# tree, mirroring tests/acceptance/AUR-433.sh's own stage_source note).
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/git
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" internal/pipeline
  copy "$root" internal/analyzer internal/prompt internal/review internal/security internal/llm pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review tests/fixtures/docs/goproject
  copy "$root" action.yml
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# build_shared builds the binary once per acceptance run and reuses it for
# every case; GOCACHE is shared process-wide, so the go test compiles and
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

# run_bin runs the built binary as a user would, from the given directory,
# with a clean provider/canary-less environment plus any extra KEY=VALUE
# pairs given anywhere among the remaining arguments (this card's own
# calls below put them last, after the subcommand and its flags, so the
# scan below is order-independent rather than leading-only). Raw exit code
# lands in the global rc without tripping errexit; stdout/stderr land in
# $run_dir/out.{stdout,stderr}.
run_bin() {
  local bin="$1" dir="$2"; shift 2
  local extra_env=() bin_args=() a
  for a in "$@"; do
    if [[ "$a" =~ ^[A-Za-z_][A-Za-z0-9_]*=.*$ ]]; then
      extra_env+=("$a")
    else
      bin_args+=("$a")
    fi
  done
  set +e
  (cd "$dir" && env -u AURUM_SECRET_CANARY -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL \
    "${extra_env[@]}" "$bin" "${bin_args[@]}") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# nominal_case is AC-001's core behavioral proof.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local known_problem_fixture="$shared_root/tests/fixtures/review/known-problem-response.json"
  [[ -f "$known_problem_fixture" ]] || infra missing_known_problem_fixture

  # --- 1. Top-level --help / -h / help. ---
  run_bin "$shared_bin" "$shared_root" --help
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  [[ -s "$run_dir/out.stderr" ]] && fail help-wrote-stderr
  grep -Fq 'review' "$run_dir/out.stdout" || fail help-missing-review
  grep -Fq 'docs' "$run_dir/out.stdout" || fail help-missing-docs
  grep -Fq 'aurumcode review --base HEAD~1' "$run_dir/out.stdout" || fail help-missing-example
  local help_stdout; help_stdout="$(cat "$run_dir/out.stdout")"

  for arg in -h help; do
    run_bin "$shared_bin" "$shared_root" "$arg"
    [[ "$rc" -eq 0 ]] || fail "${arg}-wrong-exit"
    [[ "$(cat "$run_dir/out.stdout")" == "$help_stdout" ]] || fail "${arg}-differs-from-help"
  done

  # A genuinely unknown token is still a usage error -- --help did not
  # silently widen "unknown command".
  run_bin "$shared_bin" "$shared_root" --totally-bogus-token
  [[ "$rc" -eq 2 ]] || fail unknown-command-wrong-exit
  grep -Fq 'unknown command' "$run_dir/out.stderr" || fail unknown-command-message-missing

  # --- 2. --version / version. ---
  for arg in --version version; do
    run_bin "$shared_bin" "$shared_root" "$arg"
    [[ "$rc" -eq 0 ]] || fail "${arg}-wrong-exit"
    grep -Fq 'aurumcode ' "$run_dir/out.stdout" || fail "${arg}-missing-prefix"
  done

  # --- 3. One --help convention: review and docs agree. ---
  run_bin "$shared_bin" "$shared_root" review --help
  [[ "$rc" -eq 0 ]] || fail review-help-wrong-exit
  [[ -s "$run_dir/out.stderr" ]] && fail review-help-wrote-stderr
  grep -Fq -- '-base' "$run_dir/out.stdout" || fail review-help-missing-flags

  run_bin "$shared_bin" "$shared_root" docs --help
  [[ "$rc" -eq 0 ]] || fail docs-help-wrong-exit
  [[ -s "$run_dir/out.stderr" ]] && fail docs-help-wrote-stderr
  grep -Fq 'usage: aurumcode docs' "$run_dir/out.stdout" || fail docs-help-missing-usage

  for sub in review docs; do
    run_bin "$shared_bin" "$shared_root" "$sub" --this-flag-does-not-exist
    [[ "$rc" -eq 2 ]] || fail "${sub}-usage-error-wrong-exit"
    [[ -s "$run_dir/out.stdout" ]] && fail "${sub}-usage-error-wrote-stdout"
  done

  # --- 4. No provider configured: message shows the fixture shape. ---
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1
  [[ "$rc" -eq 1 ]] || fail no-provider-wrong-exit
  grep -Fq 'no LLM provider configured' "$run_dir/out.stderr" || fail no-provider-message-missing
  grep -Fq 'tests/fixtures/review/known-problem-response.json' "$run_dir/out.stderr" || fail no-provider-missing-fixture-pointer
  grep -Fq '"severity"' "$run_dir/out.stderr" || fail no-provider-missing-fixture-shape

  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$known_problem_fixture"
  [[ "$rc" -eq 0 ]] || fail pointed-fixture-failed
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail pointed-fixture-missing-finding

  # --- 5. --limite's exit code: investigated, unchanged (docs/specs/AUR-443.md). ---
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 --limite 0.0001 "AURUMCODE_LLM_FIXTURE=$known_problem_fixture"
  [[ "$rc" -eq 1 ]] || fail limite-refusal-wrong-exit
  grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail limite-refusal-message-missing

  # --- 6. Git-repository and ref errors: one clean message, no leak. ---
  run_bin "$shared_bin" "$run_dir" review --base HEAD~1
  [[ "$rc" -eq 1 ]] || fail not-a-repo-wrong-exit
  grep -Fq 'is not a git repository' "$run_dir/out.stderr" || fail not-a-repo-message-missing
  [[ "$(grep -o 'not a git repository' "$run_dir/out.stderr" | wc -l)" -eq 1 ]] || fail not-a-repo-message-duplicated

  run_bin "$shared_bin" "$repo_dir" review --base does-not-exist-acceptance
  [[ "$rc" -eq 1 ]] || fail bad-ref-wrong-exit
  grep -Fq 'ref "does-not-exist-acceptance" not found' "$run_dir/out.stderr" || fail bad-ref-message-missing
  grep -Fq 'no such file or directory' "$run_dir/out.stderr" && fail bad-ref-leaked-filesystem-path
  grep -Fq 'refs/heads/' "$run_dir/out.stderr" && fail bad-ref-leaked-internal-path

  # A third leak shape, distinct from ENOENT and EACCES: a ref that
  # resolves to a DIRECTORY, not a file -- a hierarchical branch namespace
  # where only "feature/sub" exists and no literal "feature" ref does. The
  # pure-Go backend's os.ReadFile fails with EISDIR, which is neither
  # fs.ErrNotExist nor fs.ErrPermission, so review found it fell through
  # cleanRefError's classification into the raw internal chain. Built here
  # against the already-staged, already-writable git-demo copy (stage_source
  # already ran chmod -R u+w) rather than a shipped fixture, and removed
  # afterward so it does not affect any later scenario that also uses
  # repo_dir. The SAME two leak greps above are re-run against this case,
  # per the review's own instruction, so a regression here cannot pass
  # unnoticed.
  local hier_ref_dir="$repo_dir/refs/heads/feature"
  mkdir -p "$hier_ref_dir"
  cp "$repo_dir/refs/heads/main" "$hier_ref_dir/sub"
  run_bin "$shared_bin" "$repo_dir" review --base feature
  [[ "$rc" -eq 1 ]] || fail dir-ref-wrong-exit
  grep -Fq 'ref "feature" is not a branch' "$run_dir/out.stderr" || fail dir-ref-message-missing
  grep -Fq 'no such file or directory' "$run_dir/out.stderr" && fail dir-ref-leaked-filesystem-path
  grep -Fq 'refs/heads/' "$run_dir/out.stderr" && fail dir-ref-leaked-internal-path
  grep -Fq 'is a directory' "$run_dir/out.stderr" && fail dir-ref-leaked-raw-cause
  rm -rf "$hier_ref_dir"

  # --- 7. review --base's and docs's published contracts: byte-for-byte unchanged. ---
  local clean_fixture="$run_dir/response-clean.json"
  printf '{"issues":[],"summary":"Nothing to report."}' >"$clean_fixture"
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$clean_fixture"
  [[ "$rc" -eq 0 ]] || fail review-contract-broken
  [[ "$(cat "$run_dir/out.stdout")" == "No issues found." ]] || fail review-contract-broken

  local goproject="$shared_root/tests/fixtures/docs/goproject"
  run_bin "$shared_bin" "$shared_root" docs --source "$goproject" --output "$run_dir/site"
  [[ "$rc" -eq 0 ]] || fail docs-contract-broken
  grep -Fq 'Generated' "$run_dir/out.stdout" || fail docs-contract-broken

  # Determinism: repeating the declared command over the same input
  # produces the same output.
  run_bin "$shared_bin" "$shared_root" --help
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  [[ "$(cat "$run_dir/out.stdout")" == "$help_stdout" ]] || fail non-deterministic
}

# mutation_case is MUT-001: remove the top-level --help branch (the exact
# case arm this card added to run()) in a writable staged copy, rebuild,
# and prove the mutant flips the behavior nominal_case asserts:
# `aurumcode --help` regresses to "unknown command", exit 2 -- the
# discoverability defect this card's Outcome exists to fix. The committed
# source is never touched, so restoration is by construction: shared_bin
# still lists the subcommands GREEN.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild below recompiles only cmd/aurumcode.

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/main.go"
  local anchor='case "--help", "-h", "help":'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # Renaming the case labels removes the top-level help branch: those three
  # tokens now fall through to the `default:` arm ("unknown command", exit
  # 2) instead of printTopLevelHelp -- an unreferenced printTopLevelHelp is
  # still legal Go, so the mutant compiles.
  sed -i 's|case "--help", "-h", "help":|case "--help-DISABLED-BY-MUT-001", "-h-DISABLED", "help-DISABLED":|' "$target"
  grep -Fq 'help-DISABLED-BY-MUT-001' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  # Under the mutant, --help no longer lists the subcommands: it falls to
  # "unknown command", exit 2 -- the exact regression MUT-001 requires.
  run_bin "$bin" "$root" --help
  [[ "$rc" -ne 0 ]] || fail 'MUT-001/not-rejected'
  grep -Fq 'unknown command' "$run_dir/out.stderr" || fail 'MUT-001/not-rejected'
  grep -Fq 'review' "$run_dir/out.stdout" && fail 'MUT-001/subcommands-still-listed'

  # Restoration: the unmutated binary still lists the subcommands for the
  # same input -- the GREEN reproduces exactly.
  run_bin "$shared_bin" "$shared_root" --help
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/restoration-broken'
  grep -Fq 'review' "$run_dir/out.stdout" || fail 'MUT-001/restoration-broken'
  grep -Fq 'docs' "$run_dir/out.stdout" || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-443.go
  cat >"$root/tests/unit/aur443_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR443UnitBridge(t *testing.T) { TestAUR443(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR443UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR443:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR443:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-443.go
  cat >"$root/tests/integration/aur443_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR443IntegrationBridge(t *testing.T) { IntegrationAUR443(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR443IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR443:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR443:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-443.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-build its own copy. The
  # nested script's own exit-code vocabulary is preserved: its 79 is an
  # environment gap and must be re-emitted as infra here, never collapsed
  # into behavioral RED (see EXIT_CODE_CONVENTION.md).
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-443.sh E2EAUR443)
  rc=$?
  set -e
  ((rc != 79)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mutation_case
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR443) unit_case ;;
  IntegrationAUR443) integration_case ;;
  E2EAUR443) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
