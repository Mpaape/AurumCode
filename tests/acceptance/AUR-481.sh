#!/usr/bin/env bash
#
# Acceptance program for card AUR-481, scenario AC-001 (+ AC-002 + AC-003
# in the same nominal run, mirroring tests/acceptance/AUR-466.sh's shape).
#
# WHAT THIS PROVES
#
#   The 2026-08-26 measurement planted the same four defects (secret, sql,
#   cmd, xss) in eight languages and found Rust at zero for every
#   deterministic security rule: security/hardcoded-secret never matched
#   `const ACCESS_KEY: &str = "...";` (Rust puts a TYPE between the
#   identifier and `=`); security/sql-injection never matched
#   `"...".to_owned() + name` (Rust's concatenation puts a method call
#   between the closing quote and the `+`) nor `format!("...{}", name)`
#   (Rust's more common shape, no `+` at all); security/command-injection
#   never matched `Command::new("sh").arg("-c").arg(...)` (no
#   `exec`/`spawn`/`system`/`popen` spelling, no `shell: true` option).
#   This program proves the fix (internal/review/rules/security.yml): the
#   six Rust true-positive shapes this card restores are all found
#   (AC-001); the safe forms this card names -- a numeric constant, a
#   digit-free string constant, a $1-parametrized query, a format! call
#   with no SQL verb, and an argv-form Command with no shell -- are not
#   (AC-002); and the pre-existing Python, Node, and hardcoded-secret
#   regression fixtures still produce exactly the same findings, unchanged
#   (AC-003).
#
# RUST COMMAND-INJECTION: FIRST REVERTED, NOW RESTORED
#
#   Extending security/command-injection's pattern was first reverted
#   because it broke tests/acceptance/AUR-462.sh's own MUT-001, which
#   hardcoded that pattern's exact bytes as a literal grep/rewrite anchor
#   (confirmed by measurement: MUT-001/anchor-not-found, exit 1). AUR-462.sh
#   is now in this card's `paths`, and its MUT-001 was reanchored
#   (2026-09-03) on the rule's `- id:` line plus a content-independent
#   rewrite of the following `pattern:` line -- so it no longer depends on
#   this pattern's exact bytes, and the Rust command-injection branch is
#   restored. Go, C#, PowerShell and bash command-injection coverage (also
#   named by this card's outcome) remain out for the ordinary reason: time.
#   See docs/specs/AUR-481.md.
#
# WHY THIS READS A COMMITTED FIXTURE, NOT AN EPHEMERAL REPOSITORY
#
#   Same reasoning tests/acceptance/AUR-466.sh's own header states: the
#   sealed acceptance profile (bootstrap-readonly-v1) carries no `git`
#   binary, so tests/fixtures/review/vuln/rust-secret-sql-injection is a
#   bare, loose-object repository built by
#   tests/fixtures/repos/git-demo/build-fixture.sh from
#   tests/fixtures/review/vuln/rust-secret-sql-injection/history.spec.
#   Its HEAD~1..HEAD diff adds the identical file
#   tests/unit/AUR-481.go's aur481FixtureLines embeds as a synthetic diff,
#   so every selector agrees on line numbers.
#
# WHY NO LLM FIXTURE IS NEEDED
#
#   Same AUR-449 no-provider path AUR-462/AUR-466 rely on: `--seguranca`
#   alone, with LLM_API_KEY/LLM_BASE_URL/AURUMCODE_LLM_FIXTURE unset,
#   skips quality review and runs only the deterministic, model-free
#   security pass. This program unsets all three so every run takes that
#   path.
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

readonly card='AUR-481'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR481|IntegrationAUR481|E2EAUR481|AC-001-MUT-001|AC-001-MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  internal/review/rules/security.yml
  tests/unit/AUR-481.go
  tests/integration/AUR-481.go
  tests/e2e/AUR-481.sh
  tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git
  docs/specs/AUR-481.md
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
  internal/documentation
  internal/pipeline
  pkg/types
  internal/llm
  tests/fixtures/review/vuln/repo.git
  tests/fixtures/review/vuln/hardcoded-secret/repo.git
  tests/fixtures/review/vuln/node-xss-command-injection/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a481.XXXXXX")" || infra mktemp
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
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/config internal/prompt internal/review internal/security
  copy "$root" internal/git internal/documentation internal/pipeline
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/review/vuln
  chmod -R u+w -- "$root"
}

readonly rust_repo="$repo_root/tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git"
readonly header='Security findings (standards/security-review):'
readonly secret_citation='(rule security/hardcoded-secret: Hardcoded Secrets)'
readonly sql_citation='(rule security/sql-injection: SQL Injection Vulnerability)'
readonly cmd_citation='(rule security/command-injection: Command Injection)'

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

