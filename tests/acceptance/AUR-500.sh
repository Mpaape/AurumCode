#!/usr/bin/env bash
#
# Acceptance program for card AUR-500.
#
# WHAT THIS PROVES
#
#   internal/reviewprofile is the built-in, VERSIONED reviewer-profile system:
#   solid, seguranca and performance are deterministic presets selected by
#   repository config or by review flag. A profile sets review EMPHASIS and the
#   ENABLED RULE FAMILIES only.
#
#   AC-001: a named profile applies by config or flag and the resolution
#           DECLARES which profile entered; the flag wins over config.
#   AC-002: every built-in has a version identifier and the same input yields
#           the same enabled families and instructions.
#   AC-003: switching profiles changes only emphasis/families. Severity,
#           --fail-on, redaction, the cost cap and the deterministic security
#           pass are fixed; a definition naming any of them is REFUSED, naming
#           the clause.
#   AC-004: an unknown profile is a named error before any model call; an
#           absent profile is the zero-config no-op.
#
#   MUT-001 (apply one profile regardless of selection), MUT-002 (let a
#   profile disable the deterministic security pass) and MUT-003 (neutralize
#   the no-prefix clause lookup so a YAML merge key smuggles `security_pass`
#   past the raw scan) must each make this program exit non-zero registering
#   the mutation id, and restoring the source must reproduce the exact GREEN.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-500'
scenario='AC-001'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

readonly read_inputs=(go.mod go.sum internal/reviewprofile)
for input in "${read_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a500.XXXXXX")" || infra mktemp
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
  mkdir -p "$root/cmd/aur500harness"
  cat >"$root/cmd/aur500harness/main.go" <<'EOF'
package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func families(p reviewprofile.Profile) string {
	parts := make([]string, 0)
	for _, f := range p.Families() {
		parts = append(parts, string(f))
	}
	return strings.Join(parts, ",")
}

func main() {
	// AC-001: a named profile applies by flag and by config; flag wins.
	flagRes, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "seguranca"})
	must(err)
	cfgRes, err := reviewprofile.Resolve(reviewprofile.Selection{Config: "performance"})
	must(err)
	overRes, err := reviewprofile.Resolve(reviewprofile.Selection{Config: "performance", Flag: "solid"})
	must(err)
	fmt.Printf("FLAG_PROFILE=%s\n", flagRes.Profile.Name)
	fmt.Printf("DECLARED=%s\n", flagRes.Declared)
	fmt.Printf("CONFIG_PROFILE=%s\n", cfgRes.Profile.Name)
	fmt.Printf("OVERRIDE_PROFILE=%s\n", overRes.Profile.Name)

	// AC-002: versioned and deterministic.
	a, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "solid"})
	must(err)
	b, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "solid"})
	must(err)
	versions := true
	for _, name := range reviewprofile.Names() {
		p, ok := reviewprofile.Builtin(name)
		if !ok || strings.TrimSpace(p.Version) == "" {
			versions = false
		}
	}
	fmt.Printf("VERSIONS_SET=%t\n", versions)
	fmt.Printf("DETERMINISTIC=%t\n", a.Profile.Signature() == b.Profile.Signature() && families(a.Profile) == families(b.Profile))

	// AC-003: boundary and refusal.
	_, secErr := reviewprofile.Compile([]byte("name: evil\nemphasis: e\nsecurity_pass: false\n"))
	fmt.Printf("REFUSED=%t\n", secErr != nil && errors.Is(secErr, reviewprofile.ErrRefusedClause))
	fmt.Printf("REFUSED_CLAUSE=%t\n", secErr != nil && strings.Contains(secErr.Error(), "security_pass"))
	_, failErr := reviewprofile.Compile([]byte("name: evil\nfail_on: error\n"))
	fmt.Printf("REFUSED_FAILON=%t\n", failErr != nil && strings.Contains(failErr.Error(), "fail_on"))
	_, mergeErr := reviewprofile.Compile([]byte("name: evil\n<<: &m {security_pass: false}\n"))
	fmt.Printf("REFUSED_MERGE=%t\n", mergeErr != nil && errors.Is(mergeErr, reviewprofile.ErrRefusedClause) && strings.Contains(mergeErr.Error(), "security_pass"))
	s, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "solid"})
	must(err)
	sec, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "seguranca"})
	must(err)
	boundaryFixed := s.Effective.SecurityPassEnabled && sec.Effective.SecurityPassEnabled &&
		s.Effective.RedactionEnabled && sec.Effective.RedactionEnabled &&
		s.Effective.SeverityFloor == "" && sec.Effective.SeverityFloor == "" &&
		s.Effective.FailOnThreshold == "" && sec.Effective.FailOnThreshold == "" &&
		s.Effective.CostCap == -1 && sec.Effective.CostCap == -1
	fmt.Printf("BOUNDARY_FIXED=%t\n", boundaryFixed)
	fmt.Printf("SWITCHED=%t\n", s.Effective.Emphasis != sec.Effective.Emphasis && families(s.Profile) != families(sec.Profile))

	// AC-004: unknown is named; absent is zero-config.
	_, unknownErr := reviewprofile.Resolve(reviewprofile.Selection{Flag: "inexistente"})
	fmt.Printf("UNKNOWN_NAMED=%t\n", unknownErr != nil && errors.Is(unknownErr, reviewprofile.ErrUnknownProfile) && strings.Contains(unknownErr.Error(), "inexistente"))
	zero, err := reviewprofile.Resolve(reviewprofile.Selection{})
	must(err)
	fmt.Printf("ZERO_CONFIG=%t\n", !zero.Applied && zero.Profile.Name == "")
}
EOF
}

