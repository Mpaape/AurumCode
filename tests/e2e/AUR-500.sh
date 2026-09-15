#!/usr/bin/env bash
#
# E2E program for card AUR-500, selector E2EAUR500.
#
# WHAT THIS PROVES, AND WHY IT IS NOT THE ACCEPTANCE AGAIN
#
#   This program builds a standalone Go harness over the real public API of
#   internal/reviewprofile against a REAL .aurumcode/config.yml on a REAL temp
#   filesystem -- no git, no LLM fixture -- and asserts the end-to-end
#   composition a caller like cmd/aurumcode performs: read the repository
#   config, layer the review flag on top, resolve the profile, and confirm the
#   resolution declares the entered profile while a hostile definition cannot
#   disable the deterministic security pass.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-500'
readonly scenario='E2E'
selector="${1:-E2EAUR500}"
case "$selector" in
  E2EAUR500) ;;
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

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e500.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

root="$run_dir/root"
for input in "${read_inputs[@]}"; do
  if [[ -d "$repo_root/$input" ]]; then
    mkdir -p "$root/$input"
    chmod -R u+w -- "$root/$input" >/dev/null 2>&1 || true
    cp -R "$repo_root/$input/." "$root/$input/"
  else
    mkdir -p "$root/$(dirname "$input")"
    cp "$repo_root/$input" "$root/$input"
  fi
done
chmod -R u+w -- "$root"

# Repository under review: a real config naming performance.
repo="$run_dir/repo"
mkdir -p "$repo/.aurumcode"
printf 'review:\n  profile: performance\n' >"$repo/.aurumcode/config.yml"

mkdir -p "$root/cmd/aur500e2e"
cat >"$root/cmd/aur500e2e/main.go" <<'EOF'
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"gopkg.in/yaml.v3"
)

func main() {
	repo := os.Args[1]
	data, err := os.ReadFile(filepath.Join(repo, ".aurumcode", "config.yml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read-config:", err)
		os.Exit(2)
	}
	var cfg struct {
		Review struct {
			Profile string `yaml:"profile"`
		} `yaml:"review"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "parse-config:", err)
		os.Exit(2)
	}
	if cfg.Review.Profile != "performance" {
		fmt.Fprintln(os.Stderr, "config-profile-not-read")
		os.Exit(3)
	}

	res, err := reviewprofile.Resolve(reviewprofile.Selection{Config: cfg.Review.Profile, Flag: "seguranca"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve:", err)
		os.Exit(4)
	}
	if res.Profile.Name != "seguranca" || !strings.Contains(res.Declared, "seguranca") {
		fmt.Fprintln(os.Stderr, "flag-not-override-or-not-declared")
		os.Exit(5)
	}
	if !res.Effective.SecurityPassEnabled || !res.Effective.RedactionEnabled {
		fmt.Fprintln(os.Stderr, "boundary-weakened")
		os.Exit(6)
	}
	if _, err := reviewprofile.Compile([]byte("name: evil\nsecurity_pass: false\n")); !errors.Is(err, reviewprofile.ErrRefusedClause) {
		fmt.Fprintln(os.Stderr, "hostile-definition-accepted")
		os.Exit(7)
	}
	fmt.Println("e2e-ok")
}
EOF

bin="$run_dir/aur500e2e"
if ! (cd "$root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aur500e2e) >"$run_dir/build.log" 2>&1; then
  cat "$run_dir/build.log" >&2
  infra build_failed
fi

set +e
out="$(ulimit -v 8388608; GOMEMLIMIT=2GiB "$bin" "$repo" 2>"$run_dir/run.err")"
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
  cat "$run_dir/run.err" >&2
  fail "harness-exit:$rc"
fi
[[ "$out" == "e2e-ok" ]] || fail "unexpected-output:$out"

exit 0
