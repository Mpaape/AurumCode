#!/usr/bin/env bash
# AUR-607 acceptance: with review.language pt-BR every sentence the review
# shows a person -- terminal, policy gate lines and status, parecer
# publication lines, skipped-file notices, secret findings and the MCP
# server's next step -- comes from the i18n catalog in Portuguese; English
# (or no language) keeps the earlier bytes.
#
# Selectors:
#   all      AC-001..AC-005, MUT-001, MUT-002
#   AC-001   no fixed English sentence in a pt-BR review; English unchanged
#   AC-002   each inconclusive reason is a pt-BR sentence with [code]
#   AC-003   the policy-gate status drops the PR number and cuts at a word
#   AC-004   a Gitleaks finding leads with the catalog label, tool text as detail
#   AC-005   review.yml's missing-secrets ::error:: says in pt-BR where to register them
#   MUT-001  a terminal line that skips the catalog turns AC-001 red
#   MUT-002  a raw reason code back in pt-BR turns AC-002 red
# AC-006 (tutorial recordings and captures re-recorded by this tree's image,
# `run.sh --check` green) is proven by the tutorials themselves, not here.
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-607'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 MUT-001 MUT-002'
if [[ " $known " != *" $selector "* ]]; then
  printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
  exit 64
fi

fail() {
  printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2
  exit 1
}
infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2
  exit 79
}

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
: "${GOCACHE:=$(mktemp -d)}"
export GOCACHE

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

readonly pkgs=(./internal/i18n ./internal/gate ./internal/gate/reasons ./internal/mcpserver ./cmd/aurumcode)

# stage copies the module (whole cmd, internal, pkg) into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  chmod -R u+w -- "$root"
}

# run_test runs the named tests of the packages in root.
run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}") >"$log" 2>&1
}

base="$work/base"
stage "$base"

# ac id pattern tests...: every named test must pass.
ac() {
  local id="$1" pattern="$2" log="$work/$1.log" name
  shift 2
  if ! run_test "$base" "$pattern" "$log"; then
    tail -n 30 "$log" >&2
    fail "test-failed"
  fi
  for name in "$@"; do
    grep -Eq -- "^--- PASS: $name " "$log" || { tail -n 30 "$log" >&2; fail "$id-missing-pass:$name"; }
  done
  printf '%s/%s/pass\n' "$card" "$id"
}

mutate() {
  local id="$1" file="$2" anchor="$3" expr="$4" pattern="$5" assertion="$6"
  local root="$work/$id" log="$work/$id.log"
  stage "$root"
  grep -Fq -- "$anchor" "$root/$file" || infra "$id-anchor-missing"
  sed -i "$expr" "$root/$file"
  if grep -Fq -- "$anchor" "$root/$file"; then
    infra "$id-not-applied"
  fi
  if run_test "$root" "$pattern" "$log"; then
    fail "$id-survived"
  fi
  if grep -Eq 'build failed|setup failed' "$log"; then
    tail -n 20 "$log" >&2
    infra "$id-did-not-compile"
  fi
  grep -Eq -- "^--- FAIL: $assertion" "$log" || {
    tail -n 20 "$log" >&2
    infra "$id-unexpected-failure"
  }
  rm -rf -- "$root"
  printf '%s/%s/rejected (%s)\n' "$card" "$id" "$assertion"
}

