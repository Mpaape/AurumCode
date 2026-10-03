#!/usr/bin/env bash
# AUR-581 acceptance: gitleaks is a registered engine (category secrets,
# origin gitleaks) that scans the reviewed commit range. A token added in an
# intermediate commit and removed before the head reaches the gate line and
# the audit record with origin, file:line and rule, and no output carries
# the value; a missing binary, a missing or unverifiable range, another
# version or an error logged at exit 0 is inconclusive, never zero findings;
# under a central policy inline gitleaks:allow does not suppress and a root
# .gitleaksignore is itself a finding; the review workflow installs gitleaks
# at the lock's digest and checks the lock's version; the secrets tutorial
# passes --check. The engine is driven by a fake runner (the sealed profile
# has neither gitleaks nor docker); the measured behavior of the pinned
# binary is recorded in docs/specs/AUR-581.md.
#
# Selectors:
#   all        AC-001..AC-003, MUT-001, MUT-002, then AC-004
#   AC-001     history leak in gate line and audit, no value; lock identity
#   AC-002     missing binary and every failure are inconclusive
#   AC-003     gitleaks:allow and .gitleaksignore under policy
#   AC-004     workflow installs the locked digest/version; tutorial --check
#   MUT-001    scanning the final tree only (no range) turns AC-001 RED
#   MUT-002    letting Secret reach the Finding turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-581'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
readonly lock_file='.board/bootstrap/locks/scanners.yml'
readonly workflow='.github/workflows/review.yml'
for input in go.mod go.sum cmd internal pkg Dockerfile "$lock_file" "$workflow" internal/scanner/gitleaks/invocation.go internal/scanner/gitleaks/report.go internal/scanner/engines/engines.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a581.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) and the lock
# the identity test reads to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root/${lock_file%/*}"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  cp "$repo_root/$lock_file" "$root/$lock_file"
  chmod -R u+w -- "$root"
}

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
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

readonly ac001_tests=(TestGitleaksSecretInPullRequestHistoryReachesGateWithoutValue TestGitleaksIdentityMatchesScannersLock TestGitleaksRegistrationAndOptions)
# The real review --base hands the resolved range to the engine; the gitleaks
# finding reaches the gate line and the audit; the identity enters the digest.
readonly review_tests=(TestBaseReviewHandsTheResolvedRangeToGitleaks TestEngineIdentityEntersTheEvidenceDigest)
readonly ac002_tests=(TestGitleaksFailuresAreInconclusive)
readonly ac003_tests=(TestGitleaksInlineAllowOnlyHonoredWithoutPolicy TestGitleaksIgnoreFileIsAFindingUnderPolicy)
readonly pkgs=(./internal/scanner/gitleaks/)
# The credential shapes of AUR-582's AC-005: no tracked file may carry one.
readonly credential_shapes='AIza[0-9A-Za-z_-]{20,}|ghp_[0-9A-Za-z]{20,}|xox[baprs]-[0-9A-Za-z-]{10,}|AKIA[0-9A-Z]{16}|sk_live_[0-9a-zA-Z]{10,}|glpat-[0-9A-Za-z_-]{10,}|BEGIN [A-Z ]*PRIVATE KEY'

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
  printf '%s/%s/pass\n' "$card" "$name"
}

run_ac001() {
  run_ac AC-001 "${ac001_tests[@]}"
  local root="$run_dir/root-AC-001-review" log="$run_dir/AC-001-review.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "${review_tests[@]}")" ./cmd/aurumcode/ || { cat "$log" >&2; fail review-test-failed; }
  require_pass "$log" "${review_tests[@]}"
  printf '%s/AC-001/review/pass\n' "$card"
  local hits path
  hits=''
  for path in internal/scanner cmd/aurumcode tests/acceptance/AUR-581.sh docs/tutorials/segredos.md demo/tutoriais/segredos Dockerfile; do
    [[ -e "$repo_root/$path" ]] || continue
    hits+="$(grep -rlE --exclude-dir=.estado -- "$credential_shapes" "$repo_root/$path" || true)"
  done
  [[ -z "$hits" ]] || { printf '%s\n' "$hits" >&2; fail credential-shaped-literal; }
  printf '%s/AC-001/no-credential-literal\n' "$card"
}