build_harness() {
  local root="$1" label="$2"
  local bin="$run_dir/harness-$label"
  run_go "$root" build -o "$bin" ./cmd/aur500harness >"$root/build.log" 2>&1 || { cat "$root/build.log" >&2; infra build_failed; }
  printf '%s' "$bin"
}

check_nominal() {
  local out="$1"
  grep -qx 'FLAG_PROFILE=seguranca' <<<"$out" || fail "AC-001/flag:$out"
  grep -qx 'DECLARED=profile: seguranca (v1)' <<<"$out" || fail "AC-001/declared:$out"
  grep -qx 'CONFIG_PROFILE=performance' <<<"$out" || fail "AC-001/config:$out"
  grep -qx 'OVERRIDE_PROFILE=solid' <<<"$out" || fail "AC-001/override:$out"
  grep -qx 'VERSIONS_SET=true' <<<"$out" || fail "AC-002/version:$out"
  grep -qx 'DETERMINISTIC=true' <<<"$out" || fail "AC-002/determinism:$out"
  grep -qx 'REFUSED=true' <<<"$out" || fail "AC-003/refused:$out"
  grep -qx 'REFUSED_CLAUSE=true' <<<"$out" || fail "AC-003/clause:$out"
  grep -qx 'REFUSED_FAILON=true' <<<"$out" || fail "AC-003/failon:$out"
  grep -qx 'REFUSED_MERGE=true' <<<"$out" || fail "AC-003/merge:$out"
  grep -qx 'BOUNDARY_FIXED=true' <<<"$out" || fail "AC-003/boundary:$out"
  grep -qx 'SWITCHED=true' <<<"$out" || fail "AC-003/switched:$out"
  grep -qx 'UNKNOWN_NAMED=true' <<<"$out" || fail "AC-004/unknown:$out"
  grep -qx 'ZERO_CONFIG=true' <<<"$out" || fail "AC-004/zero-config:$out"
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

# MUT-001: apply one profile regardless of the selection. Selection.Name always
# returns "solid", so AC-001's flag/config/override assertions fail. The mutant
# must have an observable effect (FLAG_PROFILE=solid).
mutation_001() {
  scenario='MUT-001'
  local root="$run_dir/root-mut001"
  stage_source "$root"
  write_harness "$root"
  local target="$root/internal/reviewprofile/reviewprofile.go"
  local anchor='func (s Selection) Name() string {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-001/anchor-not-unique'
  local replacement="$anchor"$'\n\treturn "solid" // MUT-001: ignore the selection'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    { if (index($0, anchor) > 0) { print repl; next } print }
  ' "$target" >"$target.mut" || infra mutation_rewrite
  mv "$target.mut" "$target"
  grep -Fq 'MUT-001: ignore the selection' "$target" || infra mutation_not_applied

  local bin out
  bin="$(build_harness "$root" mut001)"
  out="$("$bin")" || infra mutant_run_failed
  if grep -qx 'FLAG_PROFILE=seguranca' <<<"$out" && grep -qx 'CONFIG_PROFILE=performance' <<<"$out" && grep -qx 'OVERRIDE_PROFILE=solid' <<<"$out"; then
    fail 'MUT-001'
  fi
  grep -qx 'FLAG_PROFILE=solid' <<<"$out" || fail 'MUT-001/mutation-had-no-effect'
}

# MUT-002: let a profile disable the deterministic security pass. The RAW YAML
# scan is neutralized, so Compile binds `security_pass: false` and the boundary
# reports the pass off. AC-003 must fail. The scan is the single load-bearing
# boundary; neutralizing it is the mutation a reviewer would try first.
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
  if grep -qx 'REFUSED=true' <<<"$out" && grep -qx 'BOUNDARY_FIXED=true' <<<"$out"; then
    fail 'MUT-002'
  fi
  grep -qx 'REFUSED=false' <<<"$out" || fail 'MUT-002/mutation-had-no-effect'
}

