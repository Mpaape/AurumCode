#!/usr/bin/env bash
#
# Acceptance program for card AUR-460, scenario AC-001.
#
# WHAT THIS PROVES
#
#   Measured against a real gateway (2026-08-14): `--modelo gpt-5.6-luna`
#   returned a 400 -- "Unsupported value: 'temperature' does not support
#   0.3 with this model. Only the default (1) value is supported." -- and
#   the same rejection hit the whole gpt-5.6 family (luna, sol, terra) and
#   sending 0 instead of 0.3 did not help either. The fixed Temperature
#   default in internal/llm.DefaultOptions, internal/review.DefaultConfig
#   and the unconditional (no omitempty) field in
#   internal/llm/provider/litellm's wire struct meant every request to the
#   gateway carried an explicit sampling parameter, whether the caller
#   wanted one or not.
#
#   This program proves the fix a local fixture gateway that reproduces
#   the measured behavior exactly: it 400s any request whose JSON body
#   contains a "temperature" key, and answers normally otherwise. Given
#   that fixture, `aurumcode review --base HEAD~1 --modelo gpt-5.6-luna`
#   must now succeed and return the finding the model reported -- and
#   repeating the exact same command against the exact same input must
#   produce byte-identical output.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-460'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR460|IntegrationAUR460|E2EAUR460|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  tests/unit/AUR-460.go
  tests/integration/AUR-460.go
  tests/e2e/AUR-460.sh
  docs/specs/AUR-460.md
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod go.sum cmd/aurumcode internal/analyzer internal/config internal/llm internal/prompt
  internal/review internal/security/redaction pkg/types
  tests/fixtures/repos/git-demo/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a460.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir" GOMAXPROCS=1

copy() { local root="$1"; shift; local p; for p in "$@"; do mkdir -p "$root/$(dirname "$p")"; cp -R "$repo_root/$p" "$root/$p"; done; }

stage_source() {
  local root="$1"; mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode cmd/regenerate-docs
  copy "$root" internal/analyzer internal/prompt internal/review internal/security internal/git internal/llm internal/pipeline
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  chmod -R u+w -- "$root"
}

shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2; infra build_failed
  fi
  shared_built=1
}

case "$selector" in
  AC-001)
    build_shared
    (cd "$repo_root" && AURUMCODE_ROOT="$shared_root" AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-460.sh E2EAUR460) >/dev/null || fail e2e-failed
    printf '%s/%s/ok\n' "$card" "$scenario"
    ;;
  TestAUR460)
    build_shared
    copy "$shared_root" tests/unit/AUR-460.go
    cat >"$shared_root/tests/unit/aur460_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR460UnitBridge(t *testing.T) { TestAUR460(t) }
EOF
    (cd "$shared_root" && AURUMCODE_ROOT="$shared_root" AURUMCODE_BIN="$shared_bin" go test -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR460UnitBridge$' -count=1) || fail unit-failed
    ;;
  IntegrationAUR460)
    build_shared
    copy "$shared_root" tests/integration/AUR-460.go
    cat >"$shared_root/tests/integration/aur460_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR460IntegrationBridge(t *testing.T) { IntegrationAUR460(t) }
