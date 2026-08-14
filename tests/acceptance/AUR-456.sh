#!/usr/bin/env bash
# AUR-456 AC-001: nobody who reads this repository is told about a
# capability the code does not have, and no dead copy of a rule or prompt
# survives beside the version actually embedded in the binary -- because the
# 2026-08-14 audit's fourteen dead paths
# (.board/cards/ready/AUR-456.md, "Achados medidos") are gone, and the two
# live exceptions (the welcome-page override, the live rules catalog) are
# proven to still be exactly what the source reads.
#
# SCOPE
#   This runs inside `bootstrap-readonly-v1`: no network, and it never
#   executes `aurumcode` or `regenerate-docs`. What THIS card accepts is
#   STATIC verification: filesystem existence over the fourteen dead paths,
#   plus a direct read of the two live artifacts the deletions must not
#   disturb.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/
# 440/445)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test` needs a
#   writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp and bridge the exported selector functions
#   into `_test.go` files there.
#
# WHY THE DEAD PATHS ARE STAGED OPTIONALLY, NOT AS REQUIRED ENTRYPOINTS
#   Unlike a sibling card whose fix changes file CONTENT (its inputs always
#   exist, before and after), this card's fix is DELETION: the fourteen dead
#   paths exist pre-fix (RED) and must not exist post-fix (GREEN). Staging
#   mirrors whatever the real checkout currently has -- copies a dead path
#   when present, skips it when absent -- and never fails either way; only
#   the Unit/Integration/E2E lanes decide whether presence is acceptable.
#
# MUT-001
#   Reintroducing .aurumcode/rules/security.yml (the card's own named
#   priority: "reintroduzir uma copia morta de regra") must fail the
#   acceptance because it diverges from the live catalog. This script's
#   `MUT-001` selector proves it: it stages a SEPARATE scratch copy from the
#   real (already-fixed) checkout, writes the exact pre-deletion bytes of
#   .aurumcode/rules/security.yml into ONLY that copy, and requires every
#   lane to fail with the AUR-456/AC-001/MUT-001 label. The tracked tree is
#   never touched.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-456 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR456|IntegrationAUR456|E2EAUR456|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Required entrypoints: artifacts that must exist both before and after this
# card's fix. Their absence is a real infrastructure gap, never this card's
# RED.
required_inputs=(
  go.mod go.sum
  internal/documentation/welcome/generator.go
  internal/review/rules/security.yml
  scripts/action-entrypoint.sh
  .aurumcode/prompts/documentation/welcome-page.md
  docs/specs/AUR-456.md
  tests/unit/AUR-456.go
  tests/integration/AUR-456.go
  tests/e2e/AUR-456.sh
)
for p in "${required_inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

# Optional: the fourteen dead paths this card deletes. Present pre-fix
# (RED), absent post-fix (GREEN); staged when present, skipped when absent,
# never gating entry.
optional_dead_files=(
  .aurumcode/prompts/changelog-generation.md
  .aurumcode/prompts/review.md
  .aurumcode/prompts/documentation.md
  .aurumcode/prompts/test.md
  .aurumcode/prompts/summary.md
  .aurumcode/prompts/documentation-generation.md
  index.md
  _config.yml
  pages-fix.md
  test-jekyll.sh
)
optional_dead_dirs=(
  .aurumcode/rules
  configs
  _api
  .github/actions/aurumcode-docs
)

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a456.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  local p
  for p in "${required_inputs[@]}"; do
    mkdir -p "$dest/$(dirname "$p")"
    cp "$repo_root/$p" "$dest/$p"
  done
  for p in "${optional_dead_files[@]}"; do
    [[ -f "$repo_root/$p" ]] || continue
    mkdir -p "$dest/$(dirname "$p")"
    cp "$repo_root/$p" "$dest/$p"
  done
  for p in "${optional_dead_dirs[@]}"; do
    [[ -d "$repo_root/$p" ]] || continue
    mkdir -p "$dest/$(dirname "$p")"
    cp -r "$repo_root/$p" "$dest/$p"
  done
}

stage "$root"

printf 'package unit\nimport "testing"\nfunc TestAUR456Bridge(t *testing.T){ TestAUR456(t) }\n' \
  >"$root/tests/unit/aur456_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR456Bridge(t *testing.T){ IntegrationAUR456(t) }\n' \
  >"$root/tests/integration/aur456_bridge_test.go"

go_lane() {
  # One offline, bounded invocation compiles and executes the requested
  # package's real assertions against the staged artifacts. root_override
  # lets MUT-001 point the same lane at a different, mutated staging area;
  # mutation_marker, set only by the MUT-001 case, is forwarded as
  # AUR456_MUTATION so the absence check can tell a deliberate mutation
  # apart from this card's own pre-fix RED (see tests/unit/AUR-456.go).
  # aur456_last_output (intentionally not `local`) lets the MUT-001 case
  # inspect the captured text after this function returns.
  local pkg="$1" root_override="${2:-$root}" mutation_marker="${3:-}" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root_override" && AURUMCODE_ROOT="$root_override" AUR456_MUTATION="$mutation_marker" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR456Bridge$' 2>&1)"
  rc=$?
  set -e
  aur456_last_output="$out"
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    return "$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || { fail "zero-tests:$pkg"; }
  ! grep -Fq '[no test files]' <<<"$out" || { fail "no-test-files:$pkg"; }
  grep -Fq -- '--- PASS: TestAUR456Bridge' <<<"$out" || { fail "selector-did-not-run:$pkg"; }
  return 0
}

