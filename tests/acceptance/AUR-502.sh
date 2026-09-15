#!/usr/bin/env bash
#
# Acceptance program for card AUR-502.
#
# WHAT THIS PROVES
#
#   internal/reviewprofile grew the TEAM-DEFINED and MULTI-AGENT halves of the
#   reviewer-profile system on top of AUR-500:
#
#   AC-001: a versioned team file declares named profiles with emphasis, rule
#           families and instructions; a missing, empty, duplicate or unknown
#           name fails high naming the problem.
#   AC-002: two or more profiles run in the same review; their findings merge
#           deterministically (stable order, no duplicate), each names the
#           profile that produced it, and the output declares which profiles
#           entered.
#   AC-003: no profile -- built-in or team -- can change severity, relax
#           --fail-on, disable redaction, change the cost cap or disable the
#           deterministic security pass; a clause that tries is refused naming
#           the clause, merge keys and aliases included.
#   AC-004: no team file and a single profile behaves like AUR-500; zero-config
#           is unchanged; product_owner is a new built-in.
#
#   MUT-001 (ignore the selection and always add profiles) and MUT-002 (let a
#   team profile disable a boundary by neutralizing the raw clause scan) must
#   each make this program exit non-zero registering the mutation id, and
#   restoring the source must reproduce the exact GREEN.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-502'
scenario='AC-001'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Stage the whole module tree, not just the two packages under test: the
# integration bridge imports internal/config, which pulls internal/llm and
# internal/security/redaction through the same module. A minimal stage makes
# Go fall back to module-graph resolution and fail on an unrelated require.
readonly read_inputs=(go.mod go.sum internal pkg cmd)
for input in "${read_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a502.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
run_go() { local dir="$1"; shift; ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" ); }

stage_source() {
  local root="$1"
  mkdir -p "$root"
  local p dest
  for p in "${read_inputs[@]}"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    dest="$root/$p"
    if [[ -d "$repo_root/$p" ]]; then
      mkdir -p "$dest"
      chmod -R u+w -- "$dest" >/dev/null 2>&1 || true
      cp -R "$repo_root/$p/." "$dest/"
    else
      mkdir -p "$root/$(dirname "$p")"
      cp "$repo_root/$p" "$dest"
    fi
  done
  chmod -R u+w -- "$root"
}

write_harness() {
  local root="$1"
  mkdir -p "$root/cmd/aur502harness"
  cat >"$root/cmd/aur502harness/main.go" <<'EOF'
package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

const teamDoc = `
profiles:
  - name: release
    version: "2"
    emphasis: upstream compatibility
    families: [quality, performance]
    instructions: Confira compatibilidade de contrato.
  - name: cliente_x
    emphasis: client-specific rules
    families: [security]
    instructions: Confira os requisitos do cliente.
`

func main() {
	team, err := reviewprofile.LoadTeam([]byte(teamDoc))
	if err != nil {
		fmt.Println("TEAM_LOAD_ERROR=", err)
		return
	}
	fmt.Printf("TEAM_NAMES=%s\n", strings.Join(team.Names(), ","))

	// AC-001: missing, empty and duplicate names fail high.
	_, missErr := reviewprofile.LoadTeam([]byte("profiles:\n  - emphasis: e\n    families: [quality]\n    instructions: i\n"))
	_, emptyErr := reviewprofile.LoadTeam([]byte("profiles:\n  - name: x\n    families: [quality]\n"))
	_, dupErr := reviewprofile.LoadTeam([]byte("profiles:\n  - name: x\n    emphasis: a\n    families: [quality]\n    instructions: i\n  - name: X\n    emphasis: b\n    families: [security]\n    instructions: j\n"))
	fmt.Printf("MISSING_NAME=%t\n", errors.Is(missErr, reviewprofile.ErrMissingName))
	fmt.Printf("EMPTY_PROFILE=%t\n", errors.Is(emptyErr, reviewprofile.ErrEmptyProfile))
	fmt.Printf("DUP_NAME=%t\n", errors.Is(dupErr, reviewprofile.ErrDuplicateProfile))

	// AC-002: multi-agent resolution and deterministic attribution.
	_, unknownErr := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"release", "nao-existe"}}, team)
	fmt.Printf("UNKNOWN_NAMED=%t\n", errors.Is(unknownErr, reviewprofile.ErrUnknownProfile) && strings.Contains(unknownErr.Error(), "nao-existe"))

	multi, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"solid", "release"}}, team)
	if err != nil {
		fmt.Println("RESOLVE_ERROR=", err)
		return
	}
	fmt.Printf("MULTI_APPLIED=%t\n", multi.Applied && len(multi.Profiles) == 2)
	fmt.Printf("MULTI_DECLARED=%t\n", strings.Contains(multi.Declared, "solid") && strings.Contains(multi.Declared, "release"))

	shared := reviewprofile.Finding{RuleID: "quality/nit", File: "a.go", Line: 10, Message: "same", Severity: "info"}
	sec := reviewprofile.Finding{RuleID: "security/x", File: "b.go", Line: 3, Message: "sec", Severity: "error"}
	merged := reviewprofile.MergeFindings([]reviewprofile.Finding{
		{Profile: "solid", RuleID: shared.RuleID, File: shared.File, Line: shared.Line, Message: shared.Message, Severity: shared.Severity},
		{Profile: "seguranca", RuleID: sec.RuleID, File: sec.File, Line: sec.Line, Message: sec.Message, Severity: sec.Severity},
		{Profile: "seguranca", RuleID: shared.RuleID, File: shared.File, Line: shared.Line, Message: shared.Message, Severity: shared.Severity},
	})
	attributed := len(merged) == 2 && merged[0].File == "a.go" && merged[0].Profile == "solid" && merged[1].Profile == "seguranca"
	fmt.Printf("MERGE_ATTRIBUTED=%t\n", attributed)
	fmt.Printf("MERGE_DEDUP=%t\n", len(merged) == 2)
	again := reviewprofile.MergeFindings([]reviewprofile.Finding{
		{Profile: "solid", RuleID: shared.RuleID, File: shared.File, Line: shared.Line, Message: shared.Message, Severity: shared.Severity},
		{Profile: "seguranca", RuleID: sec.RuleID, File: sec.File, Line: sec.Line, Message: sec.Message, Severity: sec.Severity},
		{Profile: "seguranca", RuleID: shared.RuleID, File: shared.File, Line: shared.Line, Message: shared.Message, Severity: shared.Severity},
	})
	fmt.Printf("MERGE_STABLE=%t\n", len(again) == 2 && again[0].Profile == merged[0].Profile && again[1].Profile == merged[1].Profile)

	// AC-003: boundaries fixed and hostile clauses refused by name.
	fmt.Printf("BOUNDARY_FIXED=%t\n", multi.BoundariesFixed())
	_, teamSecErr := reviewprofile.LoadTeam([]byte("profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    security_pass: false\n"))
	fmt.Printf("REFUSED_TEAM=%t\n", teamSecErr != nil && errors.Is(teamSecErr, reviewprofile.ErrRefusedClause) && strings.Contains(teamSecErr.Error(), "security_pass"))
	_, mergeErr := reviewprofile.LoadTeam([]byte("profiles:\n  - <<: &m {redact_secrets: false}\n    name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n"))
	fmt.Printf("REFUSED_MERGE=%t\n", mergeErr != nil && errors.Is(mergeErr, reviewprofile.ErrRefusedClause) && strings.Contains(mergeErr.Error(), "redact_secrets"))

	// AC-004: zero-config unchanged, single profile == AUR-500, product_owner.
	zero, _ := reviewprofile.ResolveAll(reviewprofile.Selection{}, reviewprofile.EmptyTeam())
	fmt.Printf("ZERO_CONFIG=%t\n", !zero.Applied && zero.Profiles == nil)
	single, _ := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"seguranca"}}, reviewprofile.EmptyTeam())
	legacy, _ := reviewprofile.Resolve(reviewprofile.Selection{Flag: "seguranca"})
	fmt.Printf("SINGLE_COMPAT=%t\n", single.Applied && len(single.Profiles) == 1 && single.Profiles[0].Signature() == legacy.Profile.Signature())
	po, ok := reviewprofile.Builtin("product_owner")
	fmt.Printf("PRODUCT_OWNER=%t\n", ok && po.Effective.Emphasis != "" && po.Effective.SecurityPassEnabled && po.Effective.RedactionEnabled)
}
EOF
}

