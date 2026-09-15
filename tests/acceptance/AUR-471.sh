#!/usr/bin/env bash
#
# Acceptance program for card AUR-471.
#
# WHAT THIS PROVES
#
#   The ISO/IEC 25010 weights file is EXPLICIT USER CONFIGURATION with
#   authority over the review's severity policy -- unlike repository prompt
#   or provider text, which is untrusted DATA. This program builds a real Go
#   harness over internal/config/policy and asserts:
#
#     AC-001 with declared weights, the review reports a per-characteristic
#            and an aggregate score coherent with the file weights;
#     AC-002 weights not summing to 1.0, a negative weight, or an unknown
#            characteristic each fail with a NAMED error before any model
#            call could happen;
#     AC-003 with no file, the rendered output is byte-identical to today;
#     AC-004 a policy that tries to disable the deterministic security pass
#            or secret redaction is refused, naming the refused clause.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-471'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|TestAUR471|IntegrationAUR471|E2EAUR471|AC-002-MUT-001|AC-004-MUT-002) ;;
  *) printf '%s/unknown-selector/%s\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum internal/config/policy pkg/types tests/unit/AUR-471.go tests/integration/AUR-471.go tests/e2e/AUR-471.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a471.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

run_go() { local dir="$1"; shift; ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" ); }

# stage builds a minimal module containing only the package under test and
# its one dependency, so the harness cannot accidentally test the tree.
stage() {
  local root="$1"
  mkdir -p "$root/internal/config/policy" "$root/pkg/types"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$root/"
  cp -R "$repo_root/internal/config/policy/." "$root/internal/config/policy/"
  cp -R "$repo_root/pkg/types/." "$root/pkg/types/"
  chmod -R u+w -- "$root"
}

write_harness() {
  local root="$1"
  mkdir -p "$root/cmd/aur471accept"
  cat >"$root/cmd/aur471accept/main.go" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"

	policy "github.com/Mpaape/AurumCode/internal/config/policy"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func scores() types.ISOScores {
	return types.ISOScores{
		Functionality: 80, Reliability: 90, Usability: 70, Efficiency: 60,
		Maintainability: 100, Portability: 50, Security: 40, Compatibility: 30,
	}
}

func main() {
	mode := os.Args[1]
	switch mode {
	case "ac001":
		p, err := policy.Load(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(2)
		}
		ev, err := policy.Evaluate(p, scores())
		if err != nil || ev == nil {
			fmt.Fprintf(os.Stderr, "evaluate: %v\n", err)
			os.Exit(2)
		}
		for _, row := range ev.PerCharacteristic {
			fmt.Printf("weight %s=%.6f score=%d contribution=%.6f\n", row.Characteristic, row.Weight, row.Score, row.Contribution)
		}
		fmt.Printf("aggregate=%d\n", ev.Aggregate)
	case "ac002", "ac004":
		_, err := policy.Load(os.Args[2])
		if err != nil {
			fmt.Printf("err=%v\n", err)
		} else {
			fmt.Println("ok")
		}
	case "render":
		p, err := policy.Load(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(2)
		}
		ev, err := policy.Evaluate(p, scores())
		if err != nil {
			fmt.Fprintf(os.Stderr, "evaluate: %v\n", err)
			os.Exit(2)
		}
		result := &types.ReviewResult{Verdict: "pass", Summary: "ok", ISOScores: &types.ISOScores{Functionality: 80}}
		out, err := policy.Render(result, ev)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render: %v\n", err)
			os.Exit(2)
		}
		os.Stdout.Write(out)
	case "bypass":
		result := &types.ReviewResult{Verdict: "pass", Summary: "ok", ISOScores: &types.ISOScores{Functionality: 80}}
		out, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
			os.Exit(2)
		}
		os.Stdout.Write(out)
	default:
		fmt.Fprintln(os.Stderr, "unknown mode")
		os.Exit(2)
	}
}
EOF
}

build_harness() {
  local root="$1" bin="$2"
  stage "$root"
  write_harness "$root"
  run_go "$root" build -o "$bin" ./cmd/aur471accept >"$run_dir/build.log" 2>&1 || { cat "$run_dir/build.log" >&2; infra build_failed; }
}

