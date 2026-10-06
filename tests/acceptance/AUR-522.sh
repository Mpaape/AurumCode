#!/usr/bin/env bash
# AUR-522 acceptance: the review and the gate work on a repository in any
# language, and declare what they did not cover. Every selector runs real Go
# tests offline (provider fixture, no network): cmd/aurumcode's TestAUR522*
# drive `aurumcode review --base` over the corpus in
# internal/grammar/testdata/corpus, and the internal/grammar, internal/analyzer
# and internal/context packages carry their own proofs.
#
# Selectors:
#   all             every AC below, then the skeptical mutation
#   AC-001          corpus (Java, C#, Kotlin, PHP, Ruby, Terraform, CI YAML,
#                   Dockerfile, plus a file no grammar knows): each file is
#                   reviewed or declared not reviewed with its reason
#   AC-002          a policy finding in Terraform blocks like one in Go
#   AC-003          absent structural context is declared; the gate's
#                   inconclusive setting does not fire on the gap alone
#   AC-004          binary and generated files are declared, never approved, in
#                   --base and --pr (verified checkout, or no patch and no checkout)
#   AC-004-MUT-001  the --pr path passing no notices again (binary files then
#                   look reviewed) must turn the --pr binary test RED
#   AC-001-MUT-001  silently skipping a file of unknown extension (no
#                   declaration) must turn AC-001 RED; applied to a copy
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-522'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001|AC-004-MUT-001) ;;
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
for source in \
  internal/grammar/provider.go \
  internal/analyzer/language.go \
  internal/context/extract.go \
  cmd/aurumcode/structural_coverage.go \
  cmd/aurumcode/aur522_test.go \
  internal/grammar/testdata/corpus/Dockerfile \
  internal/grammar/testdata/corpus/main.tf; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a522.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
seed_root() {
  rm -rf "$run_dir/root"
  mkdir -p "$run_dir/root"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$run_dir/root/$source"
  done
  chmod -R u+w -- "$run_dir/root"
}
seed_root

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() {
  local pattern="$1" log="$2"; shift 2
  local status
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 400s -v "$@" -run "$pattern") >"$log" 2>&1
  status=$?
  set -e
  cat "$log" >&2
  return $status
}

expect_pass() {
  local pattern="$1" name="$2" log="$run_dir/$2.log" status
  set +e
  run_go_test "$pattern" "$log" ./cmd/aurumcode/
  status=$?
  set -e
  (( status == 0 )) || fail "go-test-exit:$status"
  grep -q "^--- PASS: $name " "$log" || fail "missing-pass:$name"
}

ac001=TestAUR522CorpusEveryFileReviewedOrDeclared
ac002=TestAUR522PolicyFindingInUnheuristicLanguageBlocks
ac003=TestAUR522MissingContextDeclaredAndNotInconclusiveByItself
ac004=TestAUR522BinaryAndGeneratedAreNeverApproved
ac004pr=(TestAUR522PRBinaryWithoutPatchIsNotReviewed TestAUR522PRGeneratedFileIsNotReviewed TestAUR522PRNoCheckoutAndNoPatchIsNotReviewed TestAUR522PRPlainTextStillApproves TestAUR522ModelCannotForgeOrClearTheRetentionKey)

# The grammar, analyzer and context packages prove the runtime-backed
# structure, binary detection by content and the unknown-language path.
run_package_proof() {
  local log="$run_dir/package_proof.log" status
  set +e
  run_go_test '.' "$log" ./internal/grammar/ ./internal/context/
  status=$?
  (( status == 0 )) || fail "package-proof-exit:$status"
  run_go_test 'Language|ExtractChangedFunctions|Blob|Classify|Notice' "$log" ./internal/analyzer/
  status=$?
  set -e
  (( status == 0 )) || fail "package-proof-exit:$status"
}

