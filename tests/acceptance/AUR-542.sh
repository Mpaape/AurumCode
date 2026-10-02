#!/usr/bin/env bash
#
# Acceptance program for card AUR-542.
#
# WHAT THIS PROVES
#
#   AUR-541 audited every tests/e2e/*.sh that feeds AURUMCODE_LLM_FIXTURE a
#   real value and, beyond the evidence-gate fixture defect it fixed,
#   flagged six scripts that fail for OTHER reasons: AUR-438, AUR-443,
#   AUR-448, AUR-449, AUR-451, AUR-459. This card measured each one's root
#   cause and either fixed the test (it was outdated against a product
#   decision already done and integrated on main -- AUR-490, or a product
#   pivot already integrated on main -- commit 670c7f6) or left it
#   genuinely red with the measured cause recorded in docs/specs/AUR-542.md
#   (AUR-438, AUR-459: real, undersized-for-this-card product gaps). See
#   that spec for the full per-script table.
#
# SELECTORS
#   all             run every scenario below
#   AC-001          every one of the six scripts either exits 0 (fixed) or
#                   exits with the exact measured RED tag this card
#                   documented (genuinely unresolved, origin card reopened)
#   AC-002          docs/specs/AUR-542.md records, for every one of the six
#                   scripts, a cause, a decision, and a before/after pair
#   AC-001-MUT-001  reverting this card's own product fix in
#                   cmd/aurumcode/main.go (the qualitySkipped branch no
#                   longer also prints errNoProviderConfigured's full
#                   text) makes tests/e2e/AUR-448.sh fail again
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-542'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

required_inputs=(
  go.mod go.sum cmd/aurumcode internal/review internal/prompt tests/e2e
  docs/specs/AUR-542.md tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json tests/fixtures/scm/github
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a542.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

# run_e2e runs one e2e script from repo_root. Globals e2e_rc/e2e_out carry
# the result -- never read a function's own exit status through a
# `$(...)` command substitution for this (AUR-541's own review note,
# 97c4d39, B1): the substitution's subshell swallows it.
run_e2e() {
  local name="$1"; shift
  e2e_out="$run_dir/e2e-$name.out"
  set +e
  ( cd "$repo_root" && "$@" bash "tests/e2e/$name.sh" ) >"$e2e_out" 2>&1
  e2e_rc=$?
  set -e
}

# Fixed this card: now exits 0 directly.
readonly -a fixed_scripts=(AUR-443 AUR-448 AUR-449 AUR-451)

# Genuinely red: the origin card is reopened with the measured cause
# (docs/specs/AUR-542.md), not fixed here. The exact tag pinned below is
# the measured signature -- if a future change on cmd/aurumcode or
# internal/review/scope.go makes either script print something else
# (including, silently, exit 0), this selector must notice.
readonly -a red_scripts=(AUR-438 AUR-459)
red_tag() {
  case "$1" in
    AUR-438) printf 'missing_general_finding' ;;
    AUR-459) printf 'gate-open-on-converted-finding' ;;
  esac
}

run_ac001() {
  local any_bad=0 name
  for name in "${fixed_scripts[@]}"; do
    run_e2e "$name"
    if [[ "$e2e_rc" -eq 79 || "$e2e_rc" -eq 69 ]]; then
      cat "$e2e_out" >&2
      infra "e2e-infra:$name:$e2e_rc"
    fi
    if [[ "$e2e_rc" -ne 0 ]]; then
      cat "$e2e_out" >&2
      printf '%s/%s/regressed:%s:%s\n' "$card" "$selector" "$name" "$e2e_rc" >&2
      any_bad=1
    fi
  done
  for name in "${red_scripts[@]}"; do
    run_e2e "$name"
    if [[ "$e2e_rc" -eq 0 ]]; then
      cat "$e2e_out" >&2
      printf '%s/%s/unexpectedly-fixed:%s\n' "$card" "$selector" "$name" >&2
      any_bad=1
      continue
    fi
    if [[ "$e2e_rc" -eq 79 || "$e2e_rc" -eq 69 ]]; then
      cat "$e2e_out" >&2
      infra "e2e-infra:$name:$e2e_rc"
    fi
    if ! grep -Fq "$(red_tag "$name")" "$e2e_out"; then
      cat "$e2e_out" >&2
      printf '%s/%s/different-red-cause:%s\n' "$card" "$selector" "$name" >&2
      any_bad=1
    fi
  done
  [[ "$any_bad" -eq 0 ]]
}

