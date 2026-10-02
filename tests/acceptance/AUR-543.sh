#!/usr/bin/env bash
# AUR-543 acceptance: the per-file review cache's prompt-version key
# component must be a digest of the fixed content the prompt builder
# actually renders (instructions, built-in rule catalog, schema text),
# computed at run time -- not a hand-bumped constant. Editing any such fixed
# text must change the key, with no constant to edit (AC-001); the digest
# must be stable across runs when nothing fixed changed (AC-002); and the
# same model name served by a second, independent endpoint (a different
# LLM_BASE_URL) must force a fresh review end to end, proven with two real
# httptest servers and a persisted cache (AC-003). See docs/review-cache.md.
#
# Selectors:
#   all             run every behavior test below, then also run the
#                   AC-001-MUT-001 mutation and confirm it goes RED
#   AC-001          editing the fixed prompt template, or the built-in rule
#                   catalog, moves FixedContentDigest's result (unit), AND
#                   forces a real second request through runReview's actual
#                   production wiring with a persisted cache (end-to-end)
#   AC-002          FixedContentDigest is stable across repeated calls and
#                   across independently constructed builders
#   AC-003          end-to-end: the same model name across two real
#                   httptest servers (different LLM_BASE_URL), with a
#                   persisted cache, forces a fresh review against the
#                   second server and still reuses the first server's own
#                   entry when revisited
#   AC-001-MUT-001  make FixedContentDigest hash a fixed string literal
#                   instead of the rendered basePrompt; AC-001 must FAIL
#                   (RED) -- a digest that never moves no matter what fixed
#                   text changes is exactly the defect this card closes
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-543'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/internal/prompt/builder.go" ]] || fail missing-source
[[ -f "$repo_root/internal/review/cache/cache.go" ]] || fail missing-source
[[ -f "$repo_root/cmd/aurumcode/review_cache.go" ]] || fail missing-source
[[ -f "$repo_root/cmd/aurumcode/main.go" ]] || fail missing-source
[[ -f "$repo_root/internal/prompt/aur543_test.go" ]] || fail missing-behavior-test
[[ -f "$repo_root/cmd/aurumcode/aur543_test.go" ]] || fail missing-behavior-test
[[ -f "$repo_root/docs/review-cache.md" ]] || fail missing-doc

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a543.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/root" "$run_dir/cache" "$run_dir/gotmp"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# glob_escape makes $1 safe as the LHS pattern of a bash "${var//pat/repl}"
# substitution: that substitution matches as a filename glob, not a literal
# string, so '[' and ']' (both present in the Go source anchors below, e.g.
# "[]byte(") must be escaped or they are read as a bracket-expression
# character class instead of literal bytes. '*', '?' and '\' are escaped
# too, on the same principle, even though none of this script's anchors
# currently contain them.
glob_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\[/\\[}"
  s="${s//\]/\\]}"
  s="${s//\*/\\*}"
  s="${s//\?/\\?}"
  printf '%s' "$s"
}

# AC-001-MUT-001: make FixedContentDigest hash a fixed string literal
# instead of the rendered basePrompt (internal/prompt/builder.go) --
# reproducing exactly the defect this card closes: a cache-key component
# that never moves no matter what fixed prompt text, schema wording or
# built-in rule catalog entry changes, which is the hand-bumped-constant
# bypass AC-001 exists to refuse. Anchored on the one line that computes the
# digest's input so this file cannot match its own edit, and a missing
# anchor is infrastructure, never a silent no-op. Edits via bash's own
# literal substring substitution (not sed) because the anchor's "[]byte("
# is not safe to hand to a sed/BRE regex without its own escaping.
apply_mutation_ac001() {
  local target="$run_dir/root/internal/prompt/builder.go"
  local anchor='sum := sha256.Sum256([]byte(basePrompt))'
  local count
  count="$(grep -Fc -- "$anchor" "$target")" || infra mutation-anchor-missing
  (( count == 1 )) || infra mutation-anchor-not-unique
  # basePrompt[:0] is always the empty string regardless of basePrompt's own
  # content, so the hashed input is effectively the fixed literal appended
  # to it -- constant no matter what fixed prompt text changed -- while
  # still REFERENCING basePrompt, so the mutated function keeps compiling
  # (an unused local would be a build failure, which this script must never
  # misreport as the behavioral RED this mutation exists to produce).
  local replacement='sum := sha256.Sum256([]byte(basePrompt[:0] + "aur543-mut-001-constant-digest-input"))'
  local content escaped_anchor mutated
  content="$(cat "$target")" || infra mutation-read-failed
  escaped_anchor="$(glob_escape "$anchor")"
  mutated="${content//$escaped_anchor/$replacement}"
  printf '%s\n' "$mutated" > "$target"
  grep -Fq -- "$anchor" "$target" && infra mutation-not-applied
  grep -Fq -- "$replacement" "$target" || infra mutation-not-applied
  return 0
}

pkgs='./internal/prompt/... ./cmd/aurumcode/...'
test_pattern=''
expect_fail=''
case "$selector" in
  AC-001)         test_pattern='^(TestAUR543AC001FixedTextChangeMovesDigest|TestAUR543AC001CatalogChangeMovesDigest|TestAUR543AC001PromptEditForcesFreshReview)$' ;;
  AC-002)         test_pattern='^TestAUR543AC002DigestStableAcrossRuns$' ;;
  AC-003)         test_pattern='^TestAUR543AC003DifferentBaseURLForcesFreshReview$' ;;
  AC-001-MUT-001) test_pattern='^TestAUR543AC001.*$'; expect_fail=1; apply_mutation_ac001 ;;
  all)            test_pattern='^TestAUR543' ;;
esac

run_go_test() {
  local pattern="$1" log="$2"
  set +e
  # shellcheck disable=SC2086
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$pattern") >"$log" 2>&1
  local status=$?
  set -e
  return $status
}

log="$run_dir/test.log"
status=0
run_go_test "$test_pattern" "$log" || status=$?
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR543' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR543' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in \
      AC001FixedTextChangeMovesDigest AC001CatalogChangeMovesDigest \
      AC001PromptEditForcesFreshReview \
      AC002DigestStableAcrossRuns AC003DifferentBaseURLForcesFreshReview; do
    grep -q "^--- PASS: TestAUR543$name " "$log" || fail "missing-pass:$name"
  done

  # `all` also runs the AC-001-MUT-001 mutation and requires it to go RED,
  # in the SAME checked-out tree, after the nominal run above already
  # proved green on unmutated sources -- so a selector of "all" can never
  # pass by skipping the skeptical check the dedicated selector performs.
  apply_mutation_ac001
  mut_log="$run_dir/test-mut.log"
  mut_status=0
  run_go_test '^TestAUR543AC001.*$' "$mut_log" || mut_status=$?
  cat "$mut_log" >&2
  grep -Eq -- '^--- FAIL: TestAUR543' "$mut_log" || fail 'mutation-survived'
  (( mut_status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$mut_log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
fi

printf '%s/%s/pass\n' "$card" "$selector"
