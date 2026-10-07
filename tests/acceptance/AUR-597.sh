#!/usr/bin/env bash
# AUR-597 acceptance: the batch self review concludes. The review prompt
# explains the redaction marker (never a secret by itself), the shell of the
# two touched workflows lives in versioned scripts/ci scripts, semgrep parses
# both workflows, and the local diff fallback fails closed when the base is
# not in the checkout.
#
# Selectors:
#   all        AC-001, AC-002, AC-004, MUT-001, MUT-002, and AC-003 when a
#              semgrep binary and the rule registry are reachable (else the
#              line says so: the proof runs in the product image, see
#              docs/specs/AUR-597.md)
#   AC-001     the rendered review prompt carries the redaction-marker rule
#   AC-002     no `sh -c`, no literal `run: |` block and no `&&` chain in the
#              two workflows; their scripts exist, are referenced, pass bash -n
#   AC-003     semgrep (product rule packs) concludes on both workflows with
#              no parse error
#   AC-004     base absent from the checkout: --pr fails closed
#   MUT-001    removing the template rule turns AC-001 RED
#   MUT-002    putting `sh -c` back in a workflow turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-597'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly workflows=(.github/workflows/acceptance-sample.yml .github/workflows/providers-smoke.yml)
readonly scripts=(scripts/ci/acceptance-sample.sh scripts/ci/providers-smoke-detect.sh scripts/ci/providers-smoke.sh)
readonly template='internal/prompt/templates/review.md'
readonly template_rule='- `[REDACTED]` é a máscara que o próprio Aurum aplica antes deste prompt sobre'

for input in go.mod go.sum cmd internal pkg "$template" "${workflows[@]}" "${scripts[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a597.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
  chmod -R u+w -- "$root"
}

# stage_workflows copies the two workflows and their scripts to a fresh root.
stage_workflows() {
  local root="$1" source
  for source in "${workflows[@]}" "${scripts[@]}"; do
    mkdir -p "$root/${source%/*}"
    cp "$repo_root/$source" "$root/$source"
  done
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

run_tests() {
  local name="$1" pkg="$2" test="$3"
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  command -v go >/dev/null 2>&1 || infra missing_go
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "^${test}\$" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  grep -Eq -- "^--- PASS: ${test} " "$log" || { cat "$log" >&2; fail "missing-pass:$test"; }
}

expect_red() {
  local root="$1" log="$2" pkg="$3" test="$4"
  if go_test "$root" "$log" "^${test}\$" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,3p' >&2
}

# check_workflows asserts AC-002 over the workflows and scripts under root.
check_workflows() {
  local root="$1" wf s
  for wf in "${workflows[@]}"; do
    if grep -nE '(^|[^[:alnum:]_])sh[[:space:]]+-c([[:space:]]|$)' "$root/$wf" >&2; then
      fail "sh-c-in-workflow:$wf"
    fi
    if grep -nE '^[[:space:]]*run:[[:space:]]*\|' "$root/$wf" >&2; then
      fail "literal-run-block:$wf"
    fi
    if grep -nE '&&' "$root/$wf" >&2; then
      fail "shell-chain-in-workflow:$wf"
    fi
  done
  for s in "${scripts[@]}"; do
    bash -n "$root/$s" || fail "bash-n:$s"
    grep -Fq -- "$s" "$root/${workflows[0]}" "$root/${workflows[1]}" || fail "script-not-referenced:$s"
  done
}

run_ac001() {
  run_tests AC-001 ./internal/prompt/ TestReviewPromptExplainsTheRedactionMarker
  printf '%s/AC-001/pass\n' "$card"
}

run_ac002() {
  check_workflows "$repo_root"
  printf '%s/AC-002/pass\n' "$card"
}

# semgrep_ready says whether a semgrep binary can reach the product's rule
# packs (registry); the sealed acceptance has neither.
semgrep_ready() {
  command -v semgrep >/dev/null 2>&1 || return 1
  [[ "${AURUM597_SEMGREP:-}" == on ]]
}

run_ac003() {
  semgrep_ready || infra "semgrep-unavailable (set AURUM597_SEMGREP=on in the product image with network)"
  local root="$run_dir/root-ac3" report="$run_dir/semgrep.json"
  stage_workflows "$root"
  ( cd "$root" && semgrep scan --json --quiet --metrics=off --disable-version-check \
      --config p/security-audit --config p/owasp-top-ten "${workflows[@]}" ) >"$report" 2>"$run_dir/semgrep.err" \
    || { cat "$run_dir/semgrep.err" >&2; fail semgrep-failed; }
  python3 - "$report" <<'PY' || fail semgrep-errors
import json, sys
report = json.load(open(sys.argv[1]))
errors = report.get("errors", [])
scanned = report.get("paths", {}).get("scanned", [])
for e in errors:
    print("semgrep error:", e.get("type"), e.get("path", ""), e.get("message", "")[:200], file=sys.stderr)
if len(scanned) != 2:
    print("scanned paths:", scanned, file=sys.stderr)
    sys.exit(1)
sys.exit(1 if errors else 0)
PY
  printf '%s/AC-003/pass (semgrep, 2 workflows, 0 errors)\n' "$card"
}

run_ac004() {
  run_tests AC-004 ./cmd/aurumcode/ TestTooLargeDiffWithBaseAbsentFromCheckoutFailsClosed
  printf '%s/AC-004/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1" file
  stage "$root"
  file="$root/$template"
  grep -Fq -- "$template_rule" "$file" || infra template-rule-missing
  # Drop the rule's bullet: its first line and the indented continuation.
  awk -v rule="$template_rule" '
    index($0, rule) == 1 { skip = 1; next }
    skip && /^  / { next }
    { skip = 0; print }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  ! grep -Fq -- 'scanner de segredos sobre o conteúdo bruto' "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" ./internal/prompt/ TestReviewPromptExplainsTheRedactionMarker
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" file out
  stage_workflows "$root"
  file="$root/${workflows[0]}"
  grep -Fq -- 'sh scripts/ci/acceptance-sample.sh' "$file" || infra anchor-missing
  sed -i "s#sh scripts/ci/acceptance-sample.sh#sh -c 'sh scripts/ci/acceptance-sample.sh'#" "$file"
  if out="$( (check_workflows "$root") 2>&1 )"; then
    fail mutation-survived
  fi
  grep -Fq -- 'sh-c-in-workflow' <<<"$out" || { printf '%s\n' "$out" >&2; fail mutation-not-behavioral; }
  printf '%s\n' "$out" | sed -n '1,2p' >&2
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac001
    run_ac002
    if semgrep_ready; then run_ac003; else printf '%s/AC-003/not-run-here (no semgrep with registry; proven in the product image, docs/specs/AUR-597.md)\n' "$card"; fi
    run_ac004
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
