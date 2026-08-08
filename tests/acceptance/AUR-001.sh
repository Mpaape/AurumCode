#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-001'
readonly scenario='AC-001'
readonly max_rows=10000
readonly max_bytes=$((64 * 1024 * 1024))

selector="${1:-AC-001}"
case "$selector" in
  AC-001|IntegrationAUR001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 3
}
fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 1
}
# A capability this acceptance was not granted is never a pass and never a functional
# rejection. It exits with its own code so a caller can tell "not proven" from "wrong".
inconclusive() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 79
}
emitted() {
  [[ -n $1 ]] || return 0
  fail "${1%%$'\t'*}" "${1#*$'\t'}"
}

for tool in awk sha256sum wc find sort readlink tr grep; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readonly ledger='.board/research/legacy-disposition.md'
readonly inventory='.board/research/legacy-files.tsv'
readonly claims='tests/specs/AUR-001/claims.yaml'

# ---------------------------------------------------------------------------------
# Embedded tracked-path seal.
#
# The sealed container materializes only the card's own `paths` (and `read_paths`, which
# AUR-001 declares empty). The list below is retained as a consistency fixture, but it is
# never sufficient for a pass: a host run compares it with the independent Git index, while
# a container without that index is inconclusive.
#
# Scope: every tracked path except the ones under the card's `forbidden_paths`
# (`.git`, `.env`, `credentials`, `secrets`, `.board/cards`), which this card is not
# permitted to inventory. Regenerate with:
#
#   LC_ALL=C git ls-files \
#     | LC_ALL=C grep -vE '^(\.git|\.env|credentials|secrets|\.board/cards)(/|$)' \
#     | LC_ALL=C sort
#
# ---------------------------------------------------------------------------------
readonly seal_exclusion='^(\.git|\.env|credentials|secrets|\.board/cards)(/|$)'
IFS= read -r -d '' AURUM_TRACKED_SEAL <<'SEAL' || true
.agents/skills/ai-memory-durable-pages/SKILL.md
.agents/skills/ai-memory-handoff/SKILL.md
.agents/skills/ai-memory-learning-maintenance/SKILL.md
.agents/skills/ai-memory-retrieval/SKILL.md
.agents/skills/ai-memory-routing-install/SKILL.md
.agents/skills/escritorio/SKILL.md
.agents/skills/escritorio/frota.sh
.ai-memory.toml
.aurumcode/.gitignore
.aurumcode/Gemfile
.aurumcode/_config.yml
.aurumcode/bash/action-entrypoint.sh.md
.aurumcode/bash/build-docs-site.sh.md
.aurumcode/bash/bulk-enable-repos.sh.md
.aurumcode/bash/generate-code-docs.sh.md
.aurumcode/bash/generate-docs-simple.sh.md
.aurumcode/bash/generate-enhanced-docs.sh.md
.aurumcode/bash/run-docs-pipeline.sh.md
.aurumcode/bash/run-live-demo.sh.md
.aurumcode/bash/setup-github-app.sh.md
.aurumcode/bash/test-jekyll.sh.md
.aurumcode/index.md
.aurumcode/iso25010-weights.yml
.aurumcode/prompts/changelog-generation.md
.aurumcode/prompts/documentation-generation.md
.aurumcode/prompts/documentation.md
.aurumcode/prompts/documentation/welcome-page.md
.aurumcode/prompts/review.md
.aurumcode/prompts/summary.md
.aurumcode/prompts/test.md
.aurumcode/rules/performance.yml
.aurumcode/rules/quality.yml
.aurumcode/rules/security.yml
.board/CARD_TEMPLATE.md
.board/INDEX.md
.board/KANBAN.md
.board/README.md
.board/REVIEW_PROTOCOL.md
.board/bin/oci-run
.board/decisions/.gitkeep
.board/decisions/ADR-0001-optional-local-memory.md
.board/evidence/.gitkeep
.board/requirements/REQUIREMENTS.md
.board/research/.gitkeep
.board/research/code-review-standards.md
.board/research/legacy-disposition.md
.board/research/memory-design.md
.board/tests/validator-mutants.sh
.board/validate.sh
.claude/agents/adversarial-reviewer.md
.claude/settings.local.json
.claude/skills/ai-memory-durable-pages/SKILL.md
.claude/skills/ai-memory-handoff/SKILL.md
.claude/skills/ai-memory-learning-maintenance/SKILL.md
.claude/skills/ai-memory-retrieval/SKILL.md
.claude/skills/ai-memory-routing-install/SKILL.md
.claude/skills/escritorio/SKILL.md
.claude/skills/escritorio/frota.sh
.cursor/mcp.json
.cursor/rules/cursor_rules.mdc
.cursor/rules/self_improve.mdc
.cursor/rules/taskmaster/dev_workflow.mdc
.cursor/rules/taskmaster/taskmaster.mdc
.docker/README.md
.docker/docs.Dockerfile
.dockerignore
.env copy.example
.env.example
.gemini/settings.json
.github/actions/aurumcode-docs/README.md
.github/actions/aurumcode-docs/action.yml
.github/instructions/dev_workflow.instructions.md
.github/instructions/self_improve.instructions.md
.github/instructions/taskmaster.instructions.md
.github/instructions/vscode_rules.instructions.md
.github/workflows/demo-external-usage.yml
.github/workflows/documentation.yml
.github/workflows/examples/all-pipelines.yml
.github/workflows/examples/code-review.yml
.github/workflows/examples/documentation.yml
.github/workflows/examples/qa-testing.yml
.gitignore
.mcp.json
.taskmaster/CLAUDE.md
.taskmaster/config.json
.taskmaster/docs/IMPLEMENTATION_PLAN.md
.taskmaster/docs/SESSION_SUMMARY.md
.taskmaster/docs/doc-system-gap-analysis.md
.taskmaster/docs/prd-documentation-extractors.txt
.taskmaster/docs/prd.txt
.taskmaster/state.json
.taskmaster/tasks/task_001.txt
.taskmaster/tasks/task_002.txt
.taskmaster/tasks/task_003.txt
.taskmaster/tasks/task_004.txt
.taskmaster/tasks/task_005.txt
.taskmaster/tasks/task_006.txt
.taskmaster/tasks/task_007.txt
.taskmaster/tasks/task_008.txt
.taskmaster/tasks/task_009.txt
.taskmaster/tasks/task_010.txt
.taskmaster/tasks/tasks.json
.taskmaster/templates/example_prd.txt
.taskmaster/templates/example_prd_rpg.txt
ACTION_USAGE.md
AGENTS.md
CHANGELOG.md
CLAUDE.md
Dockerfile
GEMINI.md
Gemfile
LITELLM_QUICKSTART.md
Makefile
PAGES_SETUP.md
README.md
RUN_DOCS_PIPELINE.md
SETUP_GUIDE.md
_api/index.md
_api/llm.md
_api/pipeline.md
_api/review.md
_config.yml
action.yml
cmd/regenerate-docs/main.go
configs/.aurumcode/config.example.yml
configs/default-config.yml
demo/bad-code.go
demo/good-code.go
docker-compose.test.yml
docker-compose.yml
docs/README.md
docs/getting-started.md
generate-docs-simple.sh
go.mod
go.sum
index.md
internal/documentation/extractors/bash/extractor.go
internal/documentation/extractors/bash/extractor_test.go
internal/documentation/extractors/cpp/extractor.go
internal/documentation/extractors/cpp/extractor_test.go
internal/documentation/extractors/csharp/extractor.go
internal/documentation/extractors/csharp/extractor_test.go
internal/documentation/extractors/detector.go
internal/documentation/extractors/detector_test.go
internal/documentation/extractors/go/extractor.go
internal/documentation/extractors/go/extractor_test.go
internal/documentation/extractors/javascript/extractor.go
internal/documentation/extractors/javascript/extractor_test.go
internal/documentation/extractors/powershell/extractor.go
internal/documentation/extractors/powershell/extractor_test.go
internal/documentation/extractors/python/extractor.go
internal/documentation/extractors/python/extractor_test.go
internal/documentation/extractors/registry.go
internal/documentation/extractors/registry_test.go
internal/documentation/extractors/rust/extractor.go
internal/documentation/extractors/rust/extractor_test.go
internal/documentation/extractors/types.go
internal/documentation/incremental/cache.go
internal/documentation/incremental/detector.go
internal/documentation/incremental/incremental_test.go
internal/documentation/incremental/manager.go
internal/documentation/normalizer/frontmatter.go
internal/documentation/normalizer/normalizer.go
internal/documentation/normalizer/normalizer_test.go
internal/documentation/site/builder.go
internal/documentation/site/builder_test.go
internal/documentation/site/jekyll.go
internal/documentation/site/jekyll_test.go
internal/documentation/site/pagefind.go
internal/documentation/site/pagefind_test.go
internal/documentation/site/runner.go
internal/documentation/site/runner_test.go
internal/documentation/site/test_helpers_test.go
internal/documentation/site/types.go
internal/documentation/welcome/generator.go
internal/documentation/welcome/generator_test.go
internal/llm/cost/costtracker.go
internal/llm/cost/costtracker_test.go
internal/llm/estimator.go
internal/llm/httpbase/client.go
internal/llm/httpbase/client_test.go
internal/llm/orchestrator.go
internal/llm/orchestrator_test.go
internal/llm/provider/anthropic/anthropic.go
internal/llm/provider/litellm/provider.go
internal/llm/provider/litellm/provider_test.go
internal/llm/provider/ollama/ollama.go
internal/llm/provider/openai/openai.go
internal/llm/provider/openai/openai_test.go
internal/llm/types.go
internal/llm/types_test.go
internal/pipeline/extractor_pipeline.go
internal/pipeline/extractor_pipeline_test.go
pages-fix.md
pkg/types/config.go
pkg/types/config_test.go
pkg/types/providers.go
pkg/types/types.go
pkg/types/types_test.go
run-docs-pipeline.bat
run-docs-pipeline.sh
scripts/action-entrypoint.sh
scripts/build-docs-site.sh
scripts/bulk-enable-repos.sh
scripts/generate-code-docs.sh
scripts/generate-demo-page.go
scripts/generate-enhanced-docs.sh
scripts/run-live-demo.sh
scripts/setup-github-app.sh
test-jekyll.sh
tests/acceptance/AUR-001.sh
tests/acceptance/AUR-010.sh
tests/acceptance/AUR-014.sh
tests/acceptance/AUR-015.sh
tests/acceptance/AUR-016.sh
tests/acceptance/AUR-017.sh
tests/acceptance/AUR-018.sh
tests/acceptance/AUR-019.sh
tests/acceptance/AUR-020.sh
tests/acceptance/AUR-021.sh
tests/acceptance/AUR-203.sh
tests/acceptance/AUR-233.sh
tests/acceptance/AUR-312.sh
tests/acceptance/AUR-313.sh
tests/acceptance/AUR-333.sh
tests/acceptance/AUR-334.sh
tests/acceptance/AUR-335.sh
tests/acceptance/AUR-336.sh
tests/acceptance/AUR-337.sh
tests/acceptance/AUR-338.sh
tests/acceptance/AUR-339.sh
tests/acceptance/AUR-340.sh
tests/acceptance/AUR-351.sh
tests/acceptance/AUR-352.sh
tests/acceptance/AUR-353.sh
tests/acceptance/AUR-354.sh
tests/acceptance/AUR-355.sh
tests/acceptance/AUR-356.sh
tests/acceptance/AUR-357.sh
tests/acceptance/AUR-358.sh
tests/acceptance/AUR-359.sh
tests/acceptance/AUR-360.sh
tests/acceptance/AUR-361.sh
tests/acceptance/AUR-362.sh
tests/acceptance/AUR-363.sh
tests/acceptance/AUR-364.sh
tests/acceptance/AUR-365.sh
tests/acceptance/AUR-366.sh
tests/acceptance/AUR-367.sh
tests/acceptance/AUR-368.sh
tests/acceptance/AUR-369.sh
tests/acceptance/AUR-370.sh
tests/acceptance/AUR-371.sh
tests/acceptance/AUR-372.sh
tests/acceptance/AUR-373.sh
tests/acceptance/AUR-374.sh
tests/acceptance/AUR-375.sh
tests/acceptance/AUR-376.sh
tests/acceptance/AUR-377.sh
tests/acceptance/AUR-378.sh
tests/acceptance/AUR-379.sh
tests/acceptance/AUR-380.sh
tests/acceptance/AUR-381.sh
tests/acceptance/AUR-382.sh
tests/acceptance/AUR-383.sh
tests/acceptance/AUR-384.sh
tests/acceptance/AUR-385.sh
tests/acceptance/AUR-386.sh
tests/acceptance/AUR-387.sh
tests/acceptance/AUR-388.sh
tests/acceptance/AUR-389.sh
tests/acceptance/AUR-390.sh
tests/acceptance/AUR-391.sh
tests/fixtures/repo1/.aurumcode/config.yml
tests/fixtures/repo1/.aurumcode/prompts/code-review/custom.md
tests/fixtures/repo1/.aurumcode/rules/standards.yml
tests/security/redact-run-docs-pipeline.sh
SEAL
export AURUM_TRACKED_SEAL