build_harness() {
  local root="$1" label="$2"
  local bin="$run_dir/harness-$label"
  run_go "$root" build -o "$bin" ./cmd/aur502harness >"$root/build.log" 2>&1 || { cat "$root/build.log" >&2; infra build_failed; }
  printf '%s' "$bin"
}

check_nominal() {
  local out="$1"
  grep -qx 'TEAM_NAMES=release,cliente_x' <<<"$out" || fail "AC-001/names:$out"
  grep -qx 'MISSING_NAME=true' <<<"$out" || fail "AC-001/missing:$out"
  grep -qx 'EMPTY_PROFILE=true' <<<"$out" || fail "AC-001/empty:$out"
  grep -qx 'DUP_NAME=true' <<<"$out" || fail "AC-001/duplicate:$out"
  grep -qx 'UNKNOWN_NAMED=true' <<<"$out" || fail "AC-001/unknown:$out"
  grep -qx 'MULTI_APPLIED=true' <<<"$out" || fail "AC-002/applied:$out"
  grep -qx 'MULTI_DECLARED=true' <<<"$out" || fail "AC-002/declared:$out"
  grep -qx 'MERGE_ATTRIBUTED=true' <<<"$out" || fail "AC-002/attributed:$out"
  grep -qx 'MERGE_DEDUP=true' <<<"$out" || fail "AC-002/dedup:$out"
  grep -qx 'MERGE_STABLE=true' <<<"$out" || fail "AC-002/stable:$out"
  grep -qx 'BOUNDARY_FIXED=true' <<<"$out" || fail "AC-003/boundary:$out"
  grep -qx 'REFUSED_TEAM=true' <<<"$out" || fail "AC-003/team:$out"
  grep -qx 'REFUSED_MERGE=true' <<<"$out" || fail "AC-003/merge:$out"
  grep -qx 'ZERO_CONFIG=true' <<<"$out" || fail "AC-004/zero-config:$out"
  grep -qx 'SINGLE_COMPAT=true' <<<"$out" || fail "AC-004/single:$out"
  grep -qx 'PRODUCT_OWNER=true' <<<"$out" || fail "AC-004/product-owner:$out"
}