# AC-001-MUT-001: the defect is "skip a file of unknown extension silently":
# the declaration of files with no grammar is dropped. The mutation replaces
# the single detection line by a constant false, in the COPY only.
apply_mutation_skip_silently() {
  local target="$run_dir/root/cmd/aurumcode/structural_coverage.go"
  local anchor='if p.Detect(f.Path, nil) == "" {'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra mutation-anchor-not-unique
  sed -i 's/if p.Detect(f.Path, nil) == "" {/if p.Detect(f.Path, nil) == "" \&\& false { \/\/ AUR-522 MUT-001: unknown extension skipped silently/' "$target"
  grep -Fq 'MUT-001: unknown extension skipped silently' "$target" || infra mutation-not-applied
}

# AC-004-MUT-001: --pr feeding mergeReviewCoverage a nil notice list again.
apply_mutation_pr_no_notices() {
  local target="$run_dir/root/cmd/aurumcode/review_pr_analysis.go"
  # AUR-593 prepends the binary notices; the uninspected-file notices are
  # still the one call this mutation drops (same semantics: --pr no longer
  # reports a file it did not inspect).
  local anchor='p.binaryNotices...), uninspectedPRNotices(p.diff, p.verifiedDir)...), p.rawDiffFileCount, p.ignoredPaths)'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra pr-mutation-anchor-not-unique
  sed -i 's/uninspectedPRNotices(p.diff, p.verifiedDir)\.\.\./[]analyzer.DiffNotice(nil \/* AUR-522 MUT-001: pr notices dropped *\/).../' "$target"
  grep -Fq 'MUT-001: pr notices dropped' "$target" || infra pr-mutation-not-applied
}

check_pr_mutation_red() {
  local log="$1"
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used|\[build failed\]' "$log"; then
    fail 'pr-mutation-build-failure-not-behavioral'
  fi
  grep -Eq -- '^--- FAIL: TestAUR522PRBinaryWithoutPatchIsNotReviewed' "$log" || fail 'pr-mutation-survived'
}

check_mutation_red() {
  local log="$1"
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|\[build failed\]' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  grep -Eq -- "^--- FAIL: $ac001" "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) expect_pass "^$ac001\$" "$ac001"; printf '%s/%s/pass\n' "$card" "$selector" ;;
  AC-002) expect_pass "^$ac002\$" "$ac002"; printf '%s/%s/pass\n' "$card" "$selector" ;;
  AC-003) expect_pass "^$ac003\$" "$ac003"; printf '%s/%s/pass\n' "$card" "$selector" ;;
  AC-004)
    expect_pass '^TestAUR522(BinaryAndGenerated|PR|Model)' "$ac004"
    for name in "${ac004pr[@]}"; do grep -q "^--- PASS: $name " "$run_dir/$ac004.log" || fail "missing-pass:$name"; done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-004-MUT-001)
    apply_mutation_pr_no_notices
    log="$run_dir/mutation.log"
    run_go_test '^TestAUR522PR' "$log" ./cmd/aurumcode/ || true
    check_pr_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  AC-001-MUT-001)
    apply_mutation_skip_silently
    log="$run_dir/mutation.log"
    run_go_test "^$ac001\$" "$log" ./cmd/aurumcode/ || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    run_package_proof
    expect_pass '^TestAUR522' "$ac001"
    for name in "$ac002" "$ac003" "$ac004" "${ac004pr[@]}"; do
      grep -q "^--- PASS: $name " "$run_dir/$ac001.log" || fail "missing-pass:$name"
    done
    apply_mutation_skip_silently
    log="$run_dir/mutation.log"
    run_go_test "^$ac001\$" "$log" ./cmd/aurumcode/ || true
    check_mutation_red "$log"
    seed_root
    apply_mutation_pr_no_notices
    log="$run_dir/mutation_pr.log"
    run_go_test '^TestAUR522PR' "$log" ./cmd/aurumcode/ || true
    check_pr_mutation_red "$log"
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