[[ -f $inventory && ! -L $inventory ]] ||
  fail unmapped-tracked-file "per-file legacy inventory absent: $inventory"
[[ -r $inventory ]] || infra "inventory unreadable: $inventory"

bytes="$(wc -c <"$inventory")" || infra 'inventory size unreadable'
((bytes <= max_bytes)) || fail limit-exceeded "inventory exceeds 64 MiB: $bytes bytes"

[[ -f $ledger && ! -L $ledger ]] ||
  fail unmapped-tracked-file "disposition ledger absent: $ledger"
[[ -r $ledger ]] || infra "ledger unreadable: $ledger"

[[ -f $claims && ! -L $claims ]] ||
  fail claim-without-proof "public claim manifest absent: $claims"
[[ -r $claims ]] || infra "claim manifest unreadable: $claims"

verdict="$(
  awk -F '\t' -v max="$max_rows" '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    NR == 1 {
      if (NF != 4 || $1 != "path" || $2 != "type" || $3 != "mode" || $4 != "digest")
        emit("inventory-schema-invalid", "header must be path/type/mode/digest")
      next
    }
    { rows++ }
    rows > max { emit("limit-exceeded", "inventory exceeds " max " tracked paths") }
    NF != 4 { emit("inventory-schema-invalid", "row " NR " has " NF " fields, want 4") }
    $1 == "" || $1 ~ /^\// || $1 ~ /(^|\/)\.\.(\/|$)/ || $1 ~ /\\/ || $1 ~ /[[:cntrl:]]/ {
      emit("unsafe-path", "row " NR " declares an unusable path")
    }
    $2 != "file" && $2 != "symlink" { emit("special-file", "row " NR " declares type " $2) }
    $3 !~ /^[0-7]{6}$/ { emit("inventory-schema-invalid", "row " NR " has a non-octal mode") }
    ($2 == "file" && $3 != "100644" && $3 != "100755") ||
    ($2 == "symlink" && $3 != "120000") {
      emit("inventory-schema-invalid", "row " NR " mixes type " $2 " with mode " $3)
    }
    $4 !~ /^sha256:[0-9a-f]{64}$/ { emit("inventory-schema-invalid", "row " NR " lacks a sha256 digest") }
    seen[$1]++ { emit("duplicate-path", "path listed twice: " $1) }
    previous != "" && $1 <= previous { emit("order-not-lexical", "row " NR " breaks lexical order") }
    { previous = $1 }
    END {
      if (!done && rows == 0) print "unmapped-tracked-file\tinventory declares no tracked file"
    }
  ' "$inventory"
)" || infra 'inventory lint failed to run'
emitted "$verdict"