ac001() {
  ac AC-001 '^TestAUR607(BaseReviewSpeaksTheReviewLanguage|ReviewCheckAndCacheLinesFollowTheLanguage|PullRequestWithoutProviderSpeaksTheLanguage|ReappliedVerdictCount|RequiredQualityLineFollowsTheLanguage|NoProviderTextIsShortLinesInPortuguese|FailOnLineCountsInTheLanguage|TerminalTextsBothLanguages|SkippedFileNoticeFollowsTheLanguage|PublicationLinesFollowTheLanguage|NextStepFollowsTheLanguage|HowToFixFollowsTheLanguage|NewKeysExistInBothLocales|EnglishSingularIsThePluralWithOne|PortugueseHasNoParenthesizedPlural|LookupReportsMissingKeys)$' \
    TestAUR607BaseReviewSpeaksTheReviewLanguage TestAUR607RequiredQualityLineFollowsTheLanguage \
    TestAUR607PullRequestWithoutProviderSpeaksTheLanguage TestAUR607ReappliedVerdictCount \
    TestAUR607ReviewCheckAndCacheLinesFollowTheLanguage \
    TestAUR607NoProviderTextIsShortLinesInPortuguese TestAUR607FailOnLineCountsInTheLanguage \
    TestAUR607TerminalTextsBothLanguages TestAUR607SkippedFileNoticeFollowsTheLanguage \
    TestAUR607PublicationLinesFollowTheLanguage TestAUR607NextStepFollowsTheLanguage \
    TestAUR607HowToFixFollowsTheLanguage TestAUR607NewKeysExistInBothLocales \
    TestAUR607EnglishSingularIsThePluralWithOne TestAUR607PortugueseHasNoParenthesizedPlural \
    TestAUR607LookupReportsMissingKeys
}
ac002() {
  ac AC-002 '^TestAUR607(InconclusiveReasonIsASentenceWithItsCode|DTrackReasonFollowsTheLanguage|InconclusiveLineBothLanguages|ScannerInconclusiveLineFollowsTheLanguage|PolicyLineAndFirstBreach|ClosingLineNamesTheReasons|PolicyGateStatusInPortuguese)$' \
    TestAUR607InconclusiveReasonIsASentenceWithItsCode TestAUR607InconclusiveLineBothLanguages \
    TestAUR607ScannerInconclusiveLineFollowsTheLanguage TestAUR607PolicyLineAndFirstBreach \
    TestAUR607ClosingLineNamesTheReasons TestAUR607PolicyGateStatusInPortuguese \
    TestAUR607DTrackReasonFollowsTheLanguage
}
ac003() {
  ac AC-003 '^TestAUR607PolicyGateStatus(InPortuguese|CutsAtAWord|EnglishUnchanged)$' \
    TestAUR607PolicyGateStatusInPortuguese TestAUR607PolicyGateStatusCutsAtAWord TestAUR607PolicyGateStatusEnglishUnchanged
}
ac004() {
  ac AC-004 '^TestAUR607GitleaksFindingLeadsWithTheCatalogLabel$' TestAUR607GitleaksFindingLeadsWithTheCatalogLabel
}

# need <file> <literal> <count> fails unless the file carries the literal
# exactly count times.
need() {
  local n
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  n="$(grep -Fc -- "$2" "$repo_root/$1" || true)"
  [[ "$n" == "$3" ]] || fail "AC-005/$1/count-$n-of:$2"
}

ac005() {
  local wf=.github/workflows/review.yml
  need "$wf" '::error title=AurumCode sem provedor de modelo::Faltam os secrets LLM_API_KEY e LLM_BASE_URL;' 2
  need "$wf" 'Cadastre os dois em Settings > Secrets and variables > Actions do repositório (ou da organização)' 2
  need "$wf" 'em secrets: (ou com secrets: inherit)' 2
  need "$wf" 'o gate fica reprovado' 2
  need "$wf" 'AurumCode needs LLM_API_KEY' 0
  printf '%s/AC-005/pass\n' "$card"
}

mut001() {
  mutate MUT-001 cmd/aurumcode/review_base_analysis.go 'i18n.Text(b.reviewLanguage, "terminal.quality_skipped")' \
    's/i18n.Text(b.reviewLanguage, "terminal.quality_skipped")/"no LLM provider configured: quality review skipped; running deterministic analysis only"/' \
    '^TestAUR607BaseReviewSpeaksTheReviewLanguage$' 'TestAUR607BaseReviewSpeaksTheReviewLanguage'
}
mut002() {
  mutate MUT-002 internal/gate/reasons/reasons.go 'if text, ok := i18n.Lookup(language, reasonKeyPrefix+code); ok {' \
    's/if text, ok := i18n.Lookup(language, reasonKeyPrefix+code); ok {/if text, ok := code, true; ok {/' \
    '^TestAUR607InconclusiveReasonIsASentenceWithItsCode$' 'TestAUR607InconclusiveReasonIsASentenceWithItsCode'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 ac005 mut001 mut002; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi
