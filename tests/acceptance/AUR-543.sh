#!/usr/bin/env bash
# AUR-543 acceptance: the per-file review cache's prompt-version key
# component must be a digest of the fixed content the prompt builder
# actually renders -- instructions, built-in rule catalog, schema text,
# BOTH ReviewChangeScope instructional variants, the coverage declaration's
# header/count-lines/bullets, buildUserContent's/fixedOverhead's own
# section headers, and the "### File:" hunk header -- computed at run time,
# not a hand-bumped constant. Editing any such fixed text must change the
# key, with no constant to edit (AC-001); the digest must be stable across
# runs when nothing fixed changed (AC-002); the same model name served by a
# second, independent endpoint (a different LLM_BASE_URL) must force a
# fresh review end to end, proven with two real httptest servers and a
# persisted cache (AC-003); and a digest computation failure must degrade
# the whole cache to "no cache this run", never a crash and never a stale
# or wrong entry (N1). See docs/review-cache.md.
#
# Selectors:
#   all             run every behavior test below, then run BOTH mutations
#                   and confirm each produces RED on exactly the tests it
#                   should break
#   AC-001          editing the fixed prompt template, the built-in rule
#                   catalog, or either ReviewChangeScope instructional
#                   variant moves FixedContentDigest's result (unit); the
#                   fixed content hashed actually contains every literal
#                   this card's own review named (unit); and editing the
#                   fixed content forces a real second request through
#                   runReview's actual production wiring with a persisted
#                   cache (end-to-end)
#   AC-002          FixedContentDigest is stable across repeated calls and
#                   across independently constructed builders
#   AC-003          end-to-end: the same model name across two real
#                   httptest servers (different LLM_BASE_URL), with a
#                   persisted cache, forces a fresh review against the
#                   second server and still reuses the first server's own
#                   entry when revisited
#   N1              a prompt-version digest computation failure degrades
#                   the whole review cache to "no cache this run" -- the
#                   review still completes and reaches the model, no
#                   "reused" note, and NOTHING is written to the cache
#                   directory under the meaningless key
#   AC-001-MUT-001  make FixedContentDigest hash a fixed string literal
#                   instead of the real rendered content (internal/prompt);
#                   every AC-001 test that calls FixedContentDigest must go
#                   RED -- a digest that never moves no matter what fixed
#                   text changes is exactly the defect this card closes
#   AC-001-MUT-002  edit a USER-HALF fixed header literal (buildUserContent's
#                   own "## Code Changes", internal/prompt/builder.go) --
#                   the gap an independent review found in this card's first
#                   cut, where the digest covered only the system-half
#                   template; the containment test must go RED
#   N1-MUT-001      drop the `cacheErr = promptDigestErr` fold-in in
#                   cmd/aurumcode/main.go; N1 must go RED -- a digest
#                   failure that silently leaves caching enabled is exactly
#                   the defect N1 exists to refuse
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-543'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|N1|AC-001-MUT-001|AC-001-MUT-002|N1-MUT-001) ;;
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
[[ -f "$repo_root/internal/prompt/filetype.go" ]] || fail missing-source
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
# string, so '[' and ']' (present in the Go source anchors below, e.g.
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

# apply_literal_mutation edits $1 in place, replacing the unique occurrence
# of $2 with $3, via bash's own literal substring substitution (not sed):
# several of this card's anchors contain "[]byte(", which a sed/BRE regex
# would read as a bracket-expression character class rather than literal
# bytes. A missing or non-unique anchor is infrastructure, never a silent
# no-op.
apply_literal_mutation() {
  local target="$1" anchor="$2" replacement="$3"
  local count
  count="$(grep -Fc -- "$anchor" "$target")" || infra mutation-anchor-missing
  (( count == 1 )) || infra mutation-anchor-not-unique
  local content escaped_anchor mutated
  content="$(cat "$target")" || infra mutation-read-failed
  escaped_anchor="$(glob_escape "$anchor")"
  mutated="${content//$escaped_anchor/$replacement}"
  printf '%s\n' "$mutated" > "$target"
  grep -Fq -- "$anchor" "$target" && infra mutation-not-applied
  grep -Fq -- "$replacement" "$target" || infra mutation-not-applied
  return 0
}

# AC-001-MUT-001: make FixedContentDigest hash a fixed string literal
# instead of the real rendered content (internal/prompt/builder.go) --
# reproducing exactly the defect this card closes: a cache-key component
# that never moves no matter what fixed prompt text, schema wording,
# built-in rule catalog entry or ReviewChangeScope instruction changes,
# which is the hand-bumped-constant bypass AC-001 exists to refuse.
# content[:0] is always the empty string regardless of content's own
# value, so the hashed input is effectively the fixed literal appended to
# it -- constant no matter what fixed prompt text changed -- while still
# REFERENCING content, so the mutated function keeps compiling (an unused
# local would be a build failure, which this script must never misreport
# as the behavioral RED this mutation exists to produce).
apply_mutation_ac001() {
  apply_literal_mutation \
    "$run_dir/root/internal/prompt/builder.go" \
    'sum := sha256.Sum256([]byte(content))' \
    'sum := sha256.Sum256([]byte(content[:0] + "aur543-mut-001-constant-digest-input"))'
}

# AC-001-MUT-002: edit a USER-HALF fixed header literal -- buildUserContent's
# own "## Code Changes" line (internal/prompt/builder.go) -- the specific
# gap an independent review found in this card's first cut, where the
# digest covered only the system-half template and every literal
# buildUserContent/fixedOverhead write directly as Go string literals was
# invisible to it. TestAUR543B1FixedContentCoversUserHalfAndChangeScope
# asserts the exact string "## Code Changes" is present in the hashed
# content; this mutation removes it, so that assertion -- not a digest
# value comparison -- is what must go RED.
apply_mutation_ac001_mut002() {
  apply_literal_mutation \
    "$run_dir/root/internal/prompt/builder.go" \
    'result.WriteString("## Code Changes\n\n")' \
    'result.WriteString("## AUR543 Mutated User-Half Header\n\n")'
}

# N1-MUT-001: drop the digest-error fold-in in cmd/aurumcode/main.go --
# reproducing exactly the defect N1 exists to refuse: a prompt-version
# digest computation that failed is silently ignored, so caching stays
# enabled under a meaningless ("") promptVersion instead of degrading the
# whole run to "no cache this run".
apply_mutation_n1() {
  apply_literal_mutation \
    "$run_dir/root/cmd/aurumcode/main.go" \
    'cacheErr = promptDigestErr' \
    '_ = promptDigestErr'
}

pkgs='./internal/prompt/... ./cmd/aurumcode/...'

# For every non-mutation selector, test_pattern selects which tests run and
# want_pass names EVERY test that selector requires a PASS line for (N2: a
# selector must never be satisfied by "go test exited 0 and at least one
# test matched" when it could have silently matched zero of its own named
# tests instead).
test_pattern=''
want_pass=()
case "$selector" in
  AC-001)
    test_pattern='^(TestAUR543AC001FixedTextChangeMovesDigest|TestAUR543AC001CatalogChangeMovesDigest|TestAUR543AC001PromptEditForcesFreshReview|TestAUR543B1FixedContentCoversUserHalfAndChangeScope|TestAUR543B1ChangeScopeTextMovesDigest)$'
    want_pass=(AC001FixedTextChangeMovesDigest AC001CatalogChangeMovesDigest AC001PromptEditForcesFreshReview B1FixedContentCoversUserHalfAndChangeScope B1ChangeScopeTextMovesDigest)
    ;;
  AC-002)
    test_pattern='^TestAUR543AC002DigestStableAcrossRuns$'
    want_pass=(AC002DigestStableAcrossRuns)
    ;;
  AC-003)
    test_pattern='^TestAUR543AC003DifferentBaseURLForcesFreshReview$'
    want_pass=(AC003DifferentBaseURLForcesFreshReview)
    ;;
  N1)
    test_pattern='^TestAUR543N1DigestErrorDegradesToNoCache$'
    want_pass=(N1DigestErrorDegradesToNoCache)
    ;;
  all)
    test_pattern='^TestAUR543'
    want_pass=(AC001FixedTextChangeMovesDigest AC001CatalogChangeMovesDigest AC001PromptEditForcesFreshReview
               B1FixedContentCoversUserHalfAndChangeScope B1ChangeScopeTextMovesDigest
               AC002DigestStableAcrossRuns AC003DifferentBaseURLForcesFreshReview N1DigestErrorDegradesToNoCache)
    ;;
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