# fixture writes a weights file under $1/.aurumcode.
fixture() {
  local root="$1" body="$2"
  mkdir -p "$root/.aurumcode"
  printf '%s' "$body" >"$root/.aurumcode/iso25010-weights.yml"
}

harness_bin="$run_dir/aur471accept"
harness_root="$run_dir/harness-root"

nominal_case() {
  build_harness "$harness_root" "$harness_bin"

  # ---- AC-003: no file is byte-identical to today's output ----
  local empty="$run_dir/empty"; mkdir -p "$empty"
  local got_bypass got_zero
  got_bypass="$("$harness_bin" bypass)"
  got_zero="$("$harness_bin" render "$empty")" || fail ac003-render-failed
  [[ "$got_zero" == "$got_bypass" ]] || fail "ac003-not-byte-identical:$got_zero:vs:$got_bypass"

  # ---- AC-001: declared weights drive per-characteristic + aggregate ----
  local declared="$run_dir/declared"
  fixture "$declared" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
'
  local out1
  out1="$("$harness_bin" ac001 "$declared")" || fail ac001-run-failed
  grep -Fq 'weight security=0.170000' <<<"$out1" || fail "ac001-security-weight:$out1"
  grep -Fq 'aggregate=70' <<<"$out1" || fail "ac001-aggregate:$out1"

  local shifted="$run_dir/shifted"
  fixture "$shifted" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.06
  portability: 0.08
  security: 0.29
  compatibility: 0.05
'
  local out2
  out2="$("$harness_bin" ac001 "$shifted")" || fail ac001-shifted-run-failed
  grep -Fq 'weight security=0.290000' <<<"$out2" || fail "ac001-shifted-weight:$out2"
  grep -Fq 'aggregate=63' <<<"$out2" || fail "ac001-shifted-aggregate:$out2"

  # ---- AC-002: each bad declaration fails NAMED, before any model call ----
  local bad
  bad="$run_dir/bad-sum"
  fixture "$bad" 'weights:
  functionality: 0.50
  reliability: 0.10
  usability: 0.10
  efficiency: 0.10
  maintainability: 0.10
  portability: 0.05
  security: 0.05
  compatibility: 0.05
'
  local e_sum
  e_sum="$("$harness_bin" ac002 "$bad")" || fail ac002-sum-run-failed
  grep -Fq 'weights-sum' <<<"$e_sum" || fail "ac002-sum-not-named:$e_sum"
  grep -Fq 'ok' <<<"$e_sum" && fail "ac002-sum-was-accepted:$e_sum"

  bad="$run_dir/bad-negative"
  fixture "$bad" 'weights:
  functionality: 0.30
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: -0.05
  compatibility: 0.12
'
  local e_neg
  e_neg="$("$harness_bin" ac002 "$bad")" || fail ac002-negative-run-failed
  grep -Fq 'negative-weight' <<<"$e_neg" || fail "ac002-negative-not-named:$e_neg"

  bad="$run_dir/bad-unknown"
  fixture "$bad" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
  agility: 0.10
'
  local e_unk
  e_unk="$("$harness_bin" ac002 "$bad")" || fail ac002-unknown-run-failed
  grep -Fq 'unknown-characteristic' <<<"$e_unk" || fail "ac002-unknown-not-named:$e_unk"

  # A non-finite weight (.nan) is not negative and a NaN sum defeats every
  # `>` comparison, so it must be rejected by an explicit finiteness check.
  bad="$run_dir/bad-nan"
  fixture "$bad" 'weights:
  functionality: .nan
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
'
  local e_nan
  e_nan="$("$harness_bin" ac002 "$bad")" || fail ac002-nan-run-failed
  grep -Fq 'non-finite-weight' <<<"$e_nan" || fail "ac002-nan-not-named:$e_nan"
  grep -Fq 'ok' <<<"$e_nan" && fail "ac002-nan-was-accepted:$e_nan"

  # A characteristic declared twice has its first weight counted in the raw
  # sum but overwritten in the map, so a file with an effective sum of 0.5
  # could slip through. It must be refused, naming invalid-weight, with no
  # policy produced.
  bad="$run_dir/bad-duplicate"
  fixture "$bad" 'weights:
  functionality: 0.50
  functionality: 0.50
  reliability: 0.10
  usability: 0.10
  efficiency: 0.10
  maintainability: 0.10
  portability: 0.05
  security: 0.05
  compatibility: 0.05
'
  local e_dup
  e_dup="$("$harness_bin" ac002 "$bad")" || fail ac002-duplicate-run-failed
  grep -Fq 'invalid-weight' <<<"$e_dup" || fail "ac002-duplicate-not-named:$e_dup"
  grep -Fq 'functionality' <<<"$e_dup" || fail "ac002-duplicate-clause-unnamed:$e_dup"
  grep -Fq 'ok' <<<"$e_dup" && fail "ac002-duplicate-was-accepted:$e_dup"

  # ---- AC-004: the safety boundary is refused, naming the clause ----
  local hostile out_s
  hostile="$run_dir/hostile-pass"
  fixture "$hostile" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
security_pass: false
'
  out_s="$("$harness_bin" ac004 "$hostile")" || fail ac004-pass-run-failed
  grep -Fq 'refused-clause' <<<"$out_s" || fail "ac004-pass-not-refused:$out_s"
  grep -Fq 'security_pass' <<<"$out_s" || fail "ac004-pass-clause-unnamed:$out_s"

  # YAML null (`~`) is a disabling spelling too; it must not slip past the
  # scalar spelling check and silently leave the pass off.
  hostile="$run_dir/hostile-tilde"
  fixture "$hostile" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
security_pass: ~
'
  local out_t
  out_t="$("$harness_bin" ac004 "$hostile")" || fail ac004-tilde-run-failed
  grep -Fq 'refused-clause' <<<"$out_t" || fail "ac004-tilde-not-refused:$out_t"
  grep -Fq 'security_pass' <<<"$out_t" || fail "ac004-tilde-clause-unnamed:$out_t"
  grep -Fq 'ok' <<<"$out_t" && fail "ac004-tilde-was-accepted:$out_t"

  hostile="$run_dir/hostile-redact"
  fixture "$hostile" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
redaction: off
'
  local out_r
  out_r="$("$harness_bin" ac004 "$hostile")" || fail ac004-redact-run-failed
  grep -Fq 'refused-clause' <<<"$out_r" || fail "ac004-redact-not-refused:$out_r"
  grep -Fq 'redaction' <<<"$out_r" || fail "ac004-redact-clause-unnamed:$out_r"

  hostile="$run_dir/hostile-nested"
  fixture "$hostile" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
security:
  redact_secrets: false
'
  local out_n
  out_n="$("$harness_bin" ac004 "$hostile")" || fail ac004-nested-run-failed
  grep -Fq 'security.redact_secrets' <<<"$out_n" || fail "ac004-nested-clause-unnamed:$out_n"

  # An alias spelling: `redaction: *off` is an AliasNode pointing at the
  # anchored scalar `off: &off false`. The refusal must dereference the
  # alias and name `redaction`, not silently accept the indirection.
  hostile="$run_dir/hostile-alias"
  fixture "$hostile" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
off: &off false
redaction: *off
'
  local out_a
  out_a="$("$harness_bin" ac004 "$hostile")" || fail ac004-alias-run-failed
  grep -Fq 'refused-clause' <<<"$out_a" || fail "ac004-alias-not-refused:$out_a"
  grep -Fq 'redaction' <<<"$out_a" || fail "ac004-alias-clause-unnamed:$out_a"
  if grep -Fq 'ok' <<<"$out_a"; then fail "ac004-alias-was-accepted:$out_a"; fi
}