# AC-002: docs/specs/AUR-542.md documents, per script, a cause, a decision
# and a before/after pair -- checked structurally (every script id and the
# three section headers appear), not by re-deriving the prose.
run_ac002() {
  local spec="$repo_root/docs/specs/AUR-542.md"
  local any_bad=0 name
  for name in AUR-438 AUR-443 AUR-448 AUR-449 AUR-451 AUR-459; do
    grep -Fq "$name" "$spec" || { printf '%s/%s/spec-missing-script:%s\n' "$card" "$selector" "$name" >&2; any_bad=1; }
    # Extract this script's own "## AUR-NNN..." section, up to the next
    # "## " heading (or end of file), and require all four parts WITHIN
    # that section -- not merely somewhere in the whole document, which
    # a single shared heading anywhere would satisfy vacuously for every
    # script.
    local section
    section="$(awk -v id="## $name" '
      $0 ~ "^"id { found=1; print; next }
      found && /^## / { exit }
      found { print }
    ' "$spec")"
    [[ -n "$section" ]] || { printf '%s/%s/spec-missing-section:%s\n' "$card" "$selector" "$name" >&2; any_bad=1; continue; }
    for part in 'Causa' 'Decisao' 'Antes' 'Depois'; do
      if ! grep -Fq "$part" <<<"$section"; then
        printf '%s/%s/spec-section-missing-%s:%s\n' "$card" "$selector" "${part,,}" "$name" >&2
        any_bad=1
      fi
    done
  done
  [[ "$any_bad" -eq 0 ]]
}

# AC-001-MUT-001: stage a copy of cmd/aurumcode with this card's own
# product fix (main.go's qualitySkipped branch no longer also prints
# errNoProviderConfigured's full text) reverted, build it, and run
# tests/e2e/AUR-448.sh against that candidate binary (AURUMCODE_BIN). The
# fix's own marker comment makes the mutation precise regardless of the
# file's current line numbers; the OTHER, pre-existing occurrence of the
# same fmt.Fprintf call (the --modelo-named failure path) is left intact.
run_mut001() {
  local root="$run_dir/root-mut001"
  mkdir -p "$root"
  local top
  for top in go.mod go.sum cmd internal pkg tests; do
    [[ -e "$repo_root/$top" ]] || continue
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"

  local target="$root/cmd/aurumcode/main.go"
  [[ -f "$target" ]] || infra 'MUT-001/stage-missing'
  local before after
  before="$(sha256sum "$target" | awk '{print $1}')"

  local tmp="$root/main.go.mutated"
  awk '
    prevmatch == 1 && index($0, "fmt.Fprintf(stderr, \"aurumcode review: %v") > 0 { prevmatch = 0; next }
    { prevmatch = (index($0, "exit code, or stdout.") > 0) ? 1 : 0; print }
  ' "$target" >"$tmp" || infra 'MUT-001/rewrite'
  mv "$tmp" "$target"

  after="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$before" != "$after" ]] || infra 'MUT-001/no-change'

  local mutant_bin="$root/aurumcode-mutant"
  local build_log="$root/build.log"
  if ! ( cd "$root" && go build -o "$mutant_bin" ./cmd/aurumcode ) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    infra 'MUT-001/build-failed'
  fi

  local e2e_out="$run_dir/mut001.out" rc
  set +e
  ( cd "$repo_root" && AURUMCODE_BIN="$mutant_bin" bash tests/e2e/AUR-448.sh ) >"$e2e_out" 2>&1
  rc=$?
  set -e
  if [[ "$rc" -eq 79 || "$rc" -eq 69 ]]; then cat "$e2e_out" >&2; infra 'MUT-001/e2e-infra'; fi
  if [[ "$rc" -eq 0 ]]; then
    cat "$e2e_out" >&2
    return 1 # mutation survived: reverting the fix should have revived the RED
  fi
  grep -Fq 'no_provider_missing_rule_id_field' "$e2e_out" || { cat "$e2e_out" >&2; return 1; }
  return 0
}

case "$selector" in
  AC-001)
    run_ac001 || fail AC-001
    ;;
  AC-002)
    run_ac002 || fail AC-002
    ;;
  AC-001-MUT-001)
    run_mut001 || fail AC-001-MUT-001
    ;;
  all)
    run_ac001 || fail AC-001
    run_ac002 || fail AC-002
    run_mut001 || fail AC-001-MUT-001
    ;;
esac

printf '%s/%s/pass\n' "$card" "$selector"
exit 0
