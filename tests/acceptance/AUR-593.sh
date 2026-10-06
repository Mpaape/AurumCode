#!/usr/bin/env bash
# AUR-593 acceptance: the minimal configuration simply works. An ignored
# path and a binary file are declared "ignored", never partial coverage; a
# sast/lint scanner judges only the lines the reviewed range added; a skill
# directory is citable by the gate with a stable id and reaches the prompt
# once; the review image carries the pinned Go toolchain for govet.
#
# Selectors:
#   all        AC-001..AC-005, then MUT-001..MUT-004
#   AC-001     tests/** (ignored) + a PNG under inconclusive: block: not
#              inconclusive by coverage, the ignored files listed; a
#              generated file, and a .sh/.js/.yml/shebang file with a NUL
#              byte, still partial with approval withheld (fail-closed)
#   AC-002     pre-existing Semgrep finding in an untouched file: gate
#              passes; a finding on an added line: gate fails; recovered
#              parse errors outside the change are not invalid output
#   AC-003     directory skill cited with id <dir>#<slug>, text once
#   AC-004     review image: Go copied from the digest-pinned golang stage;
#              govet runs (finding on an added line, missing go unavailable)
#   AC-005     the workflow prefetches modules and mounts them for govet;
#              AC-001..AC-003 together (the real-tree run is in the spec)
#   MUT-001    counting ignored files as partial again turns AC-001 RED
#   MUT-002    Semgrep keeping findings off the change turns AC-002 RED
#   MUT-003    any binary content declared ignored (no format catalog): a
#              script with one NUL byte would pass; turns AC-001 RED
#   MUT-004    the format signature not checked: a script with a NUL byte
#              renamed to .png would pass; turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-593'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001|MUT-002|MUT-003|MUT-004) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg Dockerfile .github/workflows/review.yml cmd/aurumcode/aur593_test.go cmd/aurumcode/review_coverage.go internal/scanner/added_lines.go internal/scanner/semgrep/engine.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a593.XXXXXX")" || infra mktemp
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
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
  chmod -R u+w -- "$root"
}

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
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

readonly ac001_tests=(TestIgnoredAndBinaryAreDeclaredNotPartial TestGeneratedFileStillPartialUnderBlock TestNULInCodeIsNeverDeclaredBinary TestRealPNGUpperCaseIsDeclaredIgnored TestAUR522PRBinaryIsDeclaredIgnored TestAUR522PRBinaryWithoutPatchIsNotReviewed TestAUR522PRGeneratedFileIsNotReviewed)
readonly ac002_cmd=(TestSemgrepJudgesOnlyTheChange)
readonly ac002_engine=(TestSemgrepKeepsOnlyFindingsOnAddedLines TestSemgrepRecoveredParseErrorOutsideTheChangeIsNotInvalid TestSemgrepErrorsOnTheChangeStayInvalid TestSemgrepWithoutRangeFails)
readonly ac003_tests=(TestDirectorySkillIsCitableOnce)
readonly ac004_tests=(TestRealVetFindingOnAddedLine TestRealVetIgnoresUntouchedLines TestMissingGoIsUnavailable TestNoRangeIsInconclusive)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

# run_tests name pkg tests...: stage, run, require every named test to pass.
run_tests() {
  local name="$1" pkg="$2"; shift 2
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  local t
  for t in "$@"; do grep -Eq -- "^--- PASS: ${t} " "$log" || { cat "$log" >&2; fail "missing-pass:$t"; }; done
}

# expect_red root log pkg tests...: the mutated copy must compile and turn
# at least one named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2" pkg="$3"; shift 3
  if go_test "$root" "$log" "$(pattern_of "$@")" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,3p' >&2
}

run_ac001() {
  run_tests AC-001 ./cmd/aurumcode/ "${ac001_tests[@]}"
  printf '%s/AC-001/pass\n' "$card"
}

run_ac002() {
  run_tests AC-002 ./cmd/aurumcode/ "${ac002_cmd[@]}"
  run_tests AC-002 ./internal/scanner/semgrep/ "${ac002_engine[@]}"
  printf '%s/AC-002/pass\n' "$card"
}

