#!/usr/bin/env bash
#
# Acceptance program for card AUR-503, scenarios AC-001/AC-002/AC-003 plus
# the two skeptical mutations MUT-001/MUT-002.
#
# WHAT THIS PROVES
#
#   AUR-486 added the Go (`exec.Command("sh", "-c", ...)`) and C#
#   (`Process.Start("cmd.exe", "/c ping " + host)`) command-injection
#   branches, and AUR-486's round 2 anchored only `sh -c`/SQL. The
#   2026-09-15 measurement reproduced the same false-positive class in the
#   remaining branches: a textual mention of `exec.Command`,
#   `Process.Start`, `Invoke-Expression`/`iex` or `eval` inside a comment
#   (`//`, `#`, `/* */`, `--`) or a string literal, and `eval :=`/`eval =`
#   (a declaration), all produced a finding.
#
#   This program proves the fix (internal/review/rules/security.yml):
#   each of those branches is anchored to a statement context, so every
#   mention and declaration in a comment/string produces NO finding
#   (AC-001/AC-003) while the real invocation of every branch is still
#   found (AC-002). It then mutates the catalog to prove the acceptance is
#   not vacuous: removing the Go branch's statement guard re-accuses the
#   comment (MUT-001), and over-restricting it to column zero loses the
#   real invocation (MUT-002). A green AC-001/AC-002 with either mutation
#   applied would be a false green.
#
# WHY THIS GENERATES ITS INPUT AT RUNTIME
#
#   The sealed acceptance profile (bootstrap-readonly-v1) carries bash and
#   a Go toolchain but no `git` binary, and this card's `paths` do not
#   include tests/fixtures, so there is no committed fixture. Every proof at
#   or below the package boundary runs over synthetic types.Diff values at
#   the review.SecurityScan entry point; the integration proof writes its
#   own bare repository in the loose-object format
#   tests/fixtures/repos/git-demo/build-fixture.sh emits. Nothing here
#   shells out to `git`.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001/MUT-002 mutant)
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

readonly card='AUR-503'
readonly scenario='AC-001'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  internal/review/rules/security.yml
  tests/unit/AUR-503.go
  tests/integration/AUR-503.go
  docs/specs/AUR-503.md
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/config
  internal/prompt
  internal/review
  internal/security
  internal/git
  pkg/types
  tests/fixtures/review/vuln/node-xss-command-injection/repo.git
  tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a503.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"
export GOMAXPROCS=1

unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

stage_source() {
  local root="$1"
  mkdir -p "$root"
  # The whole build closure of cmd/aurumcode travels with the card; the
  # specific package list is not enumerated because its import graph grows
  # as sibling cards land, and a stale enumeration makes the build fail
  # closed for the wrong reason.
  copy "$root" go.mod go.sum cmd pkg internal
  copy "$root" tests/fixtures/review/vuln
  chmod -R u+w -- "$root"
}

# probe_source writes a single-file program that exercises the real catalog
# over one synthetic added line at the review.SecurityScan boundary, the
# same shape tests/unit/AUR-503.go uses. It is used for AC-001/AC-002/AC-003
# and for each mutation, without regenerating a repository.
probe_source() {
  cat >"$1/aur503_probe.go" <<'EOF'
package main

import (
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func scan(path, line string) int {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: 1,
			Lines:    []string{"+" + line},
		}},
	}}}
	f, err := review.SecurityScan(diff)
	if err != nil {
		fmt.Fprintln(os.Stderr, "SecurityScan:", err)
		os.Exit(2)
	}
	return len(f)
}

