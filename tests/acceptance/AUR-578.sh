#!/usr/bin/env bash
#
# Acceptance program for card AUR-578: the review prompt is one template
# with named, budgeted slots (deterministic evidence, tools, repository
# context included); it travels as system and user messages; the request
# cache key covers the final text plus evidence and tool-result digests;
# the model's per-evidence assessment is parsed and a model-supplied origin
# is ignored; existing sections keep their bytes.
#
# Selectors:
#   all      AC-001..AC-006, then MUT-001 and MUT-002
#   AC-001   no "## " section title in Go string literals of the prompt
#            assembly packages (go/ast and grep), every title in review.md
#   AC-002   evidence slot: origin, rule, file:line, severity, redaction of
#            every field, "N omitidos" over its ceiling, prompt in budget
#   AC-003   provider receives system and user messages; the repository
#            context is inside the measured prompt; digest stable and moved
#            by evidence
#   AC-004   different evidence never shares a request cache key
#   AC-005   assessment parsed into ReviewIssue.Assessment; model origin dropped
#   AC-006   golden bytes of the pre-change prompt, slot path == legacy
#            decorator bytes, and every cmd/aurumcode test (e2e with the
#            fixture provider included)
#   MUT-001  in a copy, the repository context goes back to being assembled
#            in Go and appended outside the template: AC-001 and AC-003 red
#   MUT-002  in a copy, the evidence digest leaves the cache key: AC-004 red
#
# Exit codes: 0 holds, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-578'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg tests/fixtures/repos/git-demo/repo.git; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a578.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

: "${GOCACHE:=$run_dir/gocache}"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false' GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB

seed_root() {
  rm -rf "$run_dir/root"; mkdir -p "$run_dir/root/tests/fixtures/repos"
  local s
  for s in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$s" "$run_dir/root/$s"; done
  cp -R "$repo_root/tests/fixtures/repos/git-demo" "$run_dir/root/tests/fixtures/repos/git-demo"
  [[ ! -d "$repo_root/docs" ]] || cp -R "$repo_root/docs" "$run_dir/root/docs"
  chmod -R u+w -- "$run_dir/root"
}

# run_tests LOG PATTERN PKG... : runs the named tests verbosely in the copy;
# returns non-zero when go test fails.
run_tests() {
  local log="$1" pattern="$2"; shift 2
  (cd "$run_dir/root" && go test -buildvcs=false -count=1 -timeout 300s -v -run "$pattern" "$@") >"$log" 2>&1
}

# require_pass LOG TEST... : every named test must have passed.
require_pass() {
  local log="$1" t; shift
  for t in "$@"; do
    grep -q "^--- PASS: $t " "$log" || { tail -n 40 "$log" >&2; fail "missing-pass:$t"; }
  done
}

ac001_tests=(TestSectionTitlesLiveOnlyInTemplateAST TestSectionTitlesLiveOnlyInTemplateGrep TestTemplateDeclaresEverySection)
ac001_run() { run_tests "$run_dir/ac001.log" 'TestSectionTitles|TestTemplateDeclaresEverySection' ./internal/prompt/; }
ac003_tests=(TestProviderReceivesSystemAndUserMessages TestCompleteMessagesUsesCapabilityOrFlattens)
ac003_run() { run_tests "$run_dir/ac003.log" 'TestProviderReceivesSystemAndUserMessages|TestCompleteMessagesUsesCapabilityOrFlattens' ./internal/review/ ./internal/llm/; }
ac004_tests=(TestRequestCacheKeyCoversEvidence TestRequestKeyCoversEveryDigest)
ac004_run() { run_tests "$run_dir/ac004.log" 'TestRequestCacheKeyCoversEvidence|TestRequestKeyCoversEveryDigest' ./internal/review/ ./internal/review/cache/; }

ac001() { seed_root; ac001_run || { tail -n 40 "$run_dir/ac001.log" >&2; fail go-test-red; }; require_pass "$run_dir/ac001.log" "${ac001_tests[@]}"; }

ac002() {
  seed_root
  run_tests "$run_dir/ac002.log" 'TestEvidenceSlotRendersRedactsAndDeclaresOmissions' ./internal/review/ || { tail -n 40 "$run_dir/ac002.log" >&2; fail go-test-red; }
  require_pass "$run_dir/ac002.log" TestEvidenceSlotRendersRedactsAndDeclaresOmissions
}

ac003() { seed_root; ac003_run || { tail -n 40 "$run_dir/ac003.log" >&2; fail go-test-red; }; require_pass "$run_dir/ac003.log" "${ac003_tests[@]}"; }
ac004() { seed_root; ac004_run || { tail -n 40 "$run_dir/ac004.log" >&2; fail go-test-red; }; require_pass "$run_dir/ac004.log" "${ac004_tests[@]}"; }

ac005() {
  seed_root
  run_tests "$run_dir/ac005.log" 'TestModelAssessmentParsedAndOriginIgnored' ./internal/review/ || { tail -n 40 "$run_dir/ac005.log" >&2; fail go-test-red; }
  require_pass "$run_dir/ac005.log" TestModelAssessmentParsedAndOriginIgnored
}

