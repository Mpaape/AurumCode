#!/usr/bin/env bash
# AUR-455 AC-001: a new developer who reaches this repository can compile
# it, run the first code review, run the first documentation generation,
# and find out how to plug into their own pull request -- all without
# reading source -- because `demo.sh` runs all three features offline,
# with no LLM key, and the acceptance below actually EXECUTES it and
# checks it passes, rather than only checking the file exists (see
# .board/cards/ready/AUR-455.md's closing paragraph before "## Non-goals").
#
# SCOPE
#   Runs inside `bootstrap-readonly-v1`: no network, no `git` binary. This
#   is NOT static-only verification like some sibling cards (e.g.
#   tests/acceptance/AUR-445.sh): the Unit and Integration lanes below stay
#   static (text and source), but the E2E lane really builds
#   ./cmd/regenerate-docs and ./cmd/aurumcode and runs demo.sh against
#   committed fixtures (tests/fixtures/repos/git-demo, tests/fixtures/review,
#   tests/fixtures/docs/goproject) -- no network or LLM key needed.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/440/445)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go build`/`go test`
#   need a writable module and build cache, so every Go lane below copies
#   its exact inputs into a private tree under /tmp.
#
# RESOURCE ENVIRONMENT (measured against this card's own bootstrap-readonly-v1
# image during construction; see docs/specs/AUR-455.md)
#   The profile caps memory at 256MB. Go's default build parallelism gets
#   OOM-killed compiling cmd/aurumcode at that cap; GOFLAGS=-p=1 plus
#   GOMAXPROCS=1 keep it under budget (confirmed: ~20s wall, well inside the
#   profile's 120s timeout, building both binaries from a cold GOCACHE).
#
# MUT-001
#   Breaking a command demo.sh's steps (b)/(c) rely on -- the --seguranca
#   flag README.md documents as "the best first command" -- makes demo.sh
#   stop running. This script's MUT-001 selector proves it: it stages a
#   SEPARATE scratch copy, renames ONLY that copy's --seguranca flag
#   registration in cmd/aurumcode/main.go (sed, not a source edit), rebuilds
#   the mutated binary via that copy's own demo.sh, and requires the run to
#   fail with the AUR-455/AC-001/MUT-001 label. The tracked demo.sh and
#   cmd/aurumcode/main.go are never touched. MUT-001 is a separate,
#   deliberate invocation (`bash tests/acceptance/AUR-455.sh MUT-001`),
#   proved manually by the builder during construction -- never automatic
#   inside oci-run's default entrypoint, same convention as every sibling
#   card in this office (see docs/specs/AUR-445.md's identical note).
set -euo pipefail
export LC_ALL=C
readonly card=AUR-455 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR455|IntegrationAUR455|E2EAUR455|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Entrypoints this card's behavior cannot be checked without. demo.sh is
# THIS card's own new deliverable, so its absence is the RED this card
# proves fixed (behavior-missing), never an infrastructure gap.
required_files=(
  go.mod go.sum
  README.md
  docs/getting-started.md
  demo.sh
  docs/specs/AUR-455.md
  tests/unit/AUR-455.go
  tests/integration/AUR-455.go
  tests/e2e/AUR-455.sh
  cmd/aurumcode/main.go
  cmd/regenerate-docs/main.go
  .github/workflows/examples/code-review.yml
  .github/workflows/examples/documentation.yml
  tests/fixtures/repos/git-demo/manifest.json
  tests/fixtures/review/known-problem-response.json
)
for p in "${required_files[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "behavior-missing:entrypoint_missing:$p"
done

# Directory-shaped inputs demo.sh and the compiled binaries need: every
# read_paths root this card declares that is a directory rather than a
# single file.
required_dirs=(
  cmd/aurumcode
  cmd/regenerate-docs
  internal/analyzer
  internal/prompt
  internal/review
  internal/llm
  internal/security/redaction
  internal/git/githubclient
  pkg/types
  internal/documentation/extractors
  internal/documentation/incremental
  internal/documentation/normalizer
  internal/documentation/site
  internal/documentation/welcome
  internal/pipeline
  tests/fixtures/repos/git-demo
  tests/fixtures/review
  tests/fixtures/docs/goproject
)
for d in "${required_dirs[@]}"; do
  [[ -d "$repo_root/$d" ]] || fail "behavior-missing:entrypoint_missing:$d"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a455.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

# stage DEST: copies every input this card's build/run needs into DEST,
# preserving relative layout. Mirrors what oci-run itself would materialize
# from this card's declared paths + read_paths (git-tracked files under
# each root), reimplemented with plain cp because this script runs both
# inside the sandbox and directly in a developer's worktree.
stage() {
  local dest="$1" p d
  mkdir -p "$dest"
  for p in "${required_files[@]}"; do
    mkdir -p "$dest/$(dirname "$p")"
    cp "$repo_root/$p" "$dest/$p"
  done
  for d in "${required_dirs[@]}"; do
    # `cp -r SRC DEST` copies SRC *into* DEST (nesting one level deeper)
    # when DEST already exists -- which it can here, since a file under
    # this same directory (e.g. tests/fixtures/repos/git-demo/manifest.json)
    # may already have created it via the files loop above. The trailing
    # `/.` on SRC forces content-merge semantics regardless of whether DEST
    # preexisted.
    mkdir -p "$dest/$d"
    cp -r "$repo_root/$d/." "$dest/$d/"
  done
  chmod +x "$dest/demo.sh"
}

