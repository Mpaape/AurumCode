#!/usr/bin/env bash
# AUR-429 AC-001: depois de publicar, uma verificacao abre a pagina inicial,
# segue um link do indice e confirma que o conteudo esperado esta la, em vez
# de apenas confiar que o arquivo foi enviado.
#
# LANES
#   unit         tests/unit/AUR-429.go::TestAUR429 - browserproof.VerifyDocs
#                over hand-built published trees: proof, the refusals, forgery
#                resistance and the verdict contract.
#   integration  tests/integration/AUR-429.go::IntegrationAUR429 - the real
#                chain: cmd/regenerate-docs over tests/fixtures/docs/goproject,
#                sitepublish, VerifyDocs; breaks applied to the GENERATED
#                markdown flip the verdict.
#   e2e          tests/e2e/AUR-429.sh::E2EAUR429 - the declared command as a
#                real process: raw exit codes, JSON verdict, determinism,
#                canary, misuse.
#   regression   go test ./internal/qa/browserproof/... - the only code path
#                this card touches, including its pre-existing corroboration,
#                guard and security suites.
#
# SCOPE, PER THE CARD'S DISPATCH NOTE
#   The sandbox denies network and pins no real browser, so the verification
#   navigates the built site over loopback through the offline scripted
#   driver — the instrument AUR-428's acceptance already names for this card.
#   `--url` records the published location the verdict is about; a live
#   deploy watch is a human action documented in docs/specs/AUR-429.md.
#
# RED / MUT-001
#   Without the implementation the promised verifier does not exist and this
#   accept fails with AUR-429/AC-001/behavior-missing — a deliberate
#   diagnosis, not a compile stack trace (compile failures never count as
#   RED). Under MUT-001 (a page without the expected content gets accepted)
#   the unit, integration and e2e lanes each fail surfacing the
#   AUR-429/AC-001/MUT-001 marker on stderr; restoring the code reproduces
#   the exact GREEN.
#
# The unit/integration lanes are staged into a private writable module under
# /tmp (the sandbox rootfs is read-only), exactly as AUR-424/425/428 do:
# tests/unit and tests/integration in the full repository mix other cards'
# files, so each lane compiles exactly this card's selector plus a generated
# bridge, with this card's owned package copied alongside.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-429 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR429|IntegrationAUR429|E2EAUR429) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Entry points every lane needs; their absence blocks the run.
inputs=(
  go.mod go.sum
  tests/unit/AUR-429.go
  tests/integration/AUR-429.go
  tests/e2e/AUR-429.sh
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

# The read closure of the integration/e2e chain (cmd/regenerate-docs and its
# local imports, plus the fixture). Not this card's write set: when the
# runner did not materialize them the accept is blocked, not failed.
closure=(
  cmd/regenerate-docs
  internal/documentation/extractors
  internal/documentation/incremental
  internal/documentation/normalizer
  internal/documentation/site
  internal/documentation/welcome
  internal/llm
  internal/pipeline
  tests/fixtures/docs/goproject
)
for p in "${closure[@]}"; do
  [[ -e "$repo_root/$p" ]] || infra "closure_not_materialized:$p"
done

# The card's RED: the promised behavior does not exist. These are the
# artifacts the card delivers; checking them BEFORE any go compile keeps the
# RED a deliberate behavior-missing diagnosis instead of a compile failure.
promised=(
  internal/qa/browserproof/verify.go
  internal/qa/browserproof/sitepublish/publish.go
  internal/qa/browserproof/docsverify/main.go
)
for p in "${promised[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "behavior-missing:$p is not delivered"
done

# Offline module cache (yaml.v3 for the generator build); resolved BEFORE
# HOME is redirected, since the default location derives from HOME.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a429.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

# One warm build cache for every lane, so the closure compiles once.
export GOCACHE="$run_dir/cache"

# stage_lane <package-dir> <selector-file> <selector-func>
# Copies the module skeleton, this card's owned package (non-test sources)
# and exactly this card's selector into a scratch tree, then generates the
# bridge _test file that makes the selector runnable.
stage_lane() {
  local pkg_dir="$1" selector_file="$2" selector_func="$3"
  local root="$run_dir/stage-${pkg_dir##*/}"
  mkdir -p "$root/$pkg_dir"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$root/"
  while IFS= read -r -d '' src; do
    local rel="${src#"$repo_root/"}"
    mkdir -p "$root/$(dirname "$rel")"
    cp "$src" "$root/$rel"
  done < <(find "$repo_root/internal/qa/browserproof" -type f -name '*.go' ! -name '*_test.go' -print0)
  cp "$repo_root/$selector_file" "$root/$selector_file"
  printf 'package %s\nimport "testing"\nfunc TestAUR429Bridge(t *testing.T){ %s(t) }\n' \
    "${pkg_dir##*/}" "$selector_func" >"$root/$pkg_dir/aur429_bridge_test.go"
  printf '%s' "$root"
}

# go_lane <staged-root> <package-dir>
# Compiles and executes the staged selector. AURUMCODE_ROOT points the lane
# at the real repository so the integration chain builds the real generator.
go_lane() {
  local root="$1" pkg_dir="$2" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root" && AURUMCODE_ROOT="$repo_root" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$GOCACHE" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "./$pkg_dir" -run '^TestAUR429Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    # Every distinct card token the lane emitted is surfaced on stderr, so a
    # mutation run registers its AUR-429/AC-001/MUT-001 marker alongside the
    # behavior token instead of hiding it behind the first match.
    detail="$(grep -o "$card/$scenario/[A-Za-z0-9/_:.-]*" <<<"$out" | sort -u | head -n 4 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    grep -q "$card/$scenario/infrastructure/" <<<"$out" && infra "selector:$pkg_dir"
    fail "selector-exit:$pkg_dir:$rc"
  fi
  # A lane that compiled or selected nothing proves nothing.
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || fail "zero-tests:$pkg_dir"
  ! grep -Fq '[no test files]' <<<"$out" || fail "no-test-files:$pkg_dir"
  grep -Fq -- '--- PASS: TestAUR429Bridge' <<<"$out" || fail "selector-did-not-run:$pkg_dir"
}

unit_lane() {
  local root; root="$(stage_lane tests/unit tests/unit/AUR-429.go TestAUR429)"
  go_lane "$root" tests/unit
}

integration_lane() {
  local root; root="$(stage_lane tests/integration tests/integration/AUR-429.go IntegrationAUR429)"
  go_lane "$root" tests/integration
}

# The E2E's own exit status is propagated rather than flattened: 69 means its
# inputs were not materialized (blocked/inconclusive), 64 a selector misuse;
# collapsing either into exit 1 would report missing infrastructure as a
# failed behavior, the opposite verdict.
e2e_case() {
  local rc
  set +e
  (cd "$repo_root" && GOCACHE="$GOCACHE" bash tests/e2e/AUR-429.sh E2EAUR429)
  rc=$?
  set -e
  (( rc == 0 )) && return 0
  (( rc == 69 || rc == 64 )) && exit "$rc"
  fail "selector:E2EAUR429:$rc"
}

# The package this card evolved. In a full checkout (host, reviewer) the
# whole pre-existing suite runs: corroboration, ledger, driver lock, symlink
# and secret guards. Inside the sealed profile only this card's read closure
# is materialized, and the legacy suite reads tests/fixtures/AUR-423 — a
# fixture owned by AUR-423's card, outside this card's read_paths — so there
# the lane runs every test of the surface this card added (the VerifyDocs and
# DocsVerify contract tests compile against the full package) plus the full
# sitepublish and docsverify suites, and says so. Both branches demand real
# executed assertions.
regression_lane() {
  local out rc packages=(./internal/qa/browserproof/...) run_filter=''
  if [[ ! -e "$repo_root/tests/fixtures/AUR-423" ]]; then
    printf 'regression: tests/fixtures/AUR-423 is outside this card'\''s read closure; running the AUR-429 surface\n'
    packages=(./internal/qa/browserproof ./internal/qa/browserproof/sitepublish ./internal/qa/browserproof/docsverify)
    run_filter='^Test(VerifyDocs|DocsVerify|Publish|Run)'
  fi
  set +e
  out="$( ulimit -v 8388608
    cd "$repo_root" && \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$GOCACHE" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
    go test -timeout 300s -v -count=1 -p 1 ${run_filter:+-run "$run_filter"} "${packages[@]}" 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "regression:$rc"
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count >= 3 )) || fail "regression:zero-packages:$ok_count"
  grep -Fq -- '--- PASS: TestVerifyDocsProvesASiteThatOpensAndNavigates' <<<"$out" \
    || fail 'regression:new-surface-did-not-run'
}

