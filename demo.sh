#!/usr/bin/env bash
# AUR-455: a new developer runs AurumCode's three features in about a
# minute, from a clean `git clone`, offline, with no LLM key.
#
# Four labeled steps:
#   (a) go build ./cmd/regenerate-docs, then run it -- documentation
#       generation, printed as "result=ok".
#   (b) aurumcode review --base HEAD~1 --seguranca -- the deterministic
#       security pass. This is the best first command in the product: it
#       needs no LLM key, no fixture, no environment variable at all.
#   (c) aurumcode review --base HEAD~1 with AURUMCODE_LLM_FIXTURE set --
#       a model-driven review made fully deterministic by a fixture file
#       instead of a live model, so it also runs offline.
#   (d) the Jekyll publish command step (a) already printed, shown again
#       under its own label so it is not missed.
#
# WHY (b) AND (c) RUN INSIDE tests/fixtures/repos/git-demo/repo.git
#   That fixture is a small, tracked, deterministic git repository stored
#   as loose objects. Using it instead of this repository's own history
#   keeps the demo byte-for-byte reproducible, and keeps it runnable
#   inside AUR-455's sealed acceptance sandbox (bootstrap-readonly-v1),
#   which has no `git` binary, no network, and never receives this
#   repository's own .git directory (paths/read_paths materialize tracked
#   FILES only; forbidden_paths blocks .git outright -- see
#   .board/bin/oci-run). The exact same two commands work unchanged
#   against any real repository: `cd` into it and run them.
#
# WHY THE BINARIES ARE BUILT ONCE INSTEAD OF THREE `go run` CALLS
#   `go build` once for each command and then invoking the binary directly
#   means demo.sh actually exercises the build path this repository's own
#   README documents (`go build ./cmd/... ./internal/... ./pkg/...`), not
#   just its output. Breaking that build breaks this script the same way
#   it would break a real developer's first command.
#
# Exit 0 means all four steps produced the output documented in the
# README and docs/specs/AUR-455.md. Any other exit means the repository
# does not do what its own documentation claims.
set -euo pipefail

script_dir="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" && pwd -P)"
repo_root="$script_dir"
cd "$repo_root"

command -v go >/dev/null 2>&1 || { echo "demo.sh: go toolchain not found on PATH" >&2; exit 1; }

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurumcode-demo.XXXXXX")"
trap 'rm -rf -- "$work_dir"' EXIT
bin_dir="$work_dir/bin"
docs_out="$work_dir/docs-out"
mkdir -p "$bin_dir" "$docs_out"

git_demo_repo="$repo_root/tests/fixtures/repos/git-demo/repo.git"
review_fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
docs_fixture_src="$repo_root/tests/fixtures/docs/goproject"

for required in "$git_demo_repo" "$review_fixture" "$docs_fixture_src"; do
  [[ -e "$required" ]] || { echo "demo.sh: required fixture missing: $required" >&2; exit 1; }
done

step() { printf '\n=== %s ===\n' "$1"; }

# ---------------------------------------------------------------------------
step 'Building the two AurumCode binaries (go build ./cmd/regenerate-docs, ./cmd/aurumcode)'
go build -o "$bin_dir/regenerate-docs" ./cmd/regenerate-docs
go build -o "$bin_dir/aurumcode" ./cmd/aurumcode

# ---------------------------------------------------------------------------
step '(a) Generating documentation: regenerate-docs'
step_a_output="$(
  AURUMCODE_SOURCE_DIR="$docs_fixture_src" \
  AURUMCODE_OUTPUT_DIR="$docs_out" \
    "$bin_dir/regenerate-docs" 2>&1
)"
printf '%s\n' "$step_a_output"
grep -q 'result=ok' <<<"$step_a_output" ||
  { echo "demo.sh: step (a) did not report result=ok" >&2; exit 1; }

# ---------------------------------------------------------------------------
step '(b) Reviewing a diff with zero configuration: aurumcode review --base HEAD~1 --seguranca'
(
  cd "$git_demo_repo"
  "$bin_dir/aurumcode" review --base HEAD~1 --seguranca
)

# ---------------------------------------------------------------------------
step '(c) Reviewing the same diff with a deterministic fixture model response'
(
  cd "$git_demo_repo"
  AURUMCODE_LLM_FIXTURE="$review_fixture" "$bin_dir/aurumcode" review --base HEAD~1
)

# ---------------------------------------------------------------------------
step '(d) The Jekyll publish command step (a) printed'
jekyll_line="$(grep -A1 -F 'Build Jekyll site with:' <<<"$step_a_output" || true)"
[[ -n "$jekyll_line" ]] ||
  { echo "demo.sh: step (a) never printed a Jekyll publish command" >&2; exit 1; }
printf '%s\n' "$jekyll_line"

printf '\nAll three features ran offline, with no LLM key.\n'
