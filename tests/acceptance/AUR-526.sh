#!/usr/bin/env bash
# AUR-526 acceptance: the review reads files and searches the repository in
# any language through deliberation tools (read_file, search_text,
# find_symbol over the tree-sitter grammars, changed_file_diff). The tools
# read the reviewed revision only, refuse escapes, symlinks, ignored and
# secret files, redact what they return, and share the max_read_bytes
# ceiling whose crossing makes the review inconclusive.
#
# Selectors:
#   all        AC-001..AC-008, then MUT-001
#   AC-001     Java fixture: the caller outside the diff is read; the finding stays anchored
#   AC-002     the reviewed revision only, never the working tree or another commit
#   AC-003     outside path, escaping symlink, ignored and secret files refused; redaction
#   AC-004     max_rounds and max_read_bytes make the review partial and inconclusive
#   AC-005     loop findings pass the same scope, evidence and rule gates
#   AC-006     a provider without tools keeps the current result and warns
#   AC-007     the context and profile decorators forward llm.ToolCaller
#   AC-008     a tool call without id or with invalid arguments is refused, both ways
#   MUT-001    following a symlink out of the repository turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-526'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-007|AC-008|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur526_test.go internal/review/tools/revision.go internal/review/tools/testdata/aur526/Caller.java internal/config/secret_paths.yml; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a526.XXXXXX")" || infra mktemp
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
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  # Docs as text only: the screenshots under docs/assets and the recorded
  # tutorials fill the sealed profile's temporary disk and no test reads them.
  if [[ -d "$repo_root/docs" ]]; then
    (cd "$repo_root" && find docs -path docs/assets -prune -o -type f -name '*.md' -print) |
      while IFS= read -r f; do mkdir -p "$root/$(dirname "$f")"; cp "$repo_root/$f" "$root/$f"; done
  fi
  for source in tests/fixtures tests/e2e; do
    if [[ -d "$repo_root/$source" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/$source" "$root/$source"; fi
  done
  chmod -R u+w -- "$root"
}

# free_disk root drops a staged copy, Go's temporary files and, when it is
# this run's own, the build cache: the sealed temporary disk holds one build.
free_disk() {
  rm -rf -- "$1" "$run_dir/gotmp"
  mkdir -p "$run_dir/gotmp"
  case "$GOCACHE" in "$run_dir"/*) rm -rf -- "$GOCACHE" ;; esac
}

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3" pkg rc=0; shift 3
  : >"$log"
  # One package at a time, freeing the temporary build files in between:
  # the sealed temporary disk does not hold several test binaries at once.
  local names dir files
  names="$(printf '%s' "$pattern" | sed -e 's/^\^(//' -e 's/)\$$//' | tr '|' '\n')"
  for pkg in "$@"; do
    # Build only packages that hold one of the named tests: each test binary
    # of this module is large and the sealed temporary disk is small.
    dir="$root/${pkg%/...}"; dir="${dir%/}"
    # find + grep, not grep --include: the sealed image's grep is BusyBox.
    files="$(find "$dir" -name '*_test.go' -type f)"
    [[ -n "$files" ]] || continue
    # shellcheck disable=SC2086 # one test file per word
    grep -qF -e "$(printf '%s\n' "$names" | sed 's/^/func /')" $files || continue
    ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$pkg" ) >>"$log" 2>&1 || rc=1
    rm -rf -- "$run_dir/gotmp" && mkdir -p "$run_dir/gotmp"
    case "$GOCACHE" in "$run_dir"/*) rm -rf -- "$GOCACHE" ;; esac
  done
  return "$rc"
}

# require_pass log names... fails unless every named test passed.
require_pass() {
  local log="$1" name; shift
  for name in "$@"; do
    grep -Eq -- "^--- PASS: ${name} " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

# replace_once file anchor replacement: a literal, unique edit.
replace_once() {
  local file="$1" anchor="$2" replacement="$3" count
  count="$(grep -Fc -- "$anchor" "$file")" || infra "anchor-missing:${file##*/}"
  [[ "$count" == 1 ]] || infra "anchor-not-unique:${file##*/}"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { a = ENVIRON["ANCHOR"]; r = ENVIRON["REPL"] }
    { i = index($0, a); if (i > 0) { print substr($0, 1, i - 1) r substr($0, i + length(a)) } else { print } }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  grep -Fq -- "$replacement" "$file" || infra "mutation-not-applied:${file##*/}"
}