# The embedded list is a consistency fixture, not an authority. A host run must derive the
# tracked set from Git; an OCI run that cannot receive that independent source is inconclusive.
if ! command -v git >/dev/null 2>&1; then
  inconclusive independent-tracked-source 'git is unavailable; the embedded seal cannot authorize a pass'
fi
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  inconclusive independent-tracked-source 'the checkout has no readable Git index; the embedded seal cannot authorize a pass'
fi
AURUM_LIVE_TRACKED="$({
  git ls-files -z || exit 1
} | tr '\0' '\n' | grep -vE -- "$seal_exclusion" | sort)" ||
  infra 'independent Git tracked-path derivation failed'
[[ -n $AURUM_LIVE_TRACKED ]] || fail unmapped-tracked-file 'independent Git tracked-path source is empty'
embedded_seal="${AURUM_TRACKED_SEAL%$'\n'}"
AURUM_TRACKED_SEAL="$AURUM_LIVE_TRACKED"
export AURUM_TRACKED_SEAL
seal_source='git-index'

# Coverage is bidirectional: every independently-derived tracked path must be present, and
# every inventory row must be independently tracked. This rejects unverified extra/forbidden
# rows as well as omissions.
verdict="$(
  awk -F '\t' '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    BEGIN {
      count = split(ENVIRON["AURUM_TRACKED_SEAL"], sealed, "\n")
      for (i = 1; i <= count; i++) {
        path = sealed[i]
        if (path == "") continue
        if (path ~ /^\// || path ~ /(^|\/)\.\.(\/|$)/ || path ~ /[[:cntrl:]]/)
          emit("seal-stale", "the embedded seal declares an unusable path: " path)
        want[path] = 1
        total++
      }
      if (total == 0) emit("seal-stale", "the embedded tracked-path seal is empty")
    }
    FNR > 1 && !($1 in want) { emit("stale-path", "inventory contains an untracked path: " $1) }
    FNR > 1 && ($1 in want) { delete want[$1]; matched++ }
    END {
      if (done) exit 0
      missing = 0
      for (path in want) {
        missing++
        if (first == "" || path < first) first = path
      }
      if (missing > 0)
        print "unmapped-tracked-file\tinventory omits " missing " of " total \
              " sealed tracked files, first: " first
    }
  ' "$inventory"
)" || infra 'seal coverage join failed to run'
emitted "$verdict"