# MUT-003: neutralize the no-prefix clause lookup. yaml.v3 resolves a `<<`
# merge key after the raw scan, so removing the no-prefix match lets
# `<<: &m {security_pass: false}` reach the typed Spec and turn the pass off.
# AC-003's merge vector must go RED while the flat spellings stay refused.
mutation_003() {
  scenario='MUT-003'
  local root="$run_dir/root-mut003"
  stage_source "$root"
  write_harness "$root"
  local target="$root/internal/reviewprofile/reviewprofile.go"
  local anchor=$'\t\t\tif clause, ok := refusedClauseNames[key]; ok {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-003/anchor-not-unique'
  local replacement=$'\t\t\tif clause, ok := refusedClauseNames["MUT-003:"+key]; ok { // MUT-003: neutralize the no-prefix lookup'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    { if (index($0, anchor) > 0) { print repl; next } print }
  ' "$target" >"$target.mut" || infra mutation_rewrite
  mv "$target.mut" "$target"
  grep -Fq 'MUT-003: neutralize the no-prefix lookup' "$target" || infra mutation_not_applied

  local bin out
  bin="$(build_harness "$root" mut003)"
  out="$("$bin")" || infra mutant_run_failed
  if grep -qx 'REFUSED_MERGE=true' <<<"$out"; then
    fail 'MUT-003'
  fi
  grep -qx 'REFUSED_MERGE=false' <<<"$out" || fail 'MUT-003/mutation-had-no-effect'
  grep -qx 'REFUSED=true' <<<"$out" || fail 'MUT-003/flat-still-refused'
}

bridge_run() {
  local kind="$1" fn="$2" bridge="$3" label="$4"
  local root="$run_dir/root-$kind"
  stage_source "$root"
  mkdir -p "$root/tests/$kind"
  chmod -R u+w -- "$root/tests" >/dev/null 2>&1 || true
  cp "$repo_root/tests/$kind/AUR-500.go" "$root/tests/$kind/AUR-500.go"
  cat >"$root/tests/$kind/aur500_bridge_test.go" <<EOF
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

unit_case() { bridge_run unit TestAUR500 TestAUR500UnitBridge TestAUR500; }
integration_case() { bridge_run integration IntegrationAUR500 TestAUR500IntegrationBridge IntegrationAUR500; }

e2e_case() {
  [[ -f "$repo_root/tests/e2e/AUR-500.sh" ]] || infra "missing-input:tests/e2e/AUR-500.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-500.sh" E2EAUR500
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

case "$selector" in
  MUT-001) mutation_001 ;;
  MUT-002) mutation_002 ;;
  MUT-003) mutation_003 ;;
  AC-001|AC-002|AC-003|AC-004|all)
    nominal_case
    if [[ "$selector" == "all" ]]; then
      unit_case
      integration_case
      e2e_case
      mutation_001
      mutation_002
      mutation_003
    fi
    ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