# The card's Documentation clause: the spec records the command, its options,
# the exit codes, an executable example and the offline restriction.
spec_case() {
  local spec="$repo_root/docs/specs/AUR-429.md"
  [[ -s "$spec" ]] || fail 'behavior-missing:docs/specs/AUR-429.md absent or empty'
  grep -Fq 'docs verify --url' "$spec" || fail 'behavior-missing:spec never names the declared command'
  grep -Fq 'docsverify' "$spec" || fail 'behavior-missing:spec never names the shipped verifier'
  grep -Fq 'Exit codes' "$spec" || fail 'behavior-missing:spec never documents the exit codes'
  for code in 0 1 64 69; do
    grep -Fq "\`$code\`" "$spec" || fail "behavior-missing:spec never documents exit $code"
  done
}

case "$selector" in
  AC-001)
    # Behavior lanes run before the regression lane on purpose: under the
    # card's MUT-001 both would fail, but only the behavior lanes emit the
    # AUR-429/AC-001/MUT-001 marker the card requires the accept to register.
    unit_lane
    integration_lane
    e2e_case
    regression_lane
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","command":"docsverify --url https://usuario.github.io/projeto","driver":"scripted-offline","lanes":"unit,integration,e2e,regression,spec"}\n' \
      "$card" "$scenario"
    ;;
  TestAUR429) unit_lane ;;
  IntegrationAUR429) integration_lane ;;
  E2EAUR429) e2e_case ;;
esac