run_ac003() {
  run_tests AC-003 ./cmd/aurumcode/ "${ac003_tests[@]}"
  printf '%s/AC-003/pass\n' "$card"
}

run_ac004() {
  local dockerfile="$repo_root/Dockerfile"
  grep -Eq '^FROM golang:[^ ]+@sha256:[0-9a-f]{64} AS builder$' "$dockerfile" || fail builder-not-digest-pinned
  grep -Fxq 'COPY --from=builder /usr/local/go /usr/local/go' "$dockerfile" || fail go-toolchain-not-copied
  grep -Eq '^ENV PATH=/usr/local/go/bin:' "$dockerfile" || fail go-not-on-path
  local want
  want="$(awk '$1 == "go" { print $2; exit }' "$repo_root/go.mod")"
  grep -Eq "^FROM golang:${want//./\\.}[.0-9]*-" "$dockerfile" || fail "toolchain-not-the-project-go:$want"
  run_tests AC-004 ./internal/scanner/govet/ "${ac004_tests[@]}"
  printf '%s/AC-004/pass\n' "$card"
}

run_ac005() {
  local wf="$repo_root/.github/workflows/review.yml"
  grep -Fq 'aurumcode-review mod download' "$wf" || fail no-module-prefetch
  grep -Fq -- '-e GOTOOLCHAIN=local' "$wf" || fail prefetch-may-download-toolchain
  grep -Fq -- '-v "${RUNNER_TEMP}/aurumcode-gomod:/go/pkg/mod"' "$wf" || fail module-cache-not-mounted
  run_tests AC-005 ./cmd/aurumcode/ "${ac001_tests[@]}" "${ac002_cmd[@]}" "${ac003_tests[@]}"
  printf '%s/AC-005/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/cmd/aurumcode/review_coverage.go" \
    'return c.Partial > 0 || c.uncovered() > 0' \
    'return c.Partial > 0 || c.uncovered() > 0 || c.Ignored > 0 /* MUT-001 */'
  expect_red "$root" "$run_dir/mut1.log" ./cmd/aurumcode/ TestIgnoredAndBinaryAreDeclaredNotPartial
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/scanner/semgrep/engine.go" \
    'Findings: added.Keep(findings)' \
    'Findings: findings /* MUT-002 */'
  expect_red "$root" "$run_dir/mut2.log" ./cmd/aurumcode/ "${ac002_cmd[@]}"
  printf '%s/MUT-002/rejected\n' "$card"
}

run_mut003() {
  local root="$run_dir/root-mut3"
  stage "$root"
  replace_once "$root/cmd/aurumcode/structural_coverage.go" \
    'return n.Reason == analyzer.NoticeReasonBinary && n.DeclaredFormat' \
    'return n.Reason == analyzer.NoticeReasonBinary || n.DeclaredFormat /* MUT-003 */'
  expect_red "$root" "$run_dir/mut3.log" ./cmd/aurumcode/ TestNULInCodeIsNeverDeclaredBinary
  printf '%s/MUT-003/rejected\n' "$card"
}

run_mut004() {
  local root="$run_dir/root-mut4"
  stage "$root"
  replace_once "$root/internal/grammar/binary_formats.go" \
    'if signatureMatches(content, sig) {' \
    'if signatureMatches(content, sig) || len(sig.bytes) > 0 /* MUT-004 */ {'
  expect_red "$root" "$run_dir/mut4.log" ./cmd/aurumcode/ TestNULInCodeIsNeverDeclaredBinary
  grep -Eq -- 'payload\.(png|PNG)' "$run_dir/mut4.log" || { cat "$run_dir/mut4.log" >&2; fail mut004-wrong-case; }
  printf '%s/MUT-004/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  AC-005) run_ac005 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  MUT-004) run_mut004 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_ac004
    run_ac005
    run_mut001
    run_mut002
    run_mut003
    run_mut004
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