func main() {
	var path, line string
	switch os.Args[1] {
	// AC-001: textual mentions must not fire.
	case "go-comment":
		path, line = "main.go", "\t// exec.Command(\"sh\",\"-c\",\"ping \"+host)"
	case "go-comment-sep":
		// The `;` here is INSIDE the comment, which a line anchor cannot
		// tell apart from a real statement separator.
		path, line = "main.go", "\t// foo; exec.Command(\"sh\",\"-c\",\"ping \"+host)"
	case "csharp-comment":
		path, line = "App.cs", "\t// Process.Start(\"cmd.exe\", \"/c ping \" + host)"
	case "ps-comment":
		path, line = "script.ps1", "\t# Invoke-Expression \"ping $host\""
	case "iex-string":
		path, line = "script.ps1", "\tWrite-Host \"iex is a string literal\""
	case "eval-comment":
		path, line = "deploy.sh", "\t# eval \"ping $1\" is dangerous"
	case "eval-string":
		path, line = "deploy.sh", "\tmsg=\"eval ping \\$1 in a variable\""
	// AC-002: the real invocation of each branch must fire.
	case "go-unsafe":
		path, line = "main.go", "\texec.Command(\"sh\", \"-c\", \"ping \"+host).Run()"
	// Expression contexts: a line anchor that only admits line start or a
	// `;`/`|`/`&` separator misses every one of these (the AUR-503 review
	// blocker); they are real invocations and must fire.
	case "go-expr":
		path, line = "main.go", "\tx := exec.Command(\"sh\", \"-c\", cmd)"
	case "go-if":
		path, line = "main.go", "\tif err := exec.Command(\"sh\", \"-c\", cmd); err != nil { return }"
	case "go-out":
		path, line = "main.go", "\tout, err := exec.Command(\"sh\", \"-c\", cmd)"
	case "go-return":
		path, line = "main.go", "\treturn exec.Command(\"sh\", \"-c\", cmd)"
	case "go-decl":
		path, line = "main.go", "\tcmd := exec.Command(\"sh\", \"-c\", cmd)"
	case "csharp-unsafe":
		path, line = "App.cs", "\tProcess.Start(\"cmd.exe\", \"/c ping \" + host);"
	case "csharp-var":
		path, line = "App.cs", "\tvar p = Process.Start(\"cmd.exe\", \"/c ping \" + host);"
	case "csharp-if":
		path, line = "App.cs", "\tif (x) { Process.Start(\"cmd.exe\", \"/c ping \" + host); }"
	case "ps-unsafe":
		path, line = "script.ps1", "\tInvoke-Expression \"ping $host\""
	case "ps-assign":
		path, line = "script.ps1", "\t$r = Invoke-Expression $payload"
	case "iex-unsafe":
		path, line = "script.ps1", "\tiex $payload"
	case "iex-assign":
		path, line = "script.ps1", "\t$r = iex $payload"
	case "shc-unsafe":
		path, line = "deploy.sh", "\tsh -c \"ping $1\""
	case "bashc-unsafe":
		path, line = "deploy.sh", "\tbash -c \"ping $1\""
	case "sql-shell":
		path, line = "deploy.sh", "\tquery=\"SELECT * FROM users WHERE name = '$1'\""
	// AC-003: declarations vs invocations of eval.
	case "eval-decl":
		path, line = "main.go", "\teval := compute()"
	case "eval-assign":
		path, line = "main.go", "\teval = compute()"
	case "eval-unsafe":
		path, line = "deploy.sh", "\teval \"ping $1\""
	case "eval-var":
		path, line = "deploy.sh", "\teval $cmd"
	default:
		fmt.Fprintln(os.Stderr, "unknown probe mode")
		os.Exit(2)
	}
	fmt.Printf("findings=%d\n", scan(path, line))
}
EOF
}

build_probe() {
  local root="$1" bin="$2"
  local log="$root/probe-build.log"
  if ! (cd "$root" && go build -o "$bin" ./aur503_probe.go) >"$log" 2>&1; then
    cat "$log" >&2
    infra build_failed
  fi
}

probe() {
  local bin="$1" mode="$2"
  local out
  if ! out="$("$bin" "$mode" 2>&1)"; then
    printf '%s\n' "$out" >&2
    fail "probe-$mode-failed"
  fi
  printf '%s' "$out"
}

# expect_findings asserts the probe reports exactly the requested count.
expect_findings() {
  local bin="$1" mode="$2" want="$3" label="$4"
  local out
  out="$(probe "$bin" "$mode")"
  [[ "$out" == "findings=$want" ]] || fail "$label:$mode:want=$want:got=$out"
}

# --- MUT-001: remove the command-injection context guard ------------------
# The AUR-503 fix keeps the four branches unanchored and drops a
# command-injection match whose first byte sits in a comment/string
# (codeMask, internal/review/securitypass.go). Removing that guard makes a
# comment mention match again, so `// exec.Command(...)` and
# `// foo; exec.Command(...)` fire. If they do not, the mutation did not
# take effect and the acceptance would be vacuous.
readonly mask_guard='rule.ID == "security/command-injection" && !codeMask(body)[loc[0]]'
readonly mask_noguard='rule.ID == "security/command-injection" && false'

# --- MUT-002: over-restrict the Go branch to column zero ------------------
# Requiring the token at the very start of the line loses every indented or
# expression-context real invocation, so the mutation proves AC-002's
# expression coverage is load-bearing rather than a false green.
readonly go_branch='\bexec\.Command'
readonly go_strict='^exec\.Command'

replace_once() {
  local target="$1" needle="$2" repl="$3" label="$4"
  NEEDLE="$needle" REPL="$repl" awk '
    BEGIN { needle = ENVIRON["NEEDLE"]; repl = ENVIRON["REPL"]; done = 0 }
    {
      if (!done && index($0, needle) > 0) {
        i = index($0, needle)
        $0 = substr($0, 1, i - 1) repl substr($0, i + length(needle))
        done = 1
      }
      print
    }
    END { if (!done) exit 1 }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail "$label/rewrite-failed"
  if grep -Fq -- "$needle" "$target"; then fail "$label/mutation-not-applied"; fi
}