# require_red asserts log shows every name in $@ (bare TestAUR543<name>, no
# package prefix) FAILING, and that the overall exit was non-zero, and that
# no name failed merely because the mutation broke compilation (which would
# misreport an infrastructure problem as this mutation's own behavioral
# proof).
require_red() {
  local log="$1"; shift
  for name in "$@"; do
    grep -Eq -- "^--- FAIL: TestAUR543$name( |\$)" "$log" || fail "mutation-survived:$name"
  done
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

case "$selector" in
  AC-001-MUT-001)
    apply_mutation_ac001
    log="$run_dir/test-mut.log"
    mut_status=0
    run_go_test '^TestAUR543(AC001|B1ChangeScopeTextMovesDigest)' "$log" || mut_status=$?
    cat "$log" >&2
    (( mut_status != 0 )) || fail 'mutation-survived-exit-zero'
    require_red "$log" AC001FixedTextChangeMovesDigest AC001CatalogChangeMovesDigest AC001PromptEditForcesFreshReview B1ChangeScopeTextMovesDigest
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    exit 0
    ;;
  N1-MUT-001)
    apply_mutation_n1
    log="$run_dir/test-mut.log"
    mut_status=0
    run_go_test '^TestAUR543N1DigestErrorDegradesToNoCache$' "$log" || mut_status=$?
    cat "$log" >&2
    (( mut_status != 0 )) || fail 'mutation-survived-exit-zero'
    require_red "$log" N1DigestErrorDegradesToNoCache
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    exit 0
    ;;
  AC-001-MUT-002)
    apply_mutation_ac001_mut002
    log="$run_dir/test-mut.log"
    mut_status=0
    run_go_test '^TestAUR543B1FixedContentCoversUserHalfAndChangeScope$' "$log" || mut_status=$?
    cat "$log" >&2
    (( mut_status != 0 )) || fail 'mutation-survived-exit-zero'
    require_red "$log" B1FixedContentCoversUserHalfAndChangeScope
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    exit 0
    ;;
esac

log="$run_dir/test.log"
status=0
run_go_test "$test_pattern" "$log" || status=$?
cat "$log" >&2

(( status == 0 )) || fail "go-test-exit:$status"
for name in "${want_pass[@]}"; do
  grep -q "^--- PASS: TestAUR543$name " "$log" || fail "missing-pass:$name"
done

if [[ "$selector" == all ]]; then
  # `all` also runs BOTH mutations and requires each to go RED on exactly
  # the tests it should break, in the SAME checked-out tree, after the
  # nominal run above already proved every test green on unmutated
  # sources -- so "all" can never pass by skipping what the dedicated MUT
  # selectors check.
  apply_mutation_ac001
  mut_log1="$run_dir/test-mut1.log"
  mut_status1=0
  run_go_test '^TestAUR543' "$mut_log1" || mut_status1=$?
  cat "$mut_log1" >&2
  (( mut_status1 != 0 )) || fail 'mutation-survived-exit-zero:AC-001-MUT-001'
  require_red "$mut_log1" AC001FixedTextChangeMovesDigest AC001CatalogChangeMovesDigest AC001PromptEditForcesFreshReview B1ChangeScopeTextMovesDigest

  apply_mutation_n1
  mut_log2="$run_dir/test-mut2.log"
  mut_status2=0
  run_go_test '^TestAUR543' "$mut_log2" || mut_status2=$?
  cat "$mut_log2" >&2
  (( mut_status2 != 0 )) || fail 'mutation-survived-exit-zero:N1-MUT-001'
  require_red "$mut_log2" N1DigestErrorDegradesToNoCache

  apply_mutation_ac001_mut002
  mut_log3="$run_dir/test-mut3.log"
  mut_status3=0
  run_go_test '^TestAUR543' "$mut_log3" || mut_status3=$?
  cat "$mut_log3" >&2
  (( mut_status3 != 0 )) || fail 'mutation-survived-exit-zero:AC-001-MUT-002'
  require_red "$mut_log3" B1FixedContentCoversUserHalfAndChangeScope
fi

printf '%s/%s/pass\n' "$card" "$selector"
