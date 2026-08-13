#!/usr/bin/env bash
#
# Acceptance program for card AUR-436, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode review --base HEAD~1 --modelo <nome>` lets the user choose
#   which model reviews -- including a local one -- and when nothing is
#   configured to serve the chosen model the command fails loudly with exit
#   1 and a clear, actionable error (which model, why unavailable, how to
#   configure), never an empty review with exit 0. Without `--modelo`, the
#   contracts published by AUR-430 and AUR-431 hold byte for byte.
#
# HOW "LOCAL" IS EXERCISED OFFLINE
#
#   The sandbox denies network. `--modelo local` is exercised through the
#   deterministic offline provider (AURUMCODE_LLM_FIXTURE, see
#   review.FakeProvider): the flag commands the selection and the note on
#   stderr names the chosen model. The litellm-to-local-endpoint path (the
#   flag's model name reaching a real local HTTP endpoint on the wire) is
#   proved by tests/integration/AUR-436.go with net/http/httptest on
#   loopback -- no external network involved.
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

readonly card='AUR-436'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR436|IntegrationAUR436|E2EAUR436|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a436.XXXXXX")" || infra mktemp
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
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

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

# stage_source materializes exactly what `go build ./cmd/aurumcode` and the
# review run need: this card's owned cmd/aurumcode plus the read-only
# packages it imports and the git fixture the CLI is exercised against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/analyzer internal/prompt internal/review internal/llm pkg/types
  copy "$root" tests/fixtures/repos/git-demo
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# Deterministic offline model responses. Written here, not read from the
# repo, so the acceptance depends on no response fixture it does not fully
# control. fixture_error carries one error-severity finding (also exercises
# the AUR-431 gate composition); fixture_clean carries none, proving a
# healthy chosen model may still legitimately report an empty review.
fixture_error="$run_dir/response-error.json"
fixture_clean="$run_dir/response-clean.json"
cat >"$fixture_error" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN)."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF
cat >"$fixture_clean" <<'EOF'
{
  "issues": [],
  "summary": "Nothing to report."
}
EOF

# build_shared builds the binary once per acceptance run and reuses it for
# every case; GOCACHE is shared process-wide, so the go test compiles and
# the mutation rebuild start warm instead of cold (the sealed profile's
# 256MB memory ceiling is tight for cold net/http+crypto/tls builds -- see
# tests/acceptance/AUR-430.sh's build_shared note).
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# run_review runs the built binary as a user would (cwd inside the reviewed
# repository) with the offline provider configured, and reports its raw
# exit code in the global rc without tripping errexit. run_review_noprov
# instead empties every provider-selection variable, so nothing can serve
# the chosen model. stdout/stderr land in $run_dir/out.{stdout,stderr}.
run_review() {
  local bin="$1" repo_dir="$2" fixture="$3"; shift 3
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}
run_review_noprov() {
  local bin="$1" repo_dir="$2"; shift 2
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE= LLM_API_KEY= LLM_BASE_URL= LLM_MODEL= "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# nominal_case is AC-001's core behavioral proof: the user's --modelo
# choice commands the selection (including "local"), the unavailable-model
# path errors loudly instead of reporting an empty review, and every
# pre-existing contract survives.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local rc

  # The user chooses the local model: exit 0, the finding prints, the
  # stderr note names the chosen model, and stdout stays clean of it.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --modelo local
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail behavior-missing
  grep -Fq 'reviewing with model "local"' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq 'reviewing with model' "$run_dir/out.stdout" && fail selection-note-on-stdout
  local modelo_stdout
  modelo_stdout="$(cat "$run_dir/out.stdout")"

  # Same input, same output, same exit: the selection is deterministic.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --modelo local
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  [[ "$(cat "$run_dir/out.stdout")" == "$modelo_stdout" ]] || fail non-deterministic

  # The flag, not a hardcoded name, commands the selection: another model
  # name is echoed back by the note.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --modelo qwen2.5-coder
  [[ "$rc" -eq 0 ]] || fail named-model-failed
  grep -Fq 'reviewing with model "qwen2.5-coder"' "$run_dir/out.stderr" || fail flag-does-not-command-selection

  # Without --modelo, the AUR-430/431 published contract is untouched:
  # exit 0, byte-identical stdout, and no selection note on stderr.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail no-flag-contract-broken
  [[ "$(cat "$run_dir/out.stdout")" == "$modelo_stdout" ]] || fail no-flag-stdout-changed
  grep -Fq 'reviewing with model' "$run_dir/out.stderr" && fail no-flag-gained-note

  # The card's other half (and MUT-001's target): a model nothing is
  # configured to serve fails loudly -- exit 1, the error names the model
  # and says how to configure it -- never an empty review with exit 0.
  run_review_noprov "$shared_bin" "$repo_dir" --base HEAD~1 --modelo local
  [[ "$rc" -eq 1 ]] || fail unavailable-model-wrong-exit
  grep -Fq 'No issues found.' "$run_dir/out.stdout" && fail unavailable-model-empty-review
  grep -Fq 'model "local" is unavailable' "$run_dir/out.stderr" || fail unavailable-error-missing-model
  grep -Fq 'AURUMCODE_LLM_FIXTURE' "$run_dir/out.stderr" || fail unavailable-error-missing-offline-hint
  grep -Fq 'LLM_BASE_URL' "$run_dir/out.stderr" || fail unavailable-error-missing-endpoint-hint

  # A healthy chosen model may still legitimately find nothing: exit 0 and
  # "No issues found." remain correct when the model IS available.
  run_review "$shared_bin" "$repo_dir" "$fixture_clean" --base HEAD~1 --modelo local
  [[ "$rc" -eq 0 ]] || fail clean-review-broken
  grep -Fq 'No issues found.' "$run_dir/out.stdout" || fail clean-review-lost-output

  # An explicitly empty model name (`--modelo "$VAR"` with VAR unset) is a
  # usage error, exit 2, never a silent fallback.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --modelo ''
  [[ "$rc" -eq 2 ]] || fail empty-model-not-rejected
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --modelo=
  [[ "$rc" -eq 2 ]] || fail empty-model-not-rejected

  # The selection composes with the AUR-431 gate: the chosen model's error
  # finding still closes --fail-on high with exit 3.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --modelo local --fail-on high
  [[ "$rc" -eq 3 ]] || fail gate-composition-broken
}