sealed_count="$(
  awk 'BEGIN {
    count = split(ENVIRON["AURUM_TRACKED_SEAL"], sealed, "\n")
    for (i = 1; i <= count; i++) if (sealed[i] != "") total++
    print total + 0
  }'
)" || infra 'seal size unreadable'

verdict="$(
  awk -F '\t' '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function glob2ere(pattern,   out, i, c) {
      out = "^"
      for (i = 1; i <= length(pattern); i++) {
        c = substr(pattern, i, 1)
        if (c == "*") out = out ".*"
        else if (c == "?") out = out "."
        else if (index(".^$+(){}[]|\\", c) > 0) out = out "\\" c
        else out = out c
      }
      return out "$"
    }
    FNR == NR {
      if ($0 !~ /^- disposition: [^ ]+ -> [a-z]+$/) next
      split($0, field, " ")
      rules++
      pattern[rules] = glob2ere(field[3])
      verb[rules] = field[5]
      label[rules] = field[3]
      if (field[5] !~ /^(keep|migrate|replace|quarantine|characterize|delete|absent)$/)
        emit("inventory-schema-invalid", "unknown disposition verb: " field[5])
      next
    }
    FNR == 1 { next }
    {
      matches = 0
      for (i = 1; i <= rules; i++)
        if ($1 ~ pattern[i]) { matches++; hit[i]++; last = i }
      if (matches == 0) emit("unmapped-tracked-file", "no disposition rule covers " $1)
      if (matches > 1) emit("multi-match-disposition", $1 " matches " matches " disposition rules")
      if (verb[last] == "absent") emit("stale-path", "rule " label[last] " is declared absent but " $1 " is tracked")
    }
    END {
      if (done) exit 0
      if (rules == 0) { print "unmapped-tracked-file\tledger declares no machine-readable disposition rule"; exit 0 }
      for (i = 1; i <= rules; i++)
        if (!hit[i] && verb[i] != "absent") {
          print "stale-path\tdisposition rule " label[i] " covers no tracked file"
          exit 0
        }
    }
  ' "$ledger" "$inventory"
)" || infra 'disposition join failed to run'
emitted "$verdict"

