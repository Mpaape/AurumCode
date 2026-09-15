#!/usr/bin/env bash
#
# E2E program for card AUR-471, selector E2EAUR471.
#
# WHAT THIS PROVES, AND WHY IT IS NOT THE ACCEPTANCE AGAIN
#
#   tests/acceptance/AUR-471.sh::AC-001 runs the same package through a
#   harness in the card's own staging. This program builds a standalone Go
#   harness directly over internal/config/policy against REAL files on a
#   REAL temp filesystem -- no git, no model -- and asserts the end-to-end
#   composition a caller like cmd/aurumcode performs: load the declared
#   weights file, weight the review's ISO scores by it, render the review
#   with the per-characteristic and aggregate policy view, keep the
#   zero-config output byte-identical to a plain json.Marshal, and refuse a
#   real policy that tries to switch secret redaction off.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-471'
readonly scenario='E2E'
selector="${1:-E2EAUR471}"
case "$selector" in
  E2EAUR471) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum internal/config/policy pkg/types; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e471.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

root="$run_dir/root"
cp "$repo_root/go.mod" "$repo_root/go.sum" "$root/"
mkdir -p "$root/internal/config/policy" "$root/pkg/types"
cp -R "$repo_root/internal/config/policy/." "$root/internal/config/policy/"
cp -R "$repo_root/pkg/types/." "$root/pkg/types/"
chmod -R u+w -- "$root"

# Two REAL repositories-under-review: one with declared, shifted weights;
# one with no .aurumcode file at all (zero-config).
declared="$run_dir/declared"
mkdir -p "$declared/.aurumcode"
cat >"$declared/.aurumcode/iso25010-weights.yml" <<'EOF'
weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.10
  portability: 0.08
  security: 0.25
  compatibility: 0.05
EOF
zero_repo="$run_dir/zero"
mkdir -p "$zero_repo"
hostile="$run_dir/hostile"
mkdir -p "$hostile/.aurumcode"
cat >"$hostile/.aurumcode/iso25010-weights.yml" <<'EOF'
weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
redact_secrets: false
EOF

mkdir -p "$root/cmd/aur471e2e"
cat >"$root/cmd/aur471e2e/main.go" <<'EOF'
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
	case "declared":
		p, err := policy.Load(os.Args[2])
		must(err)
		if p == nil {
			die("declared policy missing")
		}
		ev, err := policy.Evaluate(p, scores())
		must(err)
		if ev == nil || !ev.Enabled || len(ev.PerCharacteristic) != 8 {
			die("evaluation incomplete")
		}
		out, err := policy.Render(&types.ReviewResult{Verdict: "pass", Summary: "ok"}, ev)
		must(err)
		var decoded map[string]any
		must(json.Unmarshal(out, &decoded))
		if decoded["iso_policy"] == nil {
			die("rendered review must carry iso_policy")
		}
		fmt.Printf("aggregate=%d\n", ev.Aggregate)
	case "zero":
		p, err := policy.Load(os.Args[2])
		must(err)
		if p != nil {
			die("absent file must be nil policy")
		}
		ev, err := policy.Evaluate(p, scores())
		must(err)
		result := &types.ReviewResult{Verdict: "pass", Summary: "ok"}
		got, err := policy.Render(result, ev)
		must(err)
		want, err := json.Marshal(result)
		must(err)
		if string(got) != string(want) {
			die("zero-config render is not byte-identical")
		}
		fmt.Println("zero-config-identical")
	case "hostile":
		_, err := policy.Load(os.Args[2])
		if err == nil {
			die("policy disabling redaction must be refused")
		}
		fmt.Println("refused")
	default:
		fmt.Fprintln(os.Stderr, "unknown mode")
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(4)
}
EOF

bin="$run_dir/aur471e2e"
if ! (cd "$root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aur471e2e) >"$run_dir/build.log" 2>&1; then
  cat "$run_dir/build.log" >&2
  infra build_failed
fi

set +e
decl_out="$(ulimit -v 8388608; GOMEMLIMIT=2GiB "$bin" declared "$declared" 2>"$run_dir/decl.err")"; decl_rc=$?
zero_out="$(ulimit -v 8388608; GOMEMLIMIT=2GiB "$bin" zero "$zero_repo" 2>"$run_dir/zero.err")"; zero_rc=$?
host_out="$(ulimit -v 8388608; GOMEMLIMIT=2GiB "$bin" hostile "$hostile" 2>"$run_dir/host.err")"; host_rc=$?
set -e

(( decl_rc == 0 )) || { cat "$run_dir/decl.err" >&2; fail "declared-exit:$decl_rc"; }
[[ "$decl_out" == "aggregate=65" ]] || fail "declared-aggregate:$decl_out"

(( zero_rc == 0 )) || { cat "$run_dir/zero.err" >&2; fail "zero-exit:$zero_rc"; }
[[ "$zero_out" == "zero-config-identical" ]] || fail "zero-config:$zero_out"

(( host_rc == 0 )) || { cat "$run_dir/host.err" >&2; fail "hostile-exit:$host_rc"; }
[[ "$host_out" == "refused" ]] || fail "hostile-not-refused:$host_out"

exit 0