mutation_case_1() {
  local root="$run_dir/root-mut1" bin="$run_dir/probe-mut1"
  stage_source "$root"; probe_source "$root"
  replace_once "$root/internal/review/securitypass.go" "$mask_guard" "$mask_noguard" MUT-001
  build_probe "$root" "$bin"

  # The comment mentions must now be accused (the mutation reintroduces the FP).
  expect_findings "$bin" go-comment 1 'MUT-001/mutation-survived'
  expect_findings "$bin" go-comment-sep 1 'MUT-001/mutation-survived'
  # Specificity: a sibling branch's real invocation is untouched.
  expect_findings "$bin" eval-unsafe 1 'MUT-001/unrelated-shape-lost'
  expect_findings "$bin" shc-unsafe 1 'MUT-001/unrelated-shape-lost'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# --- MUT-002: over-restrict the Go branch to column zero ------------------
# Requiring the token at the very start of the line loses the indented real
# invocation AND every expression-context invocation. If they are still
# found, the restriction did not take effect and the acceptance would be
# vacuous.
mutation_case_2() {
  local root="$run_dir/root-mut2" bin="$run_dir/probe-mut2"
  stage_source "$root"; probe_source "$root"
  replace_once "$root/internal/review/rules/security.yml" "$go_branch" "$go_strict" MUT-002
  build_probe "$root" "$bin"

  # The real, tab-indented Go invocation must now be missed.
  expect_findings "$bin" go-unsafe 0 'MUT-002/restriction-did-not-take'
  # And so must an expression-context invocation.
  expect_findings "$bin" go-expr 0 'MUT-002/restriction-did-not-take'
  # Specificity: a sibling branch's real invocation is untouched.
  expect_findings "$bin" eval-unsafe 1 'MUT-002/unrelated-shape-lost'
  expect_findings "$bin" csharp-unsafe 1 'MUT-002/unrelated-shape-lost'

  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

# --- AC-001/AC-002/AC-003 over one baseline build -------------------------

baseline_bin() {
  if [[ -z "${baseline_built:-}" ]]; then
    local root="$run_dir/root-baseline"
    stage_source "$root"; probe_source "$root"
    build_probe "$root" "$run_dir/probe-baseline"
    baseline_built=1
  fi
}

ac001_case() {
  baseline_bin
  local bin="$run_dir/probe-baseline"
  expect_findings "$bin" go-comment 0 'AC-001'
  expect_findings "$bin" go-comment-sep 0 'AC-001'
  expect_findings "$bin" csharp-comment 0 'AC-001'
  expect_findings "$bin" ps-comment 0 'AC-001'
  expect_findings "$bin" iex-string 0 'AC-001'
  expect_findings "$bin" eval-comment 0 'AC-001'
  expect_findings "$bin" eval-string 0 'AC-001'
}

ac002_case() {
  baseline_bin
  local bin="$run_dir/probe-baseline"
  expect_findings "$bin" go-unsafe 1 'AC-002'
  expect_findings "$bin" go-expr 1 'AC-002'
  expect_findings "$bin" go-if 1 'AC-002'
  expect_findings "$bin" go-out 1 'AC-002'
  expect_findings "$bin" go-return 1 'AC-002'
  expect_findings "$bin" go-decl 1 'AC-002'
  expect_findings "$bin" csharp-unsafe 1 'AC-002'
  expect_findings "$bin" csharp-var 1 'AC-002'
  expect_findings "$bin" csharp-if 1 'AC-002'
  expect_findings "$bin" ps-unsafe 1 'AC-002'
  expect_findings "$bin" ps-assign 1 'AC-002'
  expect_findings "$bin" iex-unsafe 1 'AC-002'
  expect_findings "$bin" iex-assign 1 'AC-002'
  expect_findings "$bin" shc-unsafe 1 'AC-002'
  expect_findings "$bin" bashc-unsafe 1 'AC-002'
  expect_findings "$bin" sql-shell 1 'AC-002'
}

ac003_case() {
  baseline_bin
  local bin="$run_dir/probe-baseline"
  expect_findings "$bin" eval-decl 0 'AC-003'
  expect_findings "$bin" eval-assign 0 'AC-003'
  expect_findings "$bin" eval-unsafe 1 'AC-003'
  expect_findings "$bin" eval-var 1 'AC-003'
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-503.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur503_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR503UnitBridge(t *testing.T) { TestAUR503(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR503UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR503:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR503:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-503.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur503_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR503IntegrationBridge(t *testing.T) { IntegrationAUR503(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR503IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR503:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR503:zero-tests
  cleanup_root "$root"
}

run_all() {
  unit_case
  integration_case
  ac001_case
  ac002_case
  ac003_case
  mutation_case_1
  mutation_case_2
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  all) run_all ;;
  AC-001) ac001_case; printf '%s/%s/AC-001/ok\n' "$card" "$scenario" ;;
  AC-002) ac002_case; printf '%s/%s/AC-002/ok\n' "$card" "$scenario" ;;
  AC-003) ac003_case; printf '%s/%s/AC-003/ok\n' "$card" "$scenario" ;;
  MUT-001) mutation_case_1 ;;
  MUT-002) mutation_case_2 ;;
esac
