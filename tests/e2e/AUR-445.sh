#!/usr/bin/env bash
# AUR-445 E2E: the "grep de verificacao estatica sobre a documentacao de
# raiz" command the card's AC-001 names, run as a standalone, offline,
# black-box script -- independent of tests/unit/AUR-445.go and
# tests/integration/AUR-445.go's Go implementation, so a single algorithmic
# bug in one layer cannot fake a pass in every layer.
#
# WHAT IT CHECKS, over the six root documentation files this card owns
# (README.md, CHANGELOG.md, ACTION_USAGE.md, SETUP_GUIDE.md,
# RUN_DOCS_PIPELINE.md, docs/getting-started.md):
#   1. None of the five false-statement families the 2026-08-13 audit found
#      survive, after normalizing text (strip backticks, lowercase, collapse
#      all whitespace including newlines to single spaces) so a phrase
#      cannot dodge the check by being re-wrapped across a markdown line
#      break or re-spelled with different backtick placement.
#   2. The true replacement text is present where the audit demanded it.
#   3. A handful of those true statements are cross-checked directly against
#      the source they describe (LICENSE, cmd/aurumcode, cmd/regenerate-docs,
#      the Go extractor) -- doc-vs-code, not doc-vs-doc.
#
# SCOPE
#   This never executes `aurumcode` or `regenerate-docs`; it is static text
#   and source verification, matching the sandbox's no-network, no-binary-
#   execution constraint.
#
# ROOT OVERRIDE
#   AURUMCODE_ROOT, when set, is scanned instead of this script's own
#   repository checkout. tests/acceptance/AUR-445.sh's MUT-001 case uses this
#   to point the same, unmodified script at a scratch copy with the banned
#   LICENSE sentence reintroduced, proving the mutation is caught without
#   ever touching the tracked README.md.
#
# AUR445_MUTATION
#   Set to "MUT-001" ONLY by the acceptance's MUT-001 case, alongside
#   AURUMCODE_ROOT pointing at its mutated scratch copy. It exists because
#   the reintroduced LICENSE-absent sentence reads byte-identical to this
#   card's own pre-fix RED -- content alone cannot tell them apart. Without
#   this marker set, a banned LICENSE claim always reads as behavior-missing,
#   including on the untouched pre-fix tree; only the deliberate MUT-001 run
#   sees the MUT-001 label.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-445 scenario=AC-001
selector="${1:-E2EAUR445}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  E2EAUR445) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

if [[ -n "${AURUMCODE_ROOT:-}" ]]; then
  repo_root="$AURUMCODE_ROOT"
else
  script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
  repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
fi
[[ -d "$repo_root" ]] || infra "root_missing:$repo_root"

docs=(README.md CHANGELOG.md ACTION_USAGE.md SETUP_GUIDE.md RUN_DOCS_PIPELINE.md docs/getting-started.md)
for d in "${docs[@]}"; do
  [[ -s "$repo_root/$d" ]] || fail "behavior-missing:$d absent or empty"
done

# normalize FILE: strip backticks, lowercase, collapse all whitespace
# (including newlines) to single spaces. Mirrors tests/unit/AUR-445.go's
# aur445Normalize exactly, implemented independently in POSIX tools.
normalize() {
  tr -d '`' <"$1" | tr '[:upper:]' '[:lower:]' | tr '\n\t\r' '   ' | tr -s ' '
}

declare -A norm
for d in "${docs[@]}"; do
  norm["$d"]="$(normalize "$repo_root/$d")"
done

# banned "label|normalized-phrase|comma-separated-files(empty=all)"
banned=(
  'no-license-file|no license file is present|README.md,ACTION_USAGE.md,CHANGELOG.md'
  'no-code-review-output|no code-review or test-generation output|CHANGELOG.md'
  'every-knob-is-env-var|every knob is an environment variable|CHANGELOG.md,docs/getting-started.md'
  'gomarkdoc-mentioned|gomarkdoc|'
  'only-command-in-repository|the only command in this repository|RUN_DOCS_PIPELINE.md'
)

for entry in "${banned[@]}"; do
  IFS='|' read -r label phrase filelist <<<"$entry"
  targets=()
  if [[ -z "$filelist" ]]; then
    targets=("${docs[@]}")
  else
    IFS=',' read -ra targets <<<"$filelist"
  fi
  for d in "${targets[@]}"; do
    if [[ "${norm[$d]}" == *"$phrase"* ]]; then
      # Content alone cannot tell this card's own pre-fix RED apart from a
      # deliberately reintroduced MUT-001 mutation -- the banned sentence
      # reads identically either way. Only the run can: the acceptance's
      # MUT-001 case exports AUR445_MUTATION=MUT-001 when, and only when, it
      # is the one that injected the sentence. Every other run, pre-fix tree
      # included, must read as behavior-missing.
      if [[ "$label" == "no-license-file" && "${AUR445_MUTATION:-}" == "MUT-001" ]]; then
        fail "MUT-001: $d still contains the banned LICENSE-absent claim"
      fi
      fail "behavior-missing:$d still contains banned family $label"
    fi
  done
done

# required "label|normalized-anchor|comma-separated-files(any match passes)"
required=(
  'license-is-mit|[mit](license)|README.md,ACTION_USAGE.md,CHANGELOG.md'
  'review-command-documented|aurumcode review|CHANGELOG.md'
  'go-stdlib-mechanism-documented|go/parser|README.md,ACTION_USAGE.md,SETUP_GUIDE.md,RUN_DOCS_PIPELINE.md,docs/getting-started.md'
  'second-binary-documented|cmd/aurumcode|RUN_DOCS_PIPELINE.md'
  'security-pass-scope-honest|2 of the 8 rules|CHANGELOG.md'
)

for entry in "${required[@]}"; do
  IFS='|' read -r label anchor filelist <<<"$entry"
  IFS=',' read -ra targets <<<"$filelist"
  found=0
  for d in "${targets[@]}"; do
    if [[ "${norm[$d]}" == *"$anchor"* ]]; then
      found=1
      break
    fi
  done
  (( found == 1 )) || fail "behavior-missing:none of $filelist carries required anchor $label"
done

# doc-vs-code: a lightweight, independent re-check of the same facts
# tests/integration/AUR-445.go verifies structurally.
[[ -s "$repo_root/LICENSE" ]] || fail 'behavior-missing:LICENSE absent'
head -n1 "$repo_root/LICENSE" | grep -Fxq 'MIT License' \
  || fail 'behavior-missing:LICENSE first line is not "MIT License"'

for f in cmd/aurumcode/main.go cmd/regenerate-docs/main.go; do
  [[ -s "$repo_root/$f" ]] || fail "behavior-missing:$f absent"
  grep -Eq '^package main[[:space:]]*$' "$repo_root/$f" \
    || fail "behavior-missing:$f does not declare package main"
done

go_extractor_dir="$repo_root/internal/documentation/extractors/go"
[[ -d "$go_extractor_dir" ]] || fail 'behavior-missing:Go extractor directory absent'
while IFS= read -r -d '' gofile; do
  grep -Fq '"os/exec"' "$gofile" \
    && fail "behavior-missing:$(basename "$gofile") imports os/exec"
done < <(find "$go_extractor_dir" -maxdepth 1 -name '*.go' -print0)

printf '{"card":"%s","scenario":"%s","layer":"e2e","result":"pass","files_checked":%d,"banned_families":%d,"required_anchors":%d}\n' \
  "$card" "$scenario" "${#docs[@]}" "${#banned[@]}" "${#required[@]}"