# mutation_case is MUT-001: make the unavailable-model path hand the engine
# a deterministic empty review instead of the error (exactly the defect the
# card guards against: unavailable model -> "No issues found." with exit 0)
# in a writable staged copy, rebuild, and prove the mutant flips the
# behavior nominal_case asserts. The committed source is never touched, so
# restoration is by construction: shared_bin still errors GREEN.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild below recompiles only cmd/aurumcode.

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/main.go"
  local anchor='return nil, "", errors.New("no LLM provider is configured to serve it")'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # The mutant compiles against only what the file already imports:
  # review.FakeProvider answering an empty review for the model nothing is
  # configured to serve.
  sed -i 's|return nil, "", errors\.New("no LLM provider is configured to serve it")|return \&review.FakeProvider{Response: `{"issues":[],"summary":"ok"}`, NameStr: model}, "offline fixture provider", nil|' "$target"
  grep -Fq 'NameStr: model}, "offline fixture provider", nil' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  local rc
  # Under the mutant, the unavailable model silently reports an empty
  # review with exit 0 -- the exact behavior nominal_case rejects, so the
  # same accept run can never pass with this defect in place.
  run_review_noprov "$bin" "$repo_dir" --base HEAD~1 --modelo local
  [[ "$rc" -ne 1 ]] || fail 'MUT-001/not-rejected'
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/unexpected-exit'
  grep -Fq 'No issues found.' "$run_dir/out.stdout" || fail 'MUT-001/unexpected-output'

  # Restoration: the unmutated binary still errors -- the GREEN reproduces.
  run_review_noprov "$shared_bin" "$repo_dir" --base HEAD~1 --modelo local
  [[ "$rc" -eq 1 ]] || fail 'MUT-001/restoration-broken'
  grep -Fq 'model "local" is unavailable' "$run_dir/out.stderr" || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-436.go
  cat >"$root/tests/unit/aur436_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR436UnitBridge(t *testing.T) { TestAUR436(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR436UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR436:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR436:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-436.go
  cat >"$root/tests/integration/aur436_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR436IntegrationBridge(t *testing.T) { IntegrationAUR436(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR436IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR436:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR436:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-436.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-build its own copy. The
  # nested script's own exit-code vocabulary is preserved: its 79 is an
  # environment gap and must be re-emitted as infra here, never collapsed
  # into behavioral RED (see EXIT_CODE_CONVENTION.md).
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-436.sh E2EAUR436)
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
  TestAUR436) unit_case ;;
  IntegrationAUR436) integration_case ;;
  E2EAUR436) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