readonly ac001_tests=(TestAUR526JavaCallerReadThroughToolsFindingAnchoredInDiff TestAUR526SymbolFindsTheJavaCallerOutsideTheDiff TestAUR526ReadFileReachesTheModelThroughTheCommand)
readonly ac002_tests=(TestAUR526ReadsTheReviewedRevisionNeverTheWorkingTree TestAUR526DiffToolAnswersOnlyChangedFiles)
readonly ac003_tests=(TestAUR526RefusesEscapesIgnoredAndSecretFiles TestAUR526MUT001SymlinkWithMatchingBytesIsRefused TestAUR526SecretAndIgnoredPaths TestAUR526SecretPathsIgnoreCaseAndCoverCloudCredentials)
readonly ac004_tests=(TestAUR526CacheCeilingMakesTheReviewPartial TestAUR526ByteCeilingMakesTheReviewPartial TestAUR526ByteCeilingIsADeliberationLimit TestAUR526ReadCeilingIsInconclusive TestAUR580RoundsExceededIsInconclusiveAndUnpublished)
readonly ac005_tests=(TestAUR526JavaCallerReadThroughToolsFindingAnchoredInDiff TestOutsideDiffAC003ProvedFindingNeverCountsForTheGate)
readonly ac006_tests=(TestAUR580DeferredScannerRunsWhenToolsCannotBeOffered TestAUR525ToolSupportDetectableBeforeCall)
readonly ac007_tests=(TestAUR526ContextWrapperForwardsToolCalling TestAUR526ProfileDecoratorForwardsToolCalling)
readonly ac008_tests=(TestAUR526ValidateToolCall TestAUR526OrchestratorRefusesIDLessCall TestAUR526IncomingCallWithoutIDIsRefused TestAUR526OutgoingInvalidCallIsNeverSent TestAUR525MalformedArgumentsIsTypedError)
readonly pkgs=(./cmd/aurumcode/ ./internal/review/ ./internal/review/tools/ ./internal/config/ ./internal/llm/...)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
  # One staged copy of the module per AC fills the sealed temporary disk.
  free_disk "$root"
  printf '%s/%s/pass\n' "$card" "$name"
}

# expect_red root log tests...: the mutated copy must compile and turn at
# least one named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2"; shift 2
  if go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

# MUT-001: the revision follows a symbolic link (Stat instead of Lstat).
run_mut001() {
  local root="$run_dir/root-mut1"
  free_disk "$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/review/tools/revision.go" \
    'info, err := os.Lstat(full)' \
    'info, err := os.Stat(full) // MUT-001: follows a symlink'
  expect_red "$root" "$run_dir/mut1.log" "${ac003_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR526MUT001SymlinkWithMatchingBytesIsRefused' "$run_dir/mut1.log" || fail mut001-wrong-test
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac AC-004 "${ac004_tests[@]}" ;;
  AC-005) run_ac AC-005 "${ac005_tests[@]}" ;;
  AC-006) run_ac AC-006 "${ac006_tests[@]}" ;;
  AC-007) run_ac AC-007 "${ac007_tests[@]}" ;;
  AC-008) run_ac AC-008 "${ac008_tests[@]}" ;;
  MUT-001) run_mut001 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac AC-004 "${ac004_tests[@]}"
    run_ac AC-005 "${ac005_tests[@]}"
    run_ac AC-006 "${ac006_tests[@]}"
    run_ac AC-007 "${ac007_tests[@]}"
    run_ac AC-008 "${ac008_tests[@]}"
    run_mut001
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
