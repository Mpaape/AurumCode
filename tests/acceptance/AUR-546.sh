#!/usr/bin/env bash
# AUR-546 acceptance: "line_comments" is not a findings schema. The template
# does not teach it, the parser does not accept it, tests/e2e/AUR-459.sh
# (which proved the old conversion) is retired, and no finding without
# evidence reaches the result through it. See docs/specs/AUR-546.md.
#
# Selectors:
#   all      AC-001, AC-002, then MUT-001
#   AC-001   decision in the spec; template, parser and the retired e2e agree
#   AC-002   a line_comments-only reply is inconclusive (unit and real binary);
#            line comments beside issues are dropped and announced on stderr
#   MUT-001  converting line_comments without evidence into accepted findings
#            (and counting it as a findings list) turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-546'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg docs/specs/AUR-546.md tests/e2e/AUR-459.sh \
  internal/prompt/parser_line_comments.go internal/prompt/parser_line_comments_test.go \
  internal/prompt/parser_validate.go internal/prompt/templates/review.md tests/fixtures/repos/git-demo/repo.git; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a546.XXXXXX")" || infra mktemp
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
  chmod -R u+w -- "$root"
}

# go_test runs the named tests of internal/prompt in a staged root.
go_test() {
  local root="$1" pattern="$2" log="$3"
  ( cd "$root" && go test -buildvcs=false -count=1 -run "$pattern" ./internal/prompt ) >"$log" 2>&1
}

readonly ac001_tests='^(TestLineCommentsIsNeitherTaughtNorAccepted|TestReviewTemplateMatchesParser|TestAcceptedReviewFieldsAreConsumed)$'
readonly ac002_tests='^(TestLineCommentsOnlyReplyIsInconclusive|TestLineCommentsBesideIssuesAreDroppedAndAnnounced|TestNoLineCommentsNoWarning|TestAnswerWithoutFindingsListIsInconclusive)$'

ac001() {
  local spec="$repo_root/docs/specs/AUR-546.md" rc out
  grep -Fq 'Decisao' "$spec" || fail spec-lacks-decision
  grep -Fq 'line_comments sai do parser' "$spec" || fail spec-lacks-chosen-option
  grep -Fwq 'line_comments' "$repo_root/internal/prompt/templates/review.md" && fail template-teaches-line-comments
  grep -rFq 'adoptLineComments' "$repo_root/internal" && fail conversion-still-present
  out="$run_dir/e2e459.err"
  set +e; bash "$repo_root/tests/e2e/AUR-459.sh" E2EAUR459 2>"$out"; rc=$?; set -e
  [[ "$rc" -eq 69 ]] || fail "e2e-459-not-retired:rc:$rc"
  grep -Fq 'docs/specs/AUR-546.md' "$out" || fail e2e-459-retirement-unjustified
  set +e; bash "$repo_root/tests/e2e/AUR-459.sh" bogus 2>/dev/null; rc=$?; set -e
  [[ "$rc" -eq 64 ]] || fail "e2e-459-unknown-selector:rc:$rc"
  local root="$run_dir/ac001"
  stage "$root"
  go_test "$root" "$ac001_tests" "$run_dir/ac001.log" || { cat "$run_dir/ac001.log" >&2; fail behavior-red; }
}

# binary_review runs the real CLI against the git-demo fixture repository
# with a canned model reply. Globals: bin_rc, bin_out, bin_err.
binary_review() {
  local bin="$1" fixture="$2" repo="$run_dir/git-demo"
  [[ -d "$repo" ]] || { cp -R "$repo_root/tests/fixtures/repos/git-demo" "$repo"; chmod -R u+w -- "$repo"; }
  bin_out="$run_dir/bin.out"; bin_err="$run_dir/bin.err"
  set +e
  ( cd "$repo/repo.git" && env -u LLM_API_KEY -u LLM_BASE_URL -u LLM_PROVIDER -u AURUMCODE_CACHE_DIR \
    "AURUMCODE_LLM_FIXTURE=$fixture" "$bin" review --base HEAD~1 --fail-on high ) >"$bin_out" 2>"$bin_err"
  bin_rc=$?
  set -e
}