nominal_case() {
  local root="$run_dir/root-nominal"
  stage_source "$root"
  write_harness "$root"
  local bin out
  bin="$(build_harness "$root" nominal)"
  out="$("$bin")" || fail nominal-run-failed
  check_nominal "$out"
}

# MUT-001: ignore the selection and always add profiles. Multi/single/zero
# selections all gain profiles, so AC-002's declaration and AC-004's
# zero-config/single-profile assertions go RED.
mutation_001() {
  scenario='MUT-001'
  local root="$run_dir/root-mut001"
  stage_source "$root"
  write_harness "$root"
  local target="$root/internal/reviewprofile/reviewprofile.go"
  local anchor='func (s Selection) SelectedNames() []string {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-001/anchor-not-unique'
  local replacement="$anchor"$'\n\treturn []string{"solid", "seguranca"} // MUT-001: ignore the selection'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    { if (index($0, anchor) > 0) { print repl; next } print }
  ' "$target" >"$target.mut" || infra mutation_rewrite
  mv "$target.mut" "$target"
  grep -Fq 'MUT-001: ignore the selection' "$target" || infra mutation_not_applied

  local bin out
  bin="$(build_harness "$root" mut001)"
  out="$("$bin")" || infra mutant_run_failed
  if grep -qx 'ZERO_CONFIG=true' <<<"$out" && grep -qx 'SINGLE_COMPAT=true' <<<"$out" && grep -qx 'MULTI_DECLARED=true' <<<"$out"; then
    fail 'MUT-001'
  fi
  grep -qx 'ZERO_CONFIG=false' <<<"$out" || fail 'MUT-001/mutation-had-no-effect'
}

# MUT-002: let a team profile disable a boundary by neutralizing the raw clause
# scan. A team file naming security_pass: false binds the pass off, so AC-003's
# BOUNDARY_FIXED / REFUSED_TEAM assertions go RED.
mutation_002() {
  scenario='MUT-002'
  local root="$run_dir/root-mut002"
  stage_source "$root"
  write_harness "$root"
  local target="$root/internal/reviewprofile/reviewprofile.go"
  local anchor='func scanRefusedClauses(n *yaml.Node, prefix string) error {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-002/anchor-not-unique'
  local replacement="$anchor"$'\n\treturn nil // MUT-002: neutralize the raw scan'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    { if (index($0, anchor) > 0) { print repl; next } print }
  ' "$target" >"$target.mut" || infra mutation_rewrite
  mv "$target.mut" "$target"
  grep -Fq 'MUT-002: neutralize the raw scan' "$target" || infra mutation_not_applied

  local bin out
  bin="$(build_harness "$root" mut002)"
  out="$("$bin")" || infra mutant_run_failed
  if grep -qx 'REFUSED_TEAM=true' <<<"$out" && grep -qx 'BOUNDARY_FIXED=true' <<<"$out"; then
    fail 'MUT-002'
  fi
  grep -qx 'REFUSED_TEAM=false' <<<"$out" || fail 'MUT-002/mutation-had-no-effect'
}

bridge_run() {
  local kind="$1" fn="$2" bridge="$3" label="$4"
  local root="$run_dir/root-$kind"
  stage_source "$root"
  mkdir -p "$root/tests/$kind"
  chmod -R u+w -- "$root/tests" >/dev/null 2>&1 || true
  cp "$repo_root/tests/$kind/AUR-502.go" "$root/tests/$kind/AUR-502.go"
  cat >"$root/tests/$kind/aur502_bridge_test.go" <<EOF
package $kind

import "testing"

func $bridge(t *testing.T) { $fn(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s "./tests/$kind" -run "^$bridge\$" -count=1 2>&1)"
  rc=$?
  set -e
  ((rc == 0)) || { printf '%s\n' "$out" >&2; fail "selector:$label:exit:$rc"; }
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$label:zero-tests"
}

unit_case() { bridge_run unit TestAUR502 TestAUR502UnitBridge TestAUR502; }
integration_case() { bridge_run integration IntegrationAUR502 TestAUR502IntegrationBridge IntegrationAUR502; }

e2e_case() {
  [[ -f "$repo_root/tests/e2e/AUR-502.sh" ]] || infra "missing-input:tests/e2e/AUR-502.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-502.sh" E2EAUR502
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

case "$selector" in
  MUT-001) mutation_001 ;;
  MUT-002) mutation_002 ;;
  AC-001|AC-002|AC-003|AC-004|all)
    nominal_case
    if [[ "$selector" == "all" ]]; then
      unit_case
      integration_case
      e2e_case
      mutation_001
      mutation_002
    fi
    ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
