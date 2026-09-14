#!/usr/bin/env bash
#
# Acceptance program for card AUR-479, scenarios AC-001..AC-003 plus the
# two skeptical mutations MUT-001/MUT-002.
#
# WHAT THIS PROVES
#
#   internal/config assembles untrusted provider contributions into one
#   context block. Today filter.Redact ran per contribution BEFORE the
#   contributions were concatenated, so a secret split across two
#   providers (each half harmless) was reconstituted verbatim in the final
#   prompt. The fix redacts the assembled block once more; the
#   per-contribution pass is kept as defense in depth.
#
#   AC-001: a registered secret split across two contributions does not
#           appear verbatim in the final prompt.
#   AC-002: a whole secret confined to one contribution is still redacted,
#           and the per-contribution pass is load-bearing.
#   AC-003: a secret-shaped span that only exists across the boundary is
#           redacted without destroying the legitimate prose around it.
#
#   MUT-001 removes the assembled-block redaction  -> AC-001 goes RED.
#   MUT-002 removes the per-contribution redaction  -> AC-002 goes RED.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-479'
readonly scenario='AC-001..AC-003'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|TestAUR479|IntegrationAUR479|E2EAUR479|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(tests/unit/AUR-479.go tests/integration/AUR-479.go tests/e2e/AUR-479.sh)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(go.mod go.sum internal/config pkg/types internal/llm internal/security/redaction)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a479.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
run_go() { local dir="$1"; shift; ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" ); }

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum internal/config pkg/types internal/llm internal/security/redaction
  chmod -R u+w -- "$root"
}

write_harness() {
  local root="$1"
  mkdir -p "$root/cmd/aur479harness"
  cat >"$root/cmd/aur479harness/main.go" <<'EOF'
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type capture struct{ got string }

func (c *capture) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	c.got = prompt
	return llm.Response{Text: `{"issues":[]}`}, nil
}
func (c *capture) Tokens(s string) (int, error) { return len(s), nil }
func (c *capture) Name() string                 { return "harness-fake" }

type textProvider struct {
	name string
	text string
}

func (p textProvider) Name() string { return p.name }
func (p textProvider) Provide(context.Context, []string) (string, error) {
	return p.text, nil
}

func promptFor(providers []config.ContextProvider, filter *redaction.Filter) (string, error) {
	base := &capture{}
	wrapped, err := config.WrapProvider(context.Background(), base, providers, []string{"changed.go"}, filter)
	if err != nil {
		return "", err
	}
	if _, err := wrapped.Complete("BASE", llm.Options{}); err != nil {
		return "", err
	}
	return base.got, nil
}

func ac001() error {
	const secret = "AURUM-provider-split-canary-9f3a2b7c"
	prompt, err := promptFor([]config.ContextProvider{
		textProvider{name: "provider-a", text: secret[:12]},
		textProvider{name: "provider-b", text: secret[12:]},
	}, redaction.NewFilter(secret))
	if err != nil {
		return err
	}
	if containsIgnoringNewlines(prompt, secret) {
		return fmt.Errorf("split secret reconstituted: %q", prompt)
	}
	if !strings.Contains(prompt, redaction.Marker) {
		return fmt.Errorf("assembled redaction left no marker: %q", prompt)
	}
	return nil
}

// containsIgnoringNewlines reports whether needle appears in haystack once
// line breaks are removed -- the two reconstituted halves of a split
// secret are adjacent only after the newline join is ignored.
func containsIgnoringNewlines(haystack, needle string) bool {
	return strings.Contains(strings.NewReplacer("\n", "", "\r", "").Replace(haystack), needle)
}

func ac002() error {
	const secret = "AURUM-whole-contribution-canary-1a2b3c4d"
	prompt, err := promptFor([]config.ContextProvider{
		textProvider{name: "provider-a", text: "background before " + secret + " background after"},
	}, redaction.NewFilter(secret))
	if err != nil {
		return err
	}
	if strings.Contains(prompt, secret) {
		return fmt.Errorf("whole secret survived: %q", prompt)
	}
	if !strings.Contains(prompt, redaction.Marker) {
		return fmt.Errorf("no marker for whole secret: %q", prompt)
	}
	// Per-contribution redaction is load-bearing: overlapping registered
	// values must be contained inside their own contribution before the
	// join. Removing that pass leaves "[REDACTED]ghi" instead of
	// "abc\n[REDACTED]" (MUT-002).
	overlap, err := promptFor([]config.ContextProvider{
		textProvider{name: "provider-a", text: "abc"},
		textProvider{name: "provider-b", text: "defghi"},
	}, redaction.NewFilter("abcdef", "defghi"))
	if err != nil {
		return err
	}
	if !strings.Contains(overlap, "abc\n"+redaction.Marker) {
		return fmt.Errorf("per-contribution redaction not applied before assembly: %q", overlap)
	}
	return nil
}