verdict="$(
  awk -F '\t' '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function flush(   ) {
      if (id == "") return
      records++
      if (status == "implemented") {
        if (entrypoint == "" || test == "")
          emit("claim-without-proof", "claim " id " is implemented without entrypoint and test")
        if (!(entrypoint in tracked))
          emit("claim-without-proof", "claim " id " entrypoint is not a tracked file: " entrypoint)
        if (!(test in tracked))
          emit("claim-without-proof", "claim " id " test is not a tracked file: " test)
        if (test !~ /_test\.go$/ && test !~ /^tests\/acceptance\/[^\/]+\.sh$/)
          emit("claim-without-proof", "claim " id " test is not an executable test: " test)
        if (test ~ /^tests\/acceptance\/[^\/]+\.sh$/ && test_mode[test] != "100755")
          emit("claim-without-proof", "claim " id " acceptance test is not executable: " test)
      } else if (status == "disposition") {
        if (disposition !~ /^(keep|migrate|replace|quarantine|characterize|delete|absent)$/)
          emit("claims-manifest-invalid", "claim " id " has invalid disposition")
        if (reason !~ /[^ ]/ || test != "")
          emit("claims-manifest-invalid", "disposition " id " must have a reason and no test proof path")
        if (entrypoint != "" && !(entrypoint in tracked))
          emit("claim-without-proof", "disposition " id " entrypoint is not a tracked file: " entrypoint)
      } else if (status == "absent") {
        if (reason !~ /[^ ]/) emit("absent-without-reason", "claim " id " is absent without a reason")
        if (entrypoint != "" || test != "")
          emit("claims-manifest-invalid", "claim " id " is absent yet names an entrypoint or test")
      } else {
        emit("claims-manifest-invalid", "claim " id " has status " (status == "" ? "<none>" : status))
      }
      id = ""; status = ""; disposition = ""; entrypoint = ""; test = ""; reason = ""
    }
    FNR == NR { if (FNR > 1) { tracked[$1] = 1; test_mode[$1] = $3 } next }
    /^[ \t]*$/ || /^[ \t]*#/ { next }
    FNR == 1 && $0 == "claims:" { header = 1; next }
    !header { emit("claims-manifest-invalid", "manifest does not start with a claims: header") }
    /^  - claim: [^ ]+$/ {
      flush()
      id = $0
      sub(/^  - claim: /, "", id)
      if (id in claimed) emit("claims-manifest-invalid", "claim declared twice: " id)
      claimed[id] = 1
      next
    }
    /^    [a-z]+: / {
      if (id == "") emit("claims-manifest-invalid", "field outside a claim record at line " FNR)
      line = $0
      sub(/^    /, "", line)
      position = index(line, ": ")
      key = substr(line, 1, position - 1)
      value = substr(line, position + 2)
      if (key == "status") status = value
      else if (key == "disposition") disposition = value
      else if (key == "entrypoint") entrypoint = value
      else if (key == "test") test = value
      else if (key == "reason") reason = value
      else emit("claims-manifest-invalid", "unknown claim field: " key)
      next
    }
    { emit("claims-manifest-invalid", "unparsable line " FNR) }
    END {
      if (done) exit 0
      flush()
      if (!done && records == 0) print "claims-manifest-invalid\tno public claim is declared"
    }
  ' "$inventory" "$claims"
)" || infra 'claim join failed to run'
emitted "$verdict"