# MUT-001: silently normalize weights that do not sum to 1.0. The AC-002
# bad-sum case must then survive to "ok" and the nominal assertion goes RED.
mutation_sum() {
  local root="$run_dir/mut001"
  build_harness "$root" "$harness_bin"
  local target="$root/internal/config/policy/policy.go"
  local anchor='	if !(math.Abs(sum-1.0) <= weightTolerance) {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-001/anchor-not-unique'
  ANCHOR="$anchor" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"] }
    { if ($0 == anchor) { print "\tsum = 1.0 // MUT-001: silently normalize"; print } else { print } }
  ' "$target" >"$target.mut" || infra mutation_rewrite
  mv "$target.mut" "$target"
  grep -Fq 'MUT-001: silently normalize' "$target" || infra mutation_not_applied
  run_go "$root" build -o "$harness_bin" ./cmd/aur471accept >"$run_dir/build.log" 2>&1 || { cat "$run_dir/build.log" >&2; infra build_failed; }

  local bad="$run_dir/mut001-bad-sum"
  fixture "$bad" 'weights:
  functionality: 0.50
  reliability: 0.10
  usability: 0.10
  efficiency: 0.10
  maintainability: 0.10
  portability: 0.05
  security: 0.05
  compatibility: 0.05
'
  local out
  out="$("$harness_bin" ac002 "$bad")" || infra mutant_run_failed
  if grep -Fq 'ok' <<<"$out"; then
    fail 'AC-002/MUT-001'
  fi
  grep -Fq 'weights-sum' <<<"$out" || fail 'AC-002/MUT-001/mutation-had-no-effect'
}