e2e_case() {
  # mutation_marker mirrors go_lane's: forwarded as AUR456_MUTATION so
  # tests/e2e/AUR-456.sh's own, independent check applies the same
  # RED-vs-MUT-001 distinction.
  local root_override="${1:-$root}" mutation_marker="${2:-}" out rc
  set +e
  out="$(AURUMCODE_ROOT="$root_override" AUR456_MUTATION="$mutation_marker" bash "$repo_root/tests/e2e/AUR-456.sh" E2EAUR456 2>&1)"
  rc=$?
  set -e
  aur456_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

spec_case() {
  # The card's Documentation clause, plus the selector-naming deviation this
  # card records (see docs/specs/AUR-456.md's closing note and
  # tests/unit/AUR-456.go's header): the spec must name the deviation so a
  # reviewer checking "declared selectors" does not read TestAUR456 as an
  # unexplained mismatch against the card's literal TestAUR445 text.
  [[ -s "$repo_root/docs/specs/AUR-456.md" ]] || fail 'behavior-missing:docs/specs/AUR-456.md absent or empty'
  grep -Fq 'TestAUR456' "$repo_root/docs/specs/AUR-456.md" || fail 'behavior-missing:spec never names the TestAUR456 selector'
  grep -Fq 'AUR-445' "$repo_root/docs/specs/AUR-456.md" || fail 'behavior-missing:spec never records the AUR-445 template-residue naming note'
}

run_ac001() {
  local root_override="$1" label_prefix="$2"
  go_lane ./tests/unit "$root_override" || fail "${label_prefix}selector-exit:unit"
  go_lane ./tests/integration "$root_override" || fail "${label_prefix}selector-exit:integration"
  local rc
  set +e
  e2e_case "$root_override"
  rc=$?
  set -e
  if (( rc != 0 )); then
    (( rc == 69 || rc == 64 )) && exit "$rc"
    fail "${label_prefix}selector-exit:e2e:$rc"
  fi
}

case "$selector" in
  AC-001)
    run_ac001 "$root" ""
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","verification":"static-existence-and-source","dead_paths_checked":14}\n' "$card" "$scenario"
    ;;
  TestAUR456) go_lane ./tests/unit "$root" ;;
  IntegrationAUR456) go_lane ./tests/integration "$root" ;;
  E2EAUR456) e2e_case "$root" ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    # Mutate ONLY the scratch copy: reintroduce the exact pre-deletion bytes
    # of .aurumcode/rules/security.yml, the card's own named priority.
    mkdir -p "$mut_root/.aurumcode/rules"
    cat >"$mut_root/.aurumcode/rules/security.yml" <<'AUR456_DEAD_RULES_EOF'
# Security Review Rules

rules:
  - id: security/sql-injection
    title: SQL Injection Vulnerability
    description: Potential SQL injection vulnerability detected
    severity: error
    category: security
    tags: [sql, injection, database]

  - id: security/xss
    title: Cross-Site Scripting (XSS)
    description: Potential XSS vulnerability detected
    severity: error
    category: security
    tags: [xss, web, injection]

  - id: security/command-injection
    title: Command Injection
    description: Potential command injection vulnerability
    severity: error
    category: security
    tags: [command, injection, shell]

  - id: security/path-traversal
    title: Path Traversal
    description: Potential path traversal vulnerability
    severity: error
    category: security
    tags: [path, filesystem, traversal]

  - id: security/hardcoded-secrets
    title: Hardcoded Secrets
    description: Hardcoded credentials or API keys detected
    severity: error
    category: security
    tags: [secrets, credentials, keys]

  - id: security/weak-crypto
    title: Weak Cryptography
    description: Use of weak or deprecated cryptographic algorithms
    severity: warning
    category: security
    tags: [crypto, encryption, hash]

  - id: security/insecure-random
    title: Insecure Random Number Generation
    description: Use of insecure random number generator
    severity: warning
    category: security
    tags: [random, crypto, security]

  - id: security/missing-auth
    title: Missing Authentication
    description: Endpoint or function lacks proper authentication
    severity: error
    category: security
    tags: [auth, authentication, access]
AUR456_DEAD_RULES_EOF
    printf 'package unit\nimport "testing"\nfunc TestAUR456Bridge(t *testing.T){ TestAUR456(t) }\n' \
      >"$mut_root/tests/unit/aur456_bridge_test.go"
    printf 'package integration\nimport "testing"\nfunc TestAUR456Bridge(t *testing.T){ IntegrationAUR456(t) }\n' \
      >"$mut_root/tests/integration/aur456_bridge_test.go"

    unit_rc=0; go_lane ./tests/unit "$mut_root" MUT-001 || unit_rc=$?
    (( unit_rc != 0 )) || fail 'MUT-001:unit lane passed on mutated input, expected failure'
    # A nonzero exit alone is not proof of a detected mutation -- an
    # infrastructure failure (build error, missing go) is also nonzero. The
    # captured output must carry the card's own MUT-001 label.
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur456_last_output" \
      || fail 'MUT-001:unit lane failed but never reported the MUT-001 label'

    e2e_rc=0; e2e_case "$mut_root" MUT-001 || e2e_rc=$?
    (( e2e_rc != 0 )) || fail 'MUT-001:e2e lane passed on mutated input, expected failure'
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur456_last_output" \
      || fail 'MUT-001:e2e lane failed but never reported the MUT-001 label'

    printf '%s/%s/MUT-001 confirmed: unit_rc=%d e2e_rc=%d, tracked tree untouched\n' "$card" "$scenario" "$unit_rc" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
