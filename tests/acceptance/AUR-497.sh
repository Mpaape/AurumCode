#!/usr/bin/env bash
# AUR-497: every capability card embeds the REAL output of the binary against a
# tracked fixture, plus a named proof. This script re-runs each cited command
# and compares stdout byte-for-byte with the <pre class="observed"> block in
# docs/site/index.html. A mutation that edits the prose, drops a proof line, or
# restores the false token-cost claim must make it fail.
set -uo pipefail
selector="${1:-all}"
command -v go >/dev/null 2>&1 || exit 79
command -v git >/dev/null 2>&1 || exit 79

repo_root="$(cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
HTML="$repo_root/docs/site/index.html"
FX="$repo_root/tests/fixtures/site-evidence"

scratch="$(mktemp -d)"
trap 'chmod -R u+w -- "$scratch" 2>/dev/null || true; rm -rf -- "$scratch"' EXIT

( cd "$repo_root" && go build -buildvcs=false -o "$scratch/aurumcode" ./cmd/aurumcode ) || exit 1
BIN="$scratch/aurumcode"

work="$scratch/repo"; cache="$scratch/cache"; mkdir -p "$work" "$cache"
( cd "$work" \
  && git init -q && git config user.email t@t && git config user.name t \
  && cp "$FX"/base/*.py . && git add -A && git commit -qm base \
  && cp "$FX"/after/*.py . && git add -A && git commit -qm change ) || exit 1
export AURUMCODE_LLM_FIXTURE="$FX/review.json" XDG_CACHE_HOME="$cache"

# observed N: print the Nth <pre class="observed"> block, byte-for-byte.
observed() {
  awk -v want="$1" '
    index($0, "<pre class=\"observed\"") {
      n++
      s = $0
      sub(/^.*<pre class="observed"[^>]*>/, "", s)
      if (index(s, "</pre>")) { sub(/<\/pre>.*$/, "", s); if (n==want) print s; inblk=0; next }
      inblk = (n==want) ? 1 : 0
      if (n==want && s != "") print s
      next
    }
    inblk {
      if (index($0, "</pre>")) { sub(/<\/pre>.*$/, "", $0); if ($0 != "") print; inblk=0; next }
      print
    }
  ' "$HTML"
}

fail() { printf 'AUR-497/%s\n' "$1" >&2; exit 1; }

card() { # $1 index, $2 label, $3 command (run with cwd=$work)
  local n="$1" label="$2" cmd="$3"
  local got; got="$(cd "$work" && eval "$cmd")" || fail "$label/command"
  local want; want="$(observed "$n")"
  if [[ "$got" != "$want" ]]; then
    printf 'AUR-497/%s/output-mismatch\n--- got ---\n%s\n--- want ---\n%s\n' "$label" "$got" "$want" >&2
    exit 1
  fi
  printf 'AUR-497/%s/pass\n' "$label"
}

# The eight cited commands, in the order their cards appear.
FENCE="$(printf '\140\140\140')"   # three backticks, built without a literal one
export FENCE
C1='$BIN review --base HEAD~1 --seguranca 2>/dev/null'
C2='$BIN review --base HEAD~1 2>/dev/null | awk -v f="$FENCE" "\$0==f{print; exit} {print}"'
C3='AURUMCODE_PROMPT_CAPTURE="$work/p1.txt" $BIN review --base HEAD~1 >/dev/null 2>&1; grep -o "\"dependents\":\\[[^]]*\\]" "$work/p1.txt"'
C4='$BIN fix < '"$FX"'/review.json'
C5='mkdir -p .aurumcode && printf "review:\n  memory: local\n" > .aurumcode/config.yml
    $BIN review --base HEAD~1 >/dev/null 2>&1
    AURUMCODE_PROMPT_CAPTURE="$work/p2.txt" $BIN review --base HEAD~1 >/dev/null 2>&1
    rm -rf .aurumcode
    sed -n "/## Review memory/,\$p" "$work/p2.txt" | sed -n "2p" | grep -o "\"id\":\"[^\"]*\""'
C6='$BIN fix < '"$FX"'/fix-suggestions.json'
C7='{ $BIN review --base HEAD~1 --limite 0.01 2>&1 >/dev/null || true; } | grep -i aurumcode'
C8='$BIN review --base HEAD~1 --modelo local 2>&1 >/dev/null | grep review'

export work BIN

# AC-003: every cited proof path and function exists.
proofs=(
  "internal/analysis/analysis_test.go::TestAnalyzeTable"
  "internal/render/mermaid_test.go::TestMermaidHeaderEdgesAndFiles"
  "internal/context/resolver_test.go::TestResolvePython"
  "cmd/aurumcode/fix_test.go::TestRunFixAcceptsReviewResponseShape"
  "internal/memory/memory_test.go::TestLocalRoundTrip"
  "cmd/aurumcode/fix_test.go::TestRunFixBuildsPatchFromSuggestionsArray"
  "internal/llm/cost/costtracker_test.go::TestCostTrackerAllow"
  "cmd/aurumcode/redaction_wiring_test.go::TestStderrLinesSurviveTheRedactionWriter"
)
check_proofs() {
  local p file fn
  for p in "${proofs[@]}"; do
    file="${p%%::*}"; fn="${p##*::}"
    [ -f "$repo_root/$file" ] || fail "proof-missing-file:$file"
    grep -q "^func $fn(" "$repo_root/$file" || fail "proof-missing-func:$p"
    grep -qF "$p" "$HTML" || fail "proof-not-cited:$p"
  done
}

# AC-004: the false token-cost claim must not be back.
check_cost_claim() {
  if grep -q "Entra no custo de tokens do seu provedor" "$HTML"; then fail "AC-004/stale-cost-claim"; fi
  grep -q "Não gastam tokens de LLM" "$HTML" || fail "AC-004/missing-corrected-claim"
}

case "$selector" in
  all)
    check_proofs
    check_cost_claim
    card 1 analise "$C1"
    card 2 resumo "$C2"
    card 3 contexto "$C3"
    card 4 sugestao "$C4"
    card 5 memoria "$C5"
    card 6 fix "$C6"
    card 7 custo "$C7"
    card 8 modelo "$C8"
    printf 'AUR-497/all/pass\n'
    ;;
  AC-001|AC-002|AC-003|AC-004|AC-005)
    check_proofs
    card 1 analise "$C1"
    card 2 resumo "$C2"
    card 3 contexto "$C3"
    card 4 sugestao "$C4"
    card 5 memoria "$C5"
    card 6 fix "$C6"
    card 7 custo "$C7"
    card 8 modelo "$C8"
    ;;
  MUT-001)  # edit an embedded output without changing the fixture -> AC-001 fails
    sed -i '0,/## Code Review Summary/s//## Code Review Summary (edited)/' "$HTML"
    card 1 analise "$C1"
    fail "MUT-001/survived" ;;
  MUT-002)  # drop one proof line -> AC-003 fails
    sed -i '/TestResolvePython/d' "$HTML"
    check_proofs
    fail "MUT-002/survived" ;;
  MUT-003)  # restore the false cost claim -> AC-004 fails
    sed -i 's/Não gastam tokens de LLM/Entra no custo de tokens do seu provedor/' "$HTML"
    check_cost_claim
    fail "MUT-003/survived" ;;
  *) exit 64 ;;
esac