# The card's own `paths`. Every one is asserted present BEFORE any traversal, so a missing
# artifact is named instead of silently shrinking the anchor set that `find` produces.
declared_paths=(
  '.board/research/legacy-disposition.md'
  '.board/research/legacy-files.tsv'
  'tests/acceptance/AUR-001.sh'
  'docs/specs/AUR-001.md'
  'tests/specs/AUR-001'
  'tests/integration/AUR-001.go'
)
for declared in "${declared_paths[@]}"; do
  [[ -e $declared || -L $declared ]] ||
    fail declared-path-absent "card declares $declared but it is absent from the tree"
done
[[ -d 'tests/specs/AUR-001' && ! -L 'tests/specs/AUR-001' ]] ||
  fail declared-path-absent 'tests/specs/AUR-001 must be a directory holding the card specs'

# The 10 legacy surfaces this card exists to inventory. Each one must be represented in the
# seal, which is checked even when the surface itself was not handed to the container.
legacy_surfaces=(
  'go.mod' 'Dockerfile' 'action.yml' 'cmd/regenerate-docs'
  'internal/documentation' 'internal/pipeline' 'internal/llm' 'pkg/types'
  '.taskmaster' 'CLAUDE.md'
)
readonly legacy_surfaces_declared_count=10
# A loop over an emptied list would silently report success without checking
# anything, which is the exact false-green shape `.board/README.md` forbids.
# Any edit that drops or truncates the literal above is an infrastructure
# defect in the acceptance program itself, never a behavioral pass.
(( ${#legacy_surfaces[@]} == legacy_surfaces_declared_count )) \
  || inconclusive anti-shortcut-list-corrupted \
    "legacy_surfaces is ${#legacy_surfaces[@]} entries, expected $legacy_surfaces_declared_count; the anti-shortcut check must never run over an empty or truncated list"
AURUM_SURFACES="$(printf '%s\n' "${legacy_surfaces[@]}")"
export AURUM_SURFACES
verdict="$(
  awk '
    BEGIN {
      count = split(ENVIRON["AURUM_TRACKED_SEAL"], sealed, "\n")
      for (i = 1; i <= count; i++) if (sealed[i] != "") paths[sealed[i]] = 1
      count = split(ENVIRON["AURUM_SURFACES"], surface, "\n")
      for (i = 1; i <= count; i++) {
        want = surface[i]
        if (want == "") continue
        found = 0
        for (path in paths)
          if (path == want || index(path, want "/") == 1) { found = 1; break }
        if (!found) {
          print "stale-path\tlegacy surface " want " is represented by no sealed tracked path"
          exit 0
        }
      }
    }
  '
)" || infra 'legacy surface seal scan failed to run'
emitted "$verdict"

# A surface the runner never materialized cannot be verified here. That is a missing grant,
# not a verdict, so the run is reported inconclusive with the exact declaration it needs.
missing_surfaces=()
for surface in "${legacy_surfaces[@]}"; do
  [[ -e $surface || -L $surface ]] || missing_surfaces+=("$surface")
done
if ((${#missing_surfaces[@]} > 0)); then
  inconclusive undeclared-read-path \
    "legacy surfaces were not materialized; card AUR-001 must declare read_paths: [${missing_surfaces[*]}]"
fi

# Byte-level anchoring: every independently-derived path that exists in this tree has its type, mode and
# digest re-derived and compared to the inventory row. The inventory cannot carry its own
# digest, so it is the single exclusion.
root_real="$(pwd -P)" || infra 'repository root could not be canonicalized'
symlink_digest() {
  local path="$1"
  local target resolved
  target="$(readlink -- "$path")" || return 1
  resolved="$(readlink -f -- "$path")" || return 1
  [[ "$resolved" == "$root_real" || "$resolved" == "$root_real/"* ]] || return 1
  printf 'sha256:%s' "$(printf '%s' "$target" | sha256sum | awk '{print $1}')"
}
verified=0
unverified=()
while IFS= read -r path; do
  [[ -n $path ]] || continue
  [[ $path != "$inventory" ]] || continue
  [[ -e $path || -L $path ]] || { unverified+=("$path"); continue; }
  row="$(awk -F '\t' -v want="$path" 'FNR > 1 && $1 == want { print $2 "\t" $3 "\t" $4; exit }' "$inventory")" ||
    infra "inventory lookup failed for $path"
  [[ -n $row ]] || fail unmapped-tracked-file "tracked file absent from inventory: $path"
  row_type="${row%%$'\t'*}"
  row_rest="${row#*$'\t'}"
  row_mode="${row_rest%%$'\t'*}"
  row_digest="${row_rest#*$'\t'}"
  if [[ -L $path ]]; then
    [[ $row_type == symlink ]] || fail stale-path "$path is a symlink, inventory says $row_type"
    actual="$(symlink_digest "$path")" || fail unsafe-path "symlink target escapes or cannot resolve: $path"
  else
    [[ -f $path ]] || fail special-file "$path is neither a regular file nor a symlink"
    [[ $row_type == file ]] || fail stale-path "$path is a regular file, inventory says $row_type"
    if [[ -x $path ]]; then actual_mode='100755'; else actual_mode='100644'; fi
    [[ $row_mode == "$actual_mode" ]] ||
      fail stale-path "mode of $path is $actual_mode, inventory says $row_mode"
    actual="sha256:$(sha256sum -- "$path" | awk '{print $1}')"
  fi
  [[ $row_digest == "$actual" ]] || fail stale-path "digest of $path does not match the inventory"
  verified=$((verified + 1))
done <<<"$AURUM_TRACKED_SEAL"
# A tracked path the runner never materialized leaves its inventory digest unproven. That is
# a missing grant, never a pass: an unmaterialized row can carry any digest at all.
((${#unverified[@]} == 0)) ||
  inconclusive undeclared-read-path \
    "${#unverified[@]} of $sealed_count sealed paths were not materialized, so their inventory digests are unproven; first: ${unverified[0]}"

# The card's own artifacts are anchored too, including the ones minted by this card and
# therefore absent from a seal that was cut before they existed.
anchors=()
while IFS= read -r -d '' path; do
  anchors+=("$path")
done < <(
  find "${declared_paths[@]}" \( -type f -o -type l \) -print0 | sort -zu
)
((${#anchors[@]} >= 5)) ||
  fail declared-path-absent "only ${#anchors[@]} card-declared files were materialized, want at least 5"

for path in "${anchors[@]}"; do
  [[ $path != "$inventory" ]] || continue
  row="$(awk -F '\t' -v want="$path" 'FNR > 1 && $1 == want { print $2 "\t" $3 "\t" $4; exit }' "$inventory")" ||
    infra "inventory lookup failed for $path"
  [[ -n $row ]] || fail unmapped-tracked-file "tracked file absent from inventory: $path"
  row_type="${row%%$'\t'*}"
  row_rest="${row#*$'\t'}"
  row_mode="${row_rest%%$'\t'*}"
  row_digest="${row_rest#*$'\t'}"
  if [[ -L $path ]]; then
    [[ $row_type == symlink ]] || fail stale-path "$path is a symlink, inventory says $row_type"
    actual="$(symlink_digest "$path")" || fail unsafe-path "symlink target escapes or cannot resolve: $path"
  else
    [[ $row_type == file ]] || fail stale-path "$path is a regular file, inventory says $row_type"
    if [[ -x $path ]]; then actual_mode='100755'; else actual_mode='100644'; fi
    [[ $row_mode == "$actual_mode" ]] ||
      fail stale-path "mode of $path is $actual_mode, inventory says $row_mode"
    actual="sha256:$(sha256sum -- "$path" | awk '{print $1}')"
  fi
  [[ $row_digest == "$actual" ]] || fail stale-path "digest of $path does not match the inventory"
done

rows="$(awk -F '\t' 'FNR > 1 { count++ } END { print count + 0 }' "$inventory")" ||
  infra 'inventory row count failed'
claim_count="$(awk '/^  - claim: / { count++ } END { print count + 0 }' "$claims")" ||
  infra 'claim count failed'

printf '{"card":"%s","scenario":"%s","selector":"%s","tracked_files":%s,"claims":%s,"anchored":%s,"sealed_paths":%s,"seal_verified":%s,"seal_source":"%s","result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$rows" "$claim_count" "${#anchors[@]}" \
  "$sealed_count" "$verified" "$seal_source"