ac002() {
  local root="$run_dir/ac002" bin="$run_dir/aurumcode"
  stage "$root"
  go_test "$root" "$ac002_tests" "$run_dir/ac002.log" || { cat "$run_dir/ac002.log" >&2; fail behavior-red; }
  ( cd "$root" && go build -buildvcs=false -o "$bin" ./cmd/aurumcode ) >"$run_dir/build.log" 2>&1 || { cat "$run_dir/build.log" >&2; infra build_failed; }
  printf '%s\n' '{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","rule_id":"security/hardcoded-secret","body":"credential-shaped value committed"}]}' >"$run_dir/only.json"
  printf '%s\n' '{"issues":[],"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","body":"credential-shaped value committed"}]}' >"$run_dir/mixed.json"
  binary_review "$bin" "$run_dir/only.json"
  [[ "$bin_rc" -ne 0 ]] || fail binary-accepted-line-comments-only-reply
  grep -Fq 'No issues found.' "$bin_out" && fail binary-reported-no-issues
  grep -Fq 'config/demo-tokens.txt:4' "$bin_out" && fail binary-printed-unevidenced-finding
  grep -Fq 'validation_failed' "$bin_err" || { cat "$bin_err" >&2; fail binary-not-inconclusive; }
  binary_review "$bin" "$run_dir/mixed.json"
  [[ "$bin_rc" -eq 0 ]] || { cat "$bin_err" >&2; fail "binary-mixed-rc:$bin_rc"; }
  grep -Fq 'config/demo-tokens.txt:4' "$bin_out" && fail binary-printed-unevidenced-finding
  grep -Fq '1 line comment(s) ignored' "$bin_err" || { cat "$bin_err" >&2; fail binary-drop-not-announced; }
}

# replace_once swaps a unique literal anchor in a staged file; a missing or
# non-unique anchor is infrastructure, never a silent no-op.
replace_once() {
  local target="$1" anchor="$2" replacement="$3" count content
  count="$(grep -Fc -- "$anchor" "$target")" || infra "mutation-anchor-missing:${target##*/}"
  (( count == 1 )) || infra "mutation-anchor-not-unique:${target##*/}"
  content="$(cat "$target")"
  printf '%s\n' "${content/"$anchor"/"$replacement"}" >"$target"
  [[ "$(cat "$target")" != "$content" ]] || infra mutation-not-applied
  return 0
}

mut001() {
  local root="$run_dir/mut001" log="$run_dir/mut001.log"
  stage "$root"
  replace_once "$root/internal/prompt/parser_validate.go" \
    'raw, ok := fields[findingsListKey]' \
    'raw, ok := fields[findingsListKey]
	if lc, has := fields["line_comments"]; has && !ok {
		raw, ok = lc, true
	}'
  replace_once "$root/internal/prompt/parser_line_comments.go" \
    'ignored := len(result.LineComments)' \
    'for _, c := range result.LineComments {
		result.Issues = append(result.Issues, types.ReviewIssue{File: c.Path, Line: c.Line, Severity: "warning", Message: c.Body})
	}
	ignored := len(result.LineComments)'
  ( cd "$root" && go vet ./internal/prompt ) >"$run_dir/mut001.vet" 2>&1 || { cat "$run_dir/mut001.vet" >&2; infra mutation-does-not-compile; }
  if go_test "$root" "$ac002_tests" "$log"; then
    fail mutation-survived
  fi
  grep -Eq -- '--- FAIL: (TestLineCommentsOnlyReplyIsInconclusive|TestLineCommentsBesideIssuesAreDroppedAndAnnounced)' "$log" \
    || { cat "$log" >&2; infra mutation-red-for-another-reason; }
  grep -E -- '--- FAIL' "$log" >&2 || true
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  MUT-001) mut001 ;;
  all) ac001; ac002; mut001 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
