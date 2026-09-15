#!/usr/bin/env bash
#
# Acceptance program for card AUR-486, scenario AC-001 (+ AC-002 + AC-003
# in the same nominal run, mirroring tests/acceptance/AUR-481.sh's shape).
#
# WHAT THIS PROVES
#
#   The 2026-08-26 measurement planted the same four defects (secret, sql,
#   cmd, xss) in eight languages and found command injection missed in Go,
#   C#, PowerShell and bash, each for a different concrete reason:
#
#     - Go: `exec.Command("sh", "-c", "ping "+host)` -- the `exec[lv]p?e?`
#       branch needs an `l`/`v` right after `exec`, and the dot in
#       `exec.Command` breaks it anyway.
#     - C#: `Process.Start(...)` was not in the pattern.
#     - PowerShell: `Invoke-Expression` was not in the pattern.
#     - bash: `eval "ping $1"` and `sh -c "ping $1"` were not in the
#       pattern, and SQL assembled by shell interpolation
#       (`query="... WHERE name = '$1'"`) has no `+` to anchor the SQL rule.
#
#   This program proves the fix (internal/review/rules/security.yml): the
#   five command-injection idioms and the SQL-by-shell shape are found
#   (AC-001); every card-named safe form -- the Go argv form, C# separated
#   arguments, `eval` inside a comment or a string literal, and a
#   parametrized query -- is not (AC-002); and the pre-existing Rust, Node
#   and Python regression shapes are unchanged (AC-003).
#
# WHY THIS GENERATES A REPOSITORY AT RUNTIME
#
#   The sealed acceptance profile (bootstrap-readonly-v1) carries bash and a
#   Go toolchain but no `git` binary, and this card's `paths` do not include
#   tests/fixtures, so there is no committed fixture. tests/e2e/AUR-486.sh
#   and tests/integration/AUR-486.go both write the loose-object bare-repo
#   format tests/fixtures/repos/git-demo/build-fixture.sh emits -- the same
#   shape internal/analyzer's pure-Go reader inflates.
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

readonly card='AUR-486'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  all|AC-001|TestAUR486|IntegrationAUR486|E2EAUR486|AC-001-MUT-001|AC-001-MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  internal/review/rules/security.yml
  tests/unit/AUR-486.go
  tests/integration/AUR-486.go
  tests/e2e/AUR-486.sh
  docs/specs/AUR-486.md
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
  tests/fixtures/review/vuln/repo.git
  tests/fixtures/review/vuln/hardcoded-secret/repo.git
  tests/fixtures/review/vuln/node-xss-command-injection/repo.git
  tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a486.XXXXXX")" || infra mktemp
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
  # specific package list is not enumerated because cmd/aurumcode's import
  # graph grows as sibling cards land (AUR-499 added internal/changelog and
  # friends), and a stale enumeration makes the build fail closed for the
  # wrong reason.
  copy "$root" go.mod go.sum cmd pkg internal
  copy "$root" tests/fixtures/review/vuln
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
    cat "$log" >&2
    infra build_failed
  fi
  shared_built=1
}

# The provider-free probe: exercises the real catalog over a synthetic
# one-line diff at the package boundary, the same shape
# tests/unit/AUR-486.go uses. Used to run each mutation against the mutated
# catalog without regenerating a repository.
probe_source() {
  cat >"$1/aur486_probe.go" <<'EOF'
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
	switch os.Args[1] {
	case "go-unsafe":
		fmt.Printf("findings=%d\n", scan("main.go", "\texec.Command(\"sh\", \"-c\", \"ping \"+host).Run()"))
	case "go-safe":
		fmt.Printf("findings=%d\n", scan("main.go", "\texec.Command(\"ping\", host).Run()"))
	case "csharp-unsafe":
		fmt.Printf("findings=%d\n", scan("App.cs", "\tProcess.Start(\"cmd.exe\", \"/c ping \" + host);"))
	case "bash-unsafe":
		fmt.Printf("findings=%d\n", scan("deploy.sh", "\teval \"ping $1\""))
	default:
		fmt.Fprintln(os.Stderr, "unknown probe mode")
		os.Exit(2)
	}
}
EOF
}