# nominal_case is AC-001's + AC-002's + AC-003's core behavioral proof.
nominal_case() {
  build_shared

  local out_sec
  out_sec="$(cd "$rust_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
  grep -Fq "$header" <<<"$out_sec" || fail behavior-missing

  # AC-001: the six Rust true-positive shapes this card restores.
  for ln in 1 6 11 15 24 28; do
    grep -Fq "src/main.rs:$ln: [error]" <<<"$out_sec" || fail "behavior-missing:line-$ln"
  done
  grep -Fq "$secret_citation" <<<"$out_sec" || fail behavior-missing:secret-citation
  grep -Fq "$sql_citation" <<<"$out_sec" || fail behavior-missing:sql-citation
  grep -Fq "$cmd_citation" <<<"$out_sec" || fail behavior-missing:cmd-citation

  # AC-002: exactly 6 findings total -- no other line in the fixture
  # (the numeric const, the digit-free const, the parametrized query, the
  # argv-form Command) ever appears.
  local after_header count
  after_header="${out_sec#*"$header"}"
  count="$(grep -Fo '[error]' <<<"$after_header" | wc -l)"
  [[ "$count" -eq 6 ]] || fail "unexpected-finding-count:$count"

  # Determinism.
  local out_again
  out_again="$(cd "$rust_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic

  # AC-003: the pre-existing Python SQL-injection regression fixture is
  # unaffected -- same finding, same count.
  local py_repo="$shared_root/tests/fixtures/review/vuln/repo.git"
  local out_py
  out_py="$(cd "$py_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:python-regression
  grep -Fq 'src/db.py:8: [error]' <<<"$out_py" || fail regression:python-sql-injection-missing
  grep -Fq "$sql_citation" <<<"$out_py" || fail regression:python-citation-missing
  [[ "$(grep -Fo '[error]' <<<"$out_py" | wc -l)" -eq 1 ]] || fail regression:python-finding-count-changed

  # AC-003: the pre-existing hardcoded-secret regression fixture is
  # unaffected -- two findings, same lines, same citation.
  local secret_repo="$shared_root/tests/fixtures/review/vuln/hardcoded-secret/repo.git"
  local out_secret
  out_secret="$(cd "$secret_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:secret-regression
  grep -Fq "$secret_citation" <<<"$out_secret" || fail regression:secret-citation-missing
  [[ "$(grep -Fo "$secret_citation" <<<"$out_secret" | wc -l)" -eq 2 ]] || fail regression:secret-finding-count-changed
  if grep -Fq 'AURUM-FAKE-KEY-9000-2222' <<<"$out_secret"; then fail regression:secret-value-leaked; fi
  if grep -Fq 'AURUM-FAKE-PASSWORD-9000-1111' <<<"$out_secret"; then fail regression:secret-value-leaked; fi

  # AC-003: the AUR-462 Node command-injection/xss regression fixture is
  # unaffected -- same finding counts, unchanged. This is the fixture that
  # proves security/command-injection's pattern itself was never touched.
  local node462_repo="$shared_root/tests/fixtures/review/vuln/node-xss-command-injection/repo.git"
  local out_462
  out_462="$(cd "$node462_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:node462-regression
  local c462 x462
  local after462="${out_462#*"$header"}"
  c462="$(grep -Fo "$cmd_citation" <<<"$after462" | wc -l)"
  x462="$(grep -Fo '(rule security/xss: Cross-Site Scripting (XSS))' <<<"$after462" | wc -l)"
  [[ "$c462" -eq 3 ]] || fail "regression:node462-command-injection-count:$c462"
  [[ "$x462" -eq 1 ]] || fail "regression:node462-xss-count:$x462"

  # Without --seguranca: no security section leaks in.
  local out_base
  out_base="$(cd "$rust_repo" && "$shared_bin" review --base HEAD~1 2>/dev/null || true)"
  if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi
}