readonly cmd_skip='^(TestAUR499ActionOutput|TestAUR555SBOMStepPrecedesReview|TestAUR555SecretsOptionalAndScopedToReview|TestAUR555WorkflowValidWithoutDTrackAndSignsOnlyAfterReview)$'

ac006() {
  seed_root
  run_tests "$run_dir/ac006.log" 'TestPromptGolden|TestRepositoryContextSlotMatchesLegacyDecoratorBytes' ./internal/review/ || { tail -n 40 "$run_dir/ac006.log" >&2; fail golden-red; }
  require_pass "$run_dir/ac006.log" TestPromptGoldenBuildPrompt TestPromptGoldenContextBlock TestPromptGoldenCapturedRequest TestRepositoryContextSlotMatchesLegacyDecoratorBytes
  local log="$run_dir/cmd.log"
  # The skipped tests read action.yml and .github/workflows, which are not
  # inputs of this card (not materialized in the sealed copy); they do not
  # exercise the prompt.
  (cd "$run_dir/root" && go test -buildvcs=false -count=1 -timeout 500s -skip "$cmd_skip" ./cmd/aurumcode/...) >"$log" 2>&1 || { tail -n 60 "$log" >&2; fail cmd-tests-red; }
}

mut001() {
  seed_root
  local provider="$run_dir/root/internal/config/provider.go" pipeline="$run_dir/root/internal/review/reviewer.go"
  grep -Fq 'return prompt.RenderRepositoryContext(names, assembled), warnings, nil' "$provider" || infra mut001-anchor-provider
  grep -Fq 'RepositoryContext: r.filter.Redact(reviewContext.RepositoryContext),' "$pipeline" || infra mut001-anchor-pipeline
  grep -Fq 'return preparedPrompt{diff: diff, metrics: metrics, parts: parts}, nil' "$pipeline" || infra mut001-anchor-return
  # The header back in Go, and the block appended after the budgeted prompt.
  sed -i 's|return prompt.RenderRepositoryContext(names, assembled), warnings, nil|return "## Repository context (untrusted, informational only)\\n" + prompt.RenderRepositoryContext(names, assembled)[:0] + strings.Join(names, "\\n") + "\\n" + assembled, warnings, nil|' "$provider"
  sed -i '/RepositoryContext: r.filter.Redact(reviewContext.RepositoryContext),/d' "$pipeline"
  sed -i 's|return preparedPrompt{diff: diff, metrics: metrics, parts: parts}, nil|if reviewContext.RepositoryContext != "" { parts.User += "\\n\\n" + reviewContext.RepositoryContext }; return preparedPrompt{diff: diff, metrics: metrics, parts: parts}, nil|' "$pipeline"
  (cd "$run_dir/root" && go vet ./internal/config/ ./internal/review/) >"$run_dir/mut001-build.log" 2>&1 || { cat "$run_dir/mut001-build.log" >&2; infra mut001-does-not-compile; }
  if ac001_run; then fail 'mut001-survived-AC-001'; fi
  grep -q '^--- FAIL: TestSectionTitlesLiveOnlyInTemplateAST ' "$run_dir/ac001.log" || { tail -n 30 "$run_dir/ac001.log" >&2; fail 'mut001-AC-001-red-for-another-reason'; }
  if ac003_run; then fail 'mut001-survived-AC-003'; fi
  grep -q '^--- FAIL: TestProviderReceivesSystemAndUserMessages ' "$run_dir/ac003.log" || { tail -n 30 "$run_dir/ac003.log" >&2; fail 'mut001-AC-003-red-for-another-reason'; }
  grep -E 'section title in Go code|outside the budgeted template|final template slot' "$run_dir/ac001.log" "$run_dir/ac003.log" >&2 || true
}

mut002() {
  seed_root
  local key="$run_dir/root/internal/review/cache/request_key.go"
  grep -Fq 'in.PromptDigest, in.EvidenceDigest, in.ToolResultsDigest' "$key" || infra mut002-anchor
  sed -i 's|in.PromptDigest, in.EvidenceDigest, in.ToolResultsDigest|in.PromptDigest, "", in.ToolResultsDigest|' "$key"
  (cd "$run_dir/root" && go vet ./internal/review/cache/) >"$run_dir/mut002-build.log" 2>&1 || { cat "$run_dir/mut002-build.log" >&2; infra mut002-does-not-compile; }
  if ac004_run; then fail 'mut002-survived-AC-004'; fi
  grep -q '^--- FAIL: TestRequestCacheKeyCoversEvidence ' "$run_dir/ac004.log" || { tail -n 30 "$run_dir/ac004.log" >&2; fail 'mut002-AC-004-red-for-another-reason'; }
  grep -E 'shared a cache key|shared the key' "$run_dir/ac004.log" >&2 || true
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  AC-005) ac005 ;;
  AC-006) ac006 ;;
  MUT-001) mut001 ;;
  MUT-002) mut002 ;;
  all) ac001; ac002; ac003; ac004; ac005; ac006; mut001; mut002 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