# mutation_go_branch swaps the Go shell-invocation branch's marker
# `exec\.Command` for `execQCommand`, which no longer matches
# `exec.Command(...)`. The literal bytes are replaced with awk's index() so
# the backslash is data, never a regex escape.
mutation_go_branch() {
  local target="$1"
  awk '
    {
      needle = "exec\\.Command"
      replacement = "execQCommand"
      while ((i = index($0, needle)) > 0) {
        $0 = substr($0, 1, i - 1) replacement substr($0, i + length(needle))
      }
      print
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail 'MUT-001/rewrite-failed'
  if grep -Fq 'exec\.Command' "$target"; then fail 'MUT-001/mutation-not-applied'; fi
}

# mutation_widen appends a bare `\bexec\.Command\s*\(` branch to the
# command-injection pattern, so the safe argv form also matches. It anchors
# on the rule's `- id:` line and rewrites the NEXT pattern line wholesale,
# so it does not depend on the pattern's exact bytes.
mutation_widen() {
  local target="$1"
  local guard='  - id: security/command-injection'
  [[ "$(grep -Fc "$guard" "$target")" == 1 ]] || fail 'MUT-002/guard-not-unique'
  local repl='|\bexec\.Command\s*\('
  local quote="'"
  GUARD="$guard" REPL="$repl" QUOTE="$quote" awk '
    BEGIN { guard = ENVIRON["GUARD"]; repl = ENVIRON["REPL"]; quote = ENVIRON["QUOTE"]; armed = 0; done = 0 }
    {
      if (!done && index($0, guard) > 0) { armed = 1; print $0; next }
      if (armed && !done && index($0, "    pattern: ") == 1) {
        line = substr($0, 1, length($0) - 1)
        print line repl quote
        done = 1
        next
      }
      print $0
    }
    END { if (!done) exit 1 }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail 'MUT-002/rewrite-failed'
}

run_probe() {
  local root="$1" mode="$2"
  local log="$root/probe-$mode.log"
  local out
  if ! out="$(cd "$root" && go run -mod=mod ./aur486_probe.go "$mode" 2>"$log")"; then
    cat "$log" >&2
    fail "probe-$mode-failed"
  fi
  printf '%s' "$out"
}

# mutation_case_1 is MUT-001: remove the Go spelling. The Go command
# injection must vanish (AC-001 RED); the C# and bash findings must stay,
# proving the mutation is specific and did not disable the whole rule.
mutation_case_1() {
  local root="$run_dir/root-mut1"
  stage_source "$root"
  probe_source "$root"
  mutation_go_branch "$root/internal/review/rules/security.yml"

  local out
  out="$(run_probe "$root" go-unsafe)"
  [[ "$out" == "findings=0" ]] || fail 'MUT-001/mutation-survived:go-unsafe-still-found'
  out="$(run_probe "$root" csharp-unsafe)"
  [[ "$out" == "findings=1" ]] || fail 'MUT-001/unrelated-shape-lost:csharp'
  out="$(run_probe "$root" bash-unsafe)"
  [[ "$out" == "findings=1" ]] || fail 'MUT-001/unrelated-shape-lost:bash'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# mutation_case_2 is MUT-002: widen the Go branch so the safe argv form
# matches. The safe form must wrongly surface (AC-002 RED); the Go unsafe
# line must remain found.
mutation_case_2() {
  local root="$run_dir/root-mut2"
  stage_source "$root"
  probe_source "$root"
  mutation_widen "$root/internal/review/rules/security.yml"

  local out
  out="$(run_probe "$root" go-safe)"
  [[ "$out" == "findings=1" ]] || fail 'MUT-002/widening-did-not-reintroduce-false-positive'
  out="$(run_probe "$root" go-unsafe)"
  [[ "$out" == "findings=1" ]] || fail 'MUT-002/unrelated-true-positive-lost'

  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-486.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur486_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR486UnitBridge(t *testing.T) { TestAUR486(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR486UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR486:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR486:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-486.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur486_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR486IntegrationBridge(t *testing.T) { IntegrationAUR486(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR486IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR486:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR486:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-486.sh
  chmod -R u+w -- "$root"
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-486.sh E2EAUR486) || fail e2e-failed
  cleanup_root "$root"
}

run_all() {
  unit_case
  integration_case
  e2e_case
  mutation_case_1
  mutation_case_2
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  all|AC-001) run_all ;;
  TestAUR486) unit_case ;;
  IntegrationAUR486) integration_case ;;
  E2EAUR486) e2e_case ;;
  AC-001-MUT-001) mutation_case_1 ;;
  AC-001-MUT-002) mutation_case_2 ;;
esac