# mutation_case_1 is MUT-001: remove BOTH Rust sql-injection alternatives
# this card added (the .to_owned()/.to_string() branch and the format!
# branch) and prove the two Rust SQL true positives (lines 11 and 15)
# vanish -- the AC-001 property this card exists to deliver.
mutation_case_1() {
  build_shared
  local root="$run_dir/root-mut1"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  # marker is a quote-free prefix of the FIRST new Rust branch AUR-481
  # added to security/sql-injection's pattern -- deliberately quote-free
  # so it can be embedded in a bash single-quoted string with no YAML
  # single-quote-doubling ambiguity (the pattern's own `["'']` character
  # class is what makes a hand-embedded, quote-bearing anchor fragile).
  # It is unique in the file: the ORIGINAL first branch has the same
  # `\b(select|insert|update|delete)\b[^+]*` text but with no leading `|`
  # (it opens the alternation, it does not join it).
  local marker='|\b(select|insert|update|delete)\b[^+]*'
  grep -Fq "$marker" "$target" || fail 'MUT-001/marker-not-found'
  [[ "$(grep -Fc "$marker" "$target")" == 1 ]] || fail 'MUT-001/marker-not-unique'
  # Truncate the pattern line at the marker and restore the closing quote
  # -- this reverts security/sql-injection to exactly its pre-AUR-481
  # pattern (both new Rust branches removed in one cut, since the marker
  # sits at the start of the first one and the second follows it with no
  # other content after).
  local single_quote="'"
  MARKER="$marker" QUOTE="$single_quote" awk '
    BEGIN { marker = ENVIRON["MARKER"]; quote = ENVIRON["QUOTE"] }
    {
      idx = index($0, marker)
      if (idx > 0) {
        print substr($0, 1, idx - 1) quote
      } else {
        print $0
      }
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail 'MUT-001/rewrite-failed'
  grep -Fq "$marker" "$target" && fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut1"
  local log="$root/build-mut1.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local out
  out="$(cd "$rust_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-001/mutation-run-failed'
  grep -Fq "$header" <<<"$out" || fail 'MUT-001/pass-did-not-run'
  # The mutant must LOSE both Rust SQL true positives.
  if grep -Fq 'src/main.rs:11: [error]' <<<"$out"; then fail 'MUT-001/mutation-survived:line-11'; fi
  if grep -Fq 'src/main.rs:15: [error]' <<<"$out"; then fail 'MUT-001/mutation-survived:line-15'; fi

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# mutation_case_2 is MUT-002: widen the format! branch by dropping its SQL
# verb requirement, and prove the AC-002 safe form (a format! call with no
# SQL verb) is wrongly matched -- a candidate that widens past what this
# card's Non-goals forbid must be rejected.
mutation_case_2() {
  build_shared
  local root="$run_dir/root-mut2"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-002/target-missing'
  local anchor='\bformat!\s*\(\s*"[^"]*\b(?:select|insert|update|delete)\b[^"]*\{\}[^"]*"\s*,\s*[A-Za-z_$][\w$.]*'
  local replacement='\bformat!\s*\(\s*"[^"]*\{\}[^"]*"\s*,\s*[A-Za-z_$][\w$.]*'
  grep -Fq "$anchor" "$target" || fail 'MUT-002/anchor-not-found'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-002/anchor-not-unique'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    {
      idx = index($0, anchor)
      if (idx > 0) {
        print substr($0, 1, idx - 1) repl substr($0, idx + length(anchor))
      } else {
        print $0
      }
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail 'MUT-002/rewrite-failed'
  grep -Fq "$anchor" "$target" && fail 'MUT-002/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut2"
  local log="$root/build-mut2.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-002/build-failed'
  fi

  # The mutant must NOT lose the real Rust SQL true positive on line 11
  # (this mutation only widens the format! branch; it does not touch the
  # to_owned/to_string branch or the original C/Python branch).
  local out
  out="$(cd "$rust_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-002/mutation-run-failed'
  grep -Fq "$header" <<<"$out" || fail 'MUT-002/pass-did-not-run'
  grep -Fq 'src/main.rs:11: [error]' <<<"$out" || fail 'MUT-002/unrelated-true-positive-lost'

  # The mutant MUST wrongly match the AC-002 safe form (a format! call
  # with no SQL verb) -- proving the widening this mutation applies is
  # exactly what this card's Non-goals forbid. Checked at the package
  # boundary (review.SecurityScan over a synthetic diff), the same way
  # tests/unit/AUR-481.go proves the non-mutated behavior, since the
  # sealed acceptance profile has no `git` to build a throwaway repo with.
  cat >"$root/aur481_mut2_probe.go" <<'EOF'
package main

import (
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func main() {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "src/main.rs",
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: 1,
			Lines:    []string{`+    let s = format!("user {}", name);`},
		}},
	}}}
	findings, err := review.SecurityScan(diff)
	if err != nil {
		fmt.Fprintln(os.Stderr, "SecurityScan:", err)
		os.Exit(2)
	}
	for _, f := range findings {
		if f.RuleID == "security/sql-injection" {
			fmt.Println("MATCHED")
			return
		}
	}
	fmt.Println("NOT-MATCHED")
}
EOF
  local probe_out
  probe_out="$(cd "$root" && go run ./aur481_mut2_probe.go 2>&1)" || fail 'MUT-002/probe-run-failed'
  [[ "$probe_out" == "MATCHED" ]] || fail 'MUT-002/widening-did-not-reintroduce-false-positive'

  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-481.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur481_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR481UnitBridge(t *testing.T) { TestAUR481(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR481UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR481:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR481:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-481.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur481_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR481IntegrationBridge(t *testing.T) { IntegrationAUR481(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR481IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR481:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR481:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-481.sh
  chmod -R u+w -- "$root"
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-481.sh E2EAUR481) || fail e2e-failed
  cleanup_root "$root"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mutation_case_1
  mutation_case_2
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR481) unit_case ;;
  IntegrationAUR481) integration_case ;;
  E2EAUR481) e2e_case ;;
  AC-001-MUT-001) mutation_case_1 ;;
  AC-001-MUT-002) mutation_case_2 ;;
esac