# MUT-002: let the policy disable secret redaction. The AC-004 redaction
# case must then survive to "ok" and the nominal assertion goes RED.
mutation_redaction() {
  local root="$run_dir/mut002"
  build_harness "$root" "$harness_bin"
  local target="$root/internal/config/policy/policy.go"
  local anchor='		redaction := key == "redaction"'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-002/anchor-not-unique'
  ANCHOR="$anchor" SKIP1='key == "secret_redaction"' SKIP2='key == "disable_redaction"' awk '
    BEGIN { a = ENVIRON["ANCHOR"]; s1 = ENVIRON["SKIP1"]; s2 = ENVIRON["SKIP2"] }
    {
      if (index($0, a) > 0) { print "\t\tredaction := false // MUT-002: redaction clause never detected"; next }
      if (index($0, s1) > 0) { next }
      if (index($0, s2) > 0) { next }
      print
    }
  ' "$target" >"$target.mut" || infra mutation_rewrite
  mv "$target.mut" "$target"
  grep -Fq 'MUT-002: redaction clause never detected' "$target" || infra mutation_not_applied
  run_go "$root" build -o "$harness_bin" ./cmd/aur471accept >"$run_dir/build.log" 2>&1 || { cat "$run_dir/build.log" >&2; infra build_failed; }

  local hostile="$run_dir/mut002-hostile"
  fixture "$hostile" 'weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
redaction: off
'
  local out
  out="$("$harness_bin" ac004 "$hostile")" || infra mutant_run_failed
  if grep -Fq 'ok' <<<"$out"; then
    fail 'AC-004/MUT-002'
  fi
  grep -Fq 'refused-clause' <<<"$out" || fail 'AC-004/MUT-002/mutation-had-no-effect'
}

# bridge_run executes a real declared selector: copy the lane file into a
# staged module, add a _test.go bridge that calls the exported test
# function, and require `go test` to report an actual `ok` (never
# `[no test files]`).
bridge_run() {
  local lane="$1" file="$2" bridge="$3" call="$4"
  local root="$run_dir/lane-$lane"
  stage "$root"
  mkdir -p "$root/tests/$lane"
  cp "$repo_root/tests/$lane/$file" "$root/tests/$lane/$file"
  chmod -R u+w -- "$root/tests"
  cat >"$root/tests/$lane/aur471_bridge_test.go" <<EOF
package $lane

import "testing"

func $bridge(t *testing.T) { $call(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s "./tests/$lane" -run "^$bridge\$" -count=1 2>&1)"
  rc=$?
  set -e
  ((rc == 0)) || { printf '%s\n' "$out" >&2; fail "selector:$lane:exit:$rc"; }
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$lane:zero-tests"
}

unit_case() { bridge_run unit AUR-471.go TestAUR471UnitBridge TestAUR471; }
integration_case() { bridge_run integration AUR-471.go TestAUR471IntegrationBridge IntegrationAUR471; }
e2e_case() { bash "$repo_root/tests/e2e/AUR-471.sh" E2EAUR471 || exit $?; }

case "$selector" in
  AC-001|AC-002|AC-003|AC-004|all) nominal_case ;;
  AC-002-MUT-001) mutation_sum ;;
  AC-004-MUT-002) mutation_redaction ;;
  TestAUR471) unit_case ;;
  IntegrationAUR471) integration_case ;;
  E2EAUR471) e2e_case ;;
esac

if [[ "$selector" == "all" ]]; then
  unit_case
  integration_case
  e2e_case
fi
exit 0