root="$run_dir/root"
stage "$root"

printf 'package unit\nimport "testing"\nfunc TestAUR455Bridge(t *testing.T){ TestAUR455(t) }\n' \
  >"$root/tests/unit/aur455_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR455Bridge(t *testing.T){ IntegrationAUR455(t) }\n' \
  >"$root/tests/integration/aur455_bridge_test.go"

go_lane() {
  # One offline, bounded invocation compiles and executes the requested
  # package's real assertions against the staged artifacts. GOFLAGS=-p=1
  # and GOMAXPROCS=1 keep the compiler under the sandbox's 256MB cap (see
  # this script's header); GOCACHE/GOTMPDIR/HOME point at this run's own
  # writable scratch space, shared across every lane so only the first
  # invocation pays the cost of compiling the standard library.
  local pkg="$1" root_override="${2:-$root}" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root_override" && AURUMCODE_ROOT="$root_override" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS='-mod=mod -p=1' \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR455Bridge$' 2>&1)"
  rc=$?
  set -e
  aur455_last_output="$out"
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
  grep -Fq -- '--- PASS: TestAUR455Bridge' <<<"$out" || { fail "selector-did-not-run:$pkg"; }
  return 0
}

e2e_case() {
  # Same GOCACHE/parallelism environment as go_lane, because this lane
  # itself compiles two full binaries (cmd/regenerate-docs, cmd/aurumcode)
  # via demo.sh, not just a test package.
  local root_override="${1:-$root}" mutation_marker="${2:-}" out rc
  set +e
  out="$( ulimit -v 8388608
    AURUMCODE_ROOT="$root_override" AUR455_MUTATION="$mutation_marker" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS='-mod=mod -p=1' \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    bash "$repo_root/tests/e2e/AUR-455.sh" E2EAUR455 2>&1)"
  rc=$?
  set -e
  aur455_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

spec_case() {
  [[ -s "$repo_root/docs/specs/AUR-455.md" ]] || fail 'behavior-missing:docs/specs/AUR-455.md absent or empty'
  grep -Fq 'TestAUR455' "$repo_root/docs/specs/AUR-455.md" || fail 'behavior-missing:spec never names the TestAUR455 selector'
  grep -Fq 'AUR-445' "$repo_root/docs/specs/AUR-455.md" || fail 'behavior-missing:spec never records the AUR-445 template-residue naming note'
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
    printf '{"card":"%s","scenario":"%s","result":"pass","verification":"executed-demo-and-static","lanes":3}\n' "$card" "$scenario"
    ;;
  TestAUR455) go_lane ./tests/unit "$root" ;;
  IntegrationAUR455) go_lane ./tests/integration "$root" ;;
  E2EAUR455) e2e_case "$root" ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    # Mutate ONLY the scratch copy: rename the --seguranca flag's
    # registration so the flag README.md documents no longer exists in the
    # mutated binary. demo.sh itself (also copied into $mut_root, unmodified)
    # still passes --seguranca at step (b); the mutated aurumcode's flag
    # parser rejects it as unknown (flag.ContinueOnError -> exit 2, see
    # cmd/aurumcode/main.go's runReview), so demo.sh's own `set -e` makes
    # the whole script exit non-zero. This is exactly "quebrar um comando
    # documentado no README faz o aceite falhar porque o demo deixa de
    # rodar" (MUT-001, see .board/cards/ready/AUR-455.md).
    sed -i.bak 's/fs\.Bool("seguranca"/fs.Bool("seguranca-broken-by-mut-001"/' "$mut_root/cmd/aurumcode/main.go"
    rm -f "$mut_root/cmd/aurumcode/main.go.bak"
    grep -Fq 'seguranca-broken-by-mut-001' "$mut_root/cmd/aurumcode/main.go" \
      || infra 'mut001_sed_did_not_match'
    printf 'package unit\nimport "testing"\nfunc TestAUR455Bridge(t *testing.T){ TestAUR455(t) }\n' \
      >"$mut_root/tests/unit/aur455_bridge_test.go"
    printf 'package integration\nimport "testing"\nfunc TestAUR455Bridge(t *testing.T){ IntegrationAUR455(t) }\n' \
      >"$mut_root/tests/integration/aur455_bridge_test.go"

    e2e_rc=0; e2e_case "$mut_root" MUT-001 || e2e_rc=$?
    (( e2e_rc != 0 )) || fail 'MUT-001:e2e lane passed on mutated input, expected demo.sh to stop running'
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur455_last_output" \
      || fail 'MUT-001:e2e lane failed but never reported the MUT-001 label'

    printf '%s/%s/MUT-001 confirmed: e2e_rc=%d, tracked demo.sh and cmd/aurumcode/main.go untouched\n' "$card" "$scenario" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