# AC-004: the review workflow pulls the lock's image by digest, reads the
# version from the lock and refuses another one, and checks out full history.
run_ac004() {
  local wf="$repo_root/$workflow"
  grep -Fq "image=\"\$(sed -n 's/^secrets_scanner_image: //p' \"\$lock\")\"" "$wf" || fail workflow-image-not-from-lock
  grep -Fq "version=\"\$(sed -n 's/^secrets_scanner_version: //p' \"\$lock\")\"" "$wf" || fail workflow-version-not-from-lock
  grep -Fq '@sha256:[0-9a-f]{64}$' "$wf" || fail workflow-digest-not-required
  grep -Fq 'docker pull "$image"' "$wf" || fail workflow-not-pulled-by-digest
  grep -Fq 'if [ "$got" != "$version" ]; then' "$wf" || fail workflow-version-not-checked
  grep -Fq 'fetch-depth: 0' "$wf" || fail workflow-history-cut
  grep -Eq '^secrets_scanner_image: docker\.io/zricethezav/gitleaks@sha256:[0-9a-f]{64}$' "$repo_root/$lock_file" || fail lock-not-by-digest
  local locked_image locked_version
  locked_image="$(sed -n 's/^secrets_scanner_image: //p' "$repo_root/$lock_file")"
  locked_version="$(sed -n 's/^secrets_scanner_version: //p' "$repo_root/$lock_file")"
  grep -Fxq "FROM $locked_image AS gitleaks" "$repo_root/Dockerfile" || fail dockerfile-image-not-the-lock
  grep -Fxq "RUN test \"\$(gitleaks version)\" = \"$locked_version\"" "$repo_root/Dockerfile" || fail dockerfile-version-not-checked
  printf '%s/AC-004/workflow/pass\n' "$card"
  [[ -x "$repo_root/demo/tutoriais/segredos/run.sh" || -f "$repo_root/demo/tutoriais/segredos/run.sh" ]] || fail tutorial-missing
  (cd "$repo_root" && bash demo/tutoriais/segredos/run.sh --check >"$run_dir/tut.log" 2>&1) || { cat "$run_dir/tut.log" >&2; fail tutorial-check-failed; }
  printf '%s/AC-004/tutorial/pass\n' "$card"
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

run_mut001() {
  local root="$run_dir/root-mut1" f
  stage "$root"
  f="$root/internal/scanner/gitleaks/invocation.go"
  replace_once "$f" '"--log-opts=" + r.Base + ".." + r.Head,' '// MUT-001: no commit range'
  replace_once "$f" '		"git",' '		"dir", // MUT-001: the final tree only'
  expect_red "$root" "$run_dir/mut1.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestGitleaksSecretInPullRequestHistoryReachesGateWithoutValue' "$run_dir/mut1.log" || fail mut001-wrong-test
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" f
  stage "$root"
  f="$root/internal/scanner/gitleaks/report.go"
  replace_once "$f" 'Commit      string `json:"Commit"`' 'Commit      string `json:"Commit"`; Secret string `json:"Secret"` // MUT-002'
  replace_once "$f" 'desc := strings.TrimSpace(l.Description)' 'desc := strings.TrimSpace(l.Description + " " + l.Secret) // MUT-002'
  expect_red "$root" "$run_dir/mut2.log" "${ac001_tests[@]}"
  grep -Eq -- "output carries the token" "$run_dir/mut2.log" || fail mut002-wrong-reason
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac001
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_mut001
    run_mut002
    run_ac004
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