func ac003() error {
	prompt, err := promptFor([]config.ContextProvider{
		textProvider{name: "prose-a", text: `please read note: secret="`},
		textProvider{name: "prose-b", text: `not-a-credential" and then continue.`},
	}, redaction.NewFilter("unrelated-registered-value"))
	if err != nil {
		return err
	}
	if strings.Contains(prompt, "not-a-credential") {
		return fmt.Errorf("boundary secret-shaped span not redacted: %q", prompt)
	}
	if !strings.Contains(prompt, "please read note:") || !strings.Contains(prompt, "and then continue.") {
		return fmt.Errorf("legitimate prose destroyed around the span: %q", prompt)
	}
	return nil
}

func main() {
	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	var err error
	switch mode {
	case "ac001":
		err = ac001()
	case "ac002":
		err = ac002()
	case "ac003":
		err = ac003()
	case "all":
		err = ac001()
		if err == nil {
			err = ac002()
		}
		if err == nil {
			err = ac003()
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown mode")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", mode, err)
		os.Exit(1)
	}
	fmt.Println(mode + "-ok")
}
EOF
}

build_harness() {
  local root="$1" bin="$2"
  run_go "$root" build -o "$bin" ./cmd/aur479harness >"$root/build.log" 2>&1 || { cat "$root/build.log" >&2; infra build_failed; }
}

nominal_case() {
  local root="$run_dir/root-nominal"
  stage_source "$root"
  write_harness "$root"
  local bin="$root/harness"
  build_harness "$root" "$bin"
  local mode
  for mode in ac001 ac002 ac003; do
    local out
    out="$("$bin" "$mode")" || fail "$mode"
    [[ "$out" == "$mode-ok" ]] || fail "unexpected-output:$out"
  done
}

# mutate_remove_line deletes every line containing the given substring.
# Used to apply each mutation to a COPY of the staged source; the
# repository itself is never touched.
mutate_remove_line() {
  local file="$1" needle="$2"
  grep -Fq "$needle" "$file" || infra "anchor-not-found:$needle"
  awk -v n="$needle" 'index($0, n) == 0' "$file" >"$file.mut"
  mv "$file.mut" "$file"
  if grep -Fq "$needle" "$file"; then
    infra "mutation-not-applied:$needle"
  fi
}

# MUT-001: redact only per contribution, never the assembled block.
# AC-001's split secret must then be reconstituted and the harness goes RED.
mutation_001() {
  local root="$run_dir/root-mut1"
  stage_source "$root"
  mutate_remove_line "$root/internal/config/provider.go" 'assembled = filter.Redact(assembled)'
  write_harness "$root"
  local bin="$root/harness"
  build_harness "$root" "$bin"
  local out rc
  set +e
  out="$("$bin" ac001 2>&1)"
  rc=$?
  set -e
  ((rc != 0)) || fail MUT-001
  grep -Fq 'split secret reconstituted' <<<"$out" || fail 'MUT-001/mutation-had-no-effect'
}

# MUT-002: remove the per-contribution redaction, trusting only the
# assembled block. AC-002 must then go RED.
mutation_002() {
  local root="$run_dir/root-mut2"
  stage_source "$root"
  mutate_remove_line "$root/internal/config/provider.go" 'text = filter.Redact(text)'
  write_harness "$root"
  local bin="$root/harness"
  build_harness "$root" "$bin"
  local out rc
  set +e
  out="$("$bin" ac002 2>&1)"
  rc=$?
  set -e
  ((rc != 0)) || fail MUT-002
  grep -Fq 'per-contribution redaction not applied before assembly' <<<"$out" || fail 'MUT-002/mutation-had-no-effect'
}

bridge_run() {
  local pkg="$1" fn="$2" bridge="$3" label="$4"
  local root="$run_dir/root-$pkg"
  stage_source "$root"
  copy "$root" "tests/$pkg/AUR-479.go"
  chmod -R u+w -- "$root/tests"
  cat >"$root/tests/$pkg/aur479_bridge_test.go" <<EOF
package $pkg

import "testing"

func $bridge(t *testing.T) { $fn(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s "./tests/$pkg" -run "^$bridge\$" -count=1 2>&1)"
  rc=$?
  set -e
  ((rc == 0)) || { printf '%s\n' "$out" >&2; fail "selector:$label:exit:$rc"; }
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$label:zero-tests"
}

unit_case() { bridge_run unit TestAUR479 TestAUR479UnitBridge TestAUR479; }
integration_case() { bridge_run integration IntegrationAUR479 TestAUR479IntegrationBridge IntegrationAUR479; }

e2e_case() {
  [[ -f "$repo_root/tests/e2e/AUR-479.sh" ]] || infra "missing-input:tests/e2e/AUR-479.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-479.sh" E2EAUR479
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

case "$selector" in
  MUT-001) mutation_001 ;;
  MUT-002) mutation_002 ;;
  TestAUR479) unit_case ;;
  IntegrationAUR479) integration_case ;;
  E2EAUR479) e2e_case ;;
  all)
    nominal_case
    mutation_001
    mutation_002
    unit_case
    integration_case
    e2e_case
    ;;
  AC-001|AC-002|AC-003) nominal_case ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