EOF
    (cd "$shared_root" && AURUMCODE_ROOT="$shared_root" AURUMCODE_BIN="$shared_bin" go test -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR460IntegrationBridge$' -count=1) || fail integration-failed
    ;;
  E2EAUR460)
    build_shared
    (cd "$repo_root" && AURUMCODE_ROOT="$shared_root" AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-460.sh E2EAUR460) || fail e2e-failed
    ;;
  AC-001-MUT-001)
    build_shared
    mroot="$run_dir/root-mut"
    rm -rf "$mroot"; cp -R "$shared_root" "$mroot"; chmod -R u+w -- "$mroot"

    # Reintroduce exactly the defect this card removes: an unconditional,
    # always-serialized "temperature" field on the litellm wire request.
    target="$mroot/internal/llm/provider/litellm/provider.go"
    ANCHOR1='type completionRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}' REPL1='type completionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}' awk '
      BEGIN { anchor = ENVIRON["ANCHOR1"]; repl = ENVIRON["REPL1"]; content = ""; }
      { content = content $0 "\n" }
      END {
        n = index(content, anchor)
        if (n == 0) { print "anchor1 not found" > "/dev/stderr"; exit 1 }
        printf "%s%s%s", substr(content, 1, n-1), repl, substr(content, n + length(anchor))
      }
    ' "$target" > "$target.tmp1" || infra mutation_rewrite_1
    mv "$target.tmp1" "$target"

    ANCHOR2='reqBody := completionRequest{
		Model: p.ResolveModel(opts),
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: opts.MaxTokens,
	}' REPL2='reqBody := completionRequest{
		Model: p.ResolveModel(opts),
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	}' awk '
      BEGIN { anchor = ENVIRON["ANCHOR2"]; repl = ENVIRON["REPL2"]; content = ""; }
      { content = content $0 "\n" }
      END {
        n = index(content, anchor)
        if (n == 0) { print "anchor2 not found" > "/dev/stderr"; exit 1 }
        printf "%s%s%s", substr(content, 1, n-1), repl, substr(content, n + length(anchor))
      }
    ' "$target" > "$target.tmp2" || infra mutation_rewrite_2
    mv "$target.tmp2" "$target"

    grep -q '"temperature"' "$target" || infra mutation_not_applied

    mbin="$run_dir/aurumcode-mut"
    (cd "$mroot" && go build -o "$mbin" ./cmd/aurumcode) >"$mroot/build.log" 2>&1 || { cat "$mroot/build.log" >&2; infra mutant_build_failed; }

    # Reuse the same fixture-gateway source tests/e2e/AUR-460.sh builds, so
    # the mutant faces the exact same restrictive gateway the GREEN path
    # already proved it could reach.
    gw_src="$run_dir/gatewaysrc"
    mkdir -p "$gw_src"
    sed -n '/^cat > "\$work\/gatewaysrc\/main.go" <<.GATEWAY_EOF.$/,/^GATEWAY_EOF$/p' "$repo_root/tests/e2e/AUR-460.sh" \
      | sed '1d;$d' > "$gw_src/main.go"
    [[ -s "$gw_src/main.go" ]] || infra gateway_source_extract_failed
    gw_bin="$run_dir/fakegateway-mut"
    (cd "$gw_src" && go mod init aur460gatewaymut >/dev/null 2>&1 && go build -mod=mod -o "$gw_bin" .) >"$gw_src/build.log" 2>&1 || { cat "$gw_src/build.log" >&2; infra gateway_build_failed; }

    addr_file="$run_dir/gw.addr"; capture_file="$run_dir/gw.capture"
    "$gw_bin" "$addr_file" "$capture_file" &
    gw_pid=$!
    trap 'kill "'"$gw_pid"'" >/dev/null 2>&1 || true; cleanup_root "'"$run_dir"'"' EXIT INT TERM HUP

    tries=0
    while [[ ! -s "$addr_file" ]]; do
      tries=$((tries + 1))
      (( tries < 200 )) || infra gateway_never_ready
      kill -0 "$gw_pid" >/dev/null 2>&1 || infra gateway_exited_early
      sleep 0.05
    done
    gw_addr="$(cat "$addr_file")"

    demo="$mroot/tests/fixtures/repos/git-demo/repo.git"
    set +e
    (cd "$demo" && env -u AURUMCODE_LLM_FIXTURE LLM_API_KEY=test-key LLM_BASE_URL="http://$gw_addr" \
      "$mbin" review --base HEAD~1 --modelo gpt-5.6-luna) >/dev/null 2>&1
    rc=$?
    set -e
    kill "$gw_pid" >/dev/null 2>&1 || true

    # The mutant MUST reproduce the defect: the restrictive gateway rejects
    # the temperature-carrying request, so the review must fail (rc != 0).
    [[ "$rc" -ne 0 ]] || fail "MUT-001-did-not-reproduce-the-defect:rc=$rc"
    grep -q '"temperature"' "$capture_file" || fail "MUT-001-mutant-did-not-even-send-temperature"
    printf '%s/%s/MUT-001/defect-reproduced\n' "$card" "$scenario"
    ;;
esac
exit 0
