#!/usr/bin/env bash
# E2E check for AUR-441: build (or reuse) the real aurumcode binary and run
# `review --base HEAD~1` twice, as a user would, against an unchanged
# repository. Proves that the second run does not resend any file to the
# model (observed through AURUMCODE_PROMPT_CAPTURE: present with two
# captured "### File:" sections on the cold first run, absent entirely on
# the full-cache-hit second run, since the model is never called at all),
# reports how many files were reused on stderr, and reproduces the first
# run's stdout byte for byte. It also proves the sensible default cache
# location (AURUMCODE_CACHE_DIR unset) works, scoped under the reviewed
# repository. See docs/specs/AUR-441.md.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-441
selector="${1:-E2EAUR441}"
[[ "$selector" == "E2EAUR441" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

demo_src="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$demo_src" || fail missing-demo-fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a441.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-441.sh's e2e_case does).
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS='-mod=mod -p=1' go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# A deterministic model response with exactly one finding on
# config/demo-tokens.txt, the file commit 3 of git-demo adds -- the same
# planted problem tests/acceptance/AUR-430.sh's own fixture uses.
fixture="$run_dir/known-problem.json"
cat >"$fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN).",
      "suggestion": "Remove the secret from version control and load it from the environment instead."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF

count_sections() {
  # Prints the number of captured "### File:" sections, or -1 when the
  # capture file does not exist at all -- the signal a full cache hit
  # never even calls the model (FakeProvider.Complete, which alone writes
  # this file, is then never invoked).
  local f="$1"
  if [[ ! -e "$f" ]]; then
    printf -- '-1\n'
    return 0
  fi
  grep -c '### File:' "$f" || true
}

# --- Pinned AURUMCODE_CACHE_DIR: the core proof. ---
demo_repo="$run_dir/repo.git"
cp -R "$demo_src" "$demo_repo"
chmod -R u+w -- "$demo_repo"
cache_dir="$run_dir/cache"

out1="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$cache_dir" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture1.txt" "$bin" review --base HEAD~1 2>"$run_dir/err1.txt")" || fail first-run-failed
grep -Fq 'config/demo-tokens.txt' <<<"$out1" || fail behavior-missing
grep -Fq '[error]' <<<"$out1" || fail behavior-missing
sections1="$(count_sections "$run_dir/capture1.txt")"
[[ "$sections1" == "2" ]] || fail "cold-run-must-send-both-files:got:$sections1"
if grep -Fq 'reused' "$run_dir/err1.txt"; then fail cold-run-must-not-claim-reuse; fi

out2="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$cache_dir" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture2.txt" "$bin" review --base HEAD~1 2>"$run_dir/err2.txt")" || fail second-run-failed
[[ "$out2" == "$out1" ]] || fail stdout-not-byte-identical
sections2="$(count_sections "$run_dir/capture2.txt")"
[[ "$sections2" == "-1" ]] || fail "warm-run-must-never-call-the-model:got:$sections2"
grep -Fq 'reused 2 file' "$run_dir/err2.txt" || fail reuse-count-not-reported

# --- Default cache directory (AURUMCODE_CACHE_DIR unset): must still
# work, scoped under the reviewed repository, never touching the tracked
# fixture tree (this program only ever operates on its own writable
# copy). ---
default_repo="$run_dir/repo-default.git"
cp -R "$demo_src" "$default_repo"
chmod -R u+w -- "$default_repo"

d1="$(cd "$default_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-d1.txt" "$bin" review --base HEAD~1)" || fail default-first-run-failed
[[ "$(count_sections "$run_dir/capture-d1.txt")" == "2" ]] || fail default-cold-run-must-send-both-files
[[ -d "$default_repo/.aurumcode-cache" ]] || fail default-cache-dir-not-created

d2="$(cd "$default_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-d2.txt" "$bin" review --base HEAD~1)" || fail default-second-run-failed
[[ "$d2" == "$d1" ]] || fail default-stdout-not-byte-identical
[[ "$(count_sections "$run_dir/capture-d2.txt")" == "-1" ]] || fail default-warm-run-must-never-call-the-model

# --- Partial cache hit must not duplicate a cache-hit file's finding.
# The offline fixture provider is a fixed canned response that ignores
# the prompt entirely: even sending only the miss file (NOTES.txt), it
# still answers with its usual config/demo-tokens.txt finding. Without
# dropping that stale fresh answer for the cache-hit file before merging
# the cached one on top, the finding would print twice. ---
partial_repo="$run_dir/repo-partial.git"
cp -R "$demo_src" "$partial_repo"
chmod -R u+w -- "$partial_repo"
partial_cache="$run_dir/cache-partial"

p1="$(cd "$partial_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$partial_cache" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-p1.txt" "$bin" review --base HEAD~1)" || fail partial-setup-run-failed
[[ "$(count_sections "$run_dir/capture-p1.txt")" == "2" ]] || fail partial-setup-must-send-both-files

notes_entry=""
for f in "$partial_cache"/*; do
  if grep -Fq '"NOTES.txt"' "$f" 2>/dev/null; then
    notes_entry="$f"
    break
  fi
done
[[ -n "$notes_entry" ]] || fail partial-notes-entry-not-found
rm -f -- "$notes_entry"

p2="$(cd "$partial_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$partial_cache" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-p2.txt" "$bin" review --base HEAD~1)" || fail partial-run-failed
[[ "$(count_sections "$run_dir/capture-p2.txt")" == "1" ]] || fail partial-run-must-send-exactly-one-file
occurrences="$(grep -c 'config/demo-tokens.txt:4:' <<<"$p2" || true)"
[[ "$occurrences" == "1" ]] || fail "partial-hit-duplicated-finding:got:$occurrences"

printf '%s/AC-001/e2e-ok\n' "$card"
