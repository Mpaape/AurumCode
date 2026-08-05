#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-362'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR362|IntegrationAUR362|E2EAUR362) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

# Exit codes follow tests/acceptance/EXIT_CODE_CONVENTION.md:
#   0  the promised behavior was observed
#   1  behavioral RED — a deliverable this card OWNS is missing or wrong
#   64 unknown selector
#   79 inconclusive / infrastructure — a tool, a bound, or this program's own
#      setup failed. Never valid red evidence, never a pass.
# The convention forbids a shared sourced library (the acceptance program is
# materialized alone into the sealed container), so the two verdict helpers
# are declared per-file, on purpose — same pattern as AUR-359.sh, the most
# heavily hardened sibling of this card's own bootstrap-lock family.

# fail() is reserved for `.board/bootstrap/locks/scanners.yml`, this card's
# own declared `paths:` deliverable. Its absence or corruption IS the
# behavior under test failing to exist: that is evidence.
fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  [[ -z "${2:-}" ]] || printf '%s/AC-001/detail field=%s\n' "$card" "$2" >&2
  exit 1
}

# infra() is reserved for everything this card does NOT own as a behavioral
# claim: a missing tool, a bound exceeded, this program's own fixture
# machinery failing to reproduce, or a read_path this card only
# characterizes (`.github/workflows`, `Dockerfile`, `.docker/docs.Dockerfile`)
# being unreadable. An environment gap must never be laundered into
# behavioral red.
infra() {
  printf '%s/AC-001/inconclusive/%s\n' "$card" "$1" >&2
  exit 79
}

for tool in grep sha256sum wc find; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *)   script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root_unresolved
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root_unresolved

readonly source_rel='.board/bootstrap/locks/scanners.yml'
readonly cases_rel='tests/bootstrap/locks/AUR-362/cases.tsv'
readonly fixtures_rel='tests/bootstrap/locks/AUR-362/fixtures'
readonly ground_truth_rel='tests/bootstrap/locks/AUR-362/ground-truth/cli-format-fields.tsv'

# 8 KiB: a flat 46-field manifest across four scanner blocks (observed at
# 2350 bytes today); not free-form input.
readonly max_source_bytes=8192
# 4 MiB / 256 rows: the bound the card declares for the sealed fixture table.
readonly max_cases_bytes=4194304
readonly max_cases_rows=256
# 4 KiB: twelve `key<TAB>value` lines plus a documentation header (observed
# well under 4 KiB today); this file is this card's own fixture machinery
# (the oracle's input, not the lock under test), so a bound violation here
# is this program's own setup fault, never a verdict about the real lock.
readonly max_ground_truth_bytes=4096

readonly pinned_image_pattern='^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$'
readonly plain_sha256_pattern='^sha256:[0-9a-f]{64}$'
readonly full_commit_pattern='^[0-9a-f]{40}$'

# ---------------------------------------------------------------------------
# Ground truth for every non-format field this card's own lock declares,
# resolved live by this card's builder against each tool's primary source
# (docs/specs/AUR-362.md, "Per-scanner pins and their primary source") and
# carried here independently of the lock file it checks — a value the lock
# asserts about itself is never trusted un-rechecked (same discipline as
# AUR-359.sh's `expected_image`).
#
# The twelve `*_finding_format_*` fields are deliberately NOT hand-typed
# into this literal (round-2 review found three of them wrong here while
# also wrong in the lock, because both were the same author's copy of the
# same belief, not two independent checks — see
# tests/bootstrap/locks/AUR-362/ground-truth/RAW_CAPTURE.txt). They are
# read below from tests/bootstrap/locks/AUR-362/ground-truth/cli-format-fields.tsv,
# a separately-versioned, separately-hashed fixture mechanically extracted
# from live pinned-digest CLI transcripts, so a lock value and its ground
# truth can no longer go wrong together by one author typing the same wrong
# belief twice.
# ---------------------------------------------------------------------------
declare -A expected=(
  [secrets_scanner_name]='gitleaks'
  [secrets_scanner_role]='secrets'
  [secrets_scanner_version]='v8.30.1'
  [secrets_scanner_image]='docker.io/zricethezav/gitleaks@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f'
  [secrets_rulebase_id]='gitleaks-default-config-v8.30.1'
  [secrets_rulebase_sha256]='sha256:e163e53b9e7e8a8511e77271e2b323ed057759542a6d988258afe3a1fa329caf'
  [secrets_offline_mode]='fully-offline'
  [secrets_egress]='denied'

  [vuln_scanner_name]='trivy'
  [vuln_scanner_role]='vulnerabilities'
  [vuln_scanner_version]='0.73.0'
  [vuln_scanner_image]='docker.io/aquasec/trivy@sha256:7cced7cae583819fc7806d4cbc0dbbc7cad18b99f7d3e235192e6da8c091045c'
  [vuln_rulebase_id]='trivy-db-schema-2'
  [vuln_rulebase_image]='ghcr.io/aquasecurity/trivy-db@sha256:cce7b2ad966d6fe3789d65879579b8b24fce9d31052ed3ff73b1c5224f049142'
  [vuln_offline_mode]='pre-cached-db-required'
  [vuln_offline_flag]='--offline-scan'
  [vuln_egress]='denied'

  [sast_scanner_name]='semgrep'
  [sast_scanner_role]='sast'
  [sast_scanner_version]='1.172.0'
  [sast_scanner_image]='docker.io/semgrep/semgrep@sha256:65dcd4408adda7c183a6b4550cb1e9b19f7f627a6fbb7e0559bd466bedc44d7b'
  [sast_rulebase_id]='semgrep-rules-pinned-commit'
  [sast_rulebase_ref]='311ca4e9ba59d700624539bf658e3d29b134ee77'
  [sast_offline_mode]='local-config-required'
  [sast_offline_flag]='--config'
  [sast_egress]='denied'

  [shell_scanner_name]='shellcheck'
  [shell_scanner_role]='shell'
  [shell_scanner_version]='v0.11.0'
  [shell_scanner_image]='docker.io/koalaman/shellcheck@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d'
  [shell_rulebase_id]='shellcheck-builtin-checks-v0.11.0'
  [shell_rulebase_digest]='sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d'
  [shell_offline_mode]='fully-offline'
  [shell_egress]='denied'
)

# Fields whose value is an OCI image reference that MUST be digest-pinned.
# A well-formed-but-wrong digest here is a supply-chain integrity tamper
# (source_integrity_error); a digest replaced by a floating tag/branch is
# MUT-003's own class (scanner-ref-mutable) — `vuln_rulebase_image` carries
# the identical threat shape as the four `*_scanner_image` fields it sits
# beside, so it is deliberately in this same set as this card's own extra
# mutation of MUT-003's class, not a new one.
pinned_fields='secrets_scanner_image vuln_scanner_image vuln_rulebase_image sast_scanner_image shell_scanner_image'

# Rule-base content digests that are not themselves image references: wrong
# shape is a structural defect (invalid_lock_or_manifest); right shape but
# wrong value is a content tamper (source_integrity_error).
plainsha_fields='secrets_rulebase_sha256 shell_rulebase_digest'
fullcommit_fields='sast_rulebase_ref'

# The one CLI-contract surface each tool exposes: format flag, its default,
# and its accepted value set. A mismatch here is MUT-002's own class
# (finding-schema-mismatch) regardless of which of the three sub-fields
# diverged. Unlike every other field in `expected`, these twelve are NOT
# hand-typed into this script (round-2 review found three of them wrong
# here while also wrong in the lock, because both were the same author's
# copy of the same belief, not two independent checks — see
# tests/bootstrap/locks/AUR-362/ground-truth/RAW_CAPTURE.txt). Their values
# are read into `expected` below, from ground_truth_rel, a separately
# versioned, separately hashed fixture mechanically extracted from live
# pinned-digest CLI transcripts — a lock value and its ground truth can no
# longer go wrong together by one author typing the same wrong belief twice.
fmt_fields='secrets_finding_format_flag secrets_finding_format_default secrets_finding_format_values vuln_finding_format_flag vuln_finding_format_default vuln_finding_format_values sast_finding_format_flag sast_finding_format_default sast_finding_format_values shell_finding_format_flag shell_finding_format_default shell_finding_format_values'

# Scanner families this card's read_paths must never invoke without a
# corresponding lock entry (MUT-001's own class: scanner-unlocked). None of
# these appear anywhere under .github/workflows, Dockerfile or
# .docker/docs.Dockerfile today — docs/specs/AUR-362.md records that as this
# card's own precondition. This list is deliberately disjoint from the four
# tool names this card DOES lock (gitleaks/trivy/semgrep/shellcheck), so a
# match can only ever mean a fifth, unlocked scanner was introduced.
foreign_scanners='bandit snyk codeql checkov trufflehog grype syft gosec'

is_pinned_image() { [[ "$1" =~ $pinned_image_pattern ]]; }
is_plain_sha256() { [[ "$1" =~ $plain_sha256_pattern ]]; }
is_full_commit()  { [[ "$1" =~ $full_commit_pattern ]]; }

field_in_set() {
  local needle="$1" haystack="$2" item
  for item in $haystack; do
    [[ "$item" == "$needle" ]] && return 0
  done
  return 1
}

# ---------------------------------------------------------------------------
# Load the CLI-contract ground truth (fmt_fields) into `expected` from
# ground_truth_rel, this card's own fixture machinery — never a verdict
# about the real lock, so every failure path here is infra, same discipline
# as cases_rel below. Runs once, before Phase 1, so both Phase 1 (the real
# tree) and Phase 2 (every sealed fixture) compare against the identical,
# independently-sourced values.
# ---------------------------------------------------------------------------
ground_truth_file="$repo_root/$ground_truth_rel"
[[ -e "$ground_truth_file" ]] || infra 'ground_truth_absent'
[[ -f "$ground_truth_file" && ! -L "$ground_truth_file" ]] || infra 'ground_truth_unreadable'
[[ -r "$ground_truth_file" ]] || infra 'ground_truth_unreadable'
gt_bytes="$(wc -c <"$ground_truth_file")" || infra 'ground_truth_unreadable'
[[ "$gt_bytes" =~ ^[0-9]+$ ]] || infra 'ground_truth_unreadable'
(( gt_bytes > 0 && gt_bytes <= max_ground_truth_bytes )) || infra 'ground_truth_out_of_bounds'

gt_seen=''
while IFS=$'\t' read -r gt_key gt_val || [[ -n "$gt_key" ]]; do
  [[ -n "$gt_key" ]] || continue
  [[ "${gt_key:0:1}" != '#' ]] || continue
  [[ -n "$gt_val" ]] || infra "ground_truth_malformed_row:$gt_key"
  field_in_set "$gt_key" "$fmt_fields" || infra "ground_truth_unknown_field:$gt_key"
  field_in_set "$gt_key" "$gt_seen" && infra "ground_truth_duplicate_field:$gt_key"
  gt_seen="$gt_seen $gt_key"
  expected["$gt_key"]="$gt_val"
done <"$ground_truth_file"

for field in $fmt_fields; do
  field_in_set "$field" "$gt_seen" || infra "ground_truth_field_missing:$field"
done

# ---------------------------------------------------------------------------
# verify_scanners_dir <root>
#   Prints one of: pass | scanner_lock_incomplete | source_integrity_error |
#   invalid_lock_or_manifest | input_limit_exceeded | scanner-ref-mutable |
#   finding-schema-mismatch | scanner-unlocked
#   Returns: 0 pass; 1 behavioral (the printed word is the typed code);
#            2 infra (the printed word is the infra reason).
# Used both for the real repository tree (Phase 1) and for every sealed
# fixture (Phase 2), so the real verdict and the self-test run through one
# recomputation path, never two divergent ones.
# ---------------------------------------------------------------------------
verify_scanners_dir() {
  local root="$1"
  local source_file="$root/$source_rel"
  local bytes line key val field

  [[ -e "$source_file" ]] || { printf 'scanner_lock_incomplete\n'; return 1; }
  [[ ! -L "$source_file" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ -f "$source_file" ]] || { printf 'scanner_lock_incomplete\n'; return 1; }
  [[ -r "$source_file" ]] || { printf 'source_unreadable\n'; return 2; }
  bytes="$(wc -c <"$source_file")" || { printf 'source_unreadable\n'; return 2; }
  [[ "$bytes" =~ ^[0-9]+$ ]] || { printf 'source_size_unreadable\n'; return 2; }
  (( bytes > 0 )) || { printf 'invalid_lock_or_manifest\n'; return 1; }
  (( bytes <= max_source_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }

  grep -Fqx 'schema: bootstrap-lock-v1' "$source_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Fqx "card: $card" "$source_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Eq 'sha256:[0-9a-f]{64}' "$source_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }

  # Parse every "key: value" line into a fresh map, scoped to this call so a
  # fixture never leaks a stale key into the next one.
  local -A actual=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" ]] || continue
    case "$line" in
      *': '*) key="${line%%: *}"; val="${line#*: }" ;;
      *)      continue ;;
    esac
    [[ -n "$key" ]] || continue
    actual["$key"]="$val"
  done <"$source_file"

  # Every field this card's own schema declares must be present. A field
  # entirely missing from the file is a structural defect, not a value
  # tamper: invalid_lock_or_manifest.
  for field in "${!expected[@]}"; do
    [[ -n "${actual[$field]+x}" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
  done

  # Pinned OCI image identities: shape first (floating reference is MUT-003's
  # class), then exact value (a well-formed but wrong digest is a content
  # tamper, not a floating one).
  for field in $pinned_fields; do
    val="${actual[$field]}"
    is_pinned_image "$val" || { printf 'scanner-ref-mutable\n'; return 1; }
    [[ "$val" == "${expected[$field]}" ]] || { printf 'source_integrity_error\n'; return 1; }
  done

  # Rule-base content digests that are not image references.
  for field in $plainsha_fields; do
    val="${actual[$field]}"
    is_plain_sha256 "$val" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ "$val" == "${expected[$field]}" ]] || { printf 'source_integrity_error\n'; return 1; }
  done
  for field in $fullcommit_fields; do
    val="${actual[$field]}"
    is_full_commit "$val" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ "$val" == "${expected[$field]}" ]] || { printf 'source_integrity_error\n'; return 1; }
  done

  # Finding-format CLI contract: any divergence is MUT-002's own class.
  for field in $fmt_fields; do
    [[ "${actual[$field]}" == "${expected[$field]}" ]] || { printf 'finding-schema-mismatch\n'; return 1; }
  done

  # Every remaining field (name, role, version, rulebase_id, offline_mode,
  # offline_flag, egress): a well-formed but wrong value is a generic
  # invalid field.
  for field in "${!expected[@]}"; do
    field_in_set "$field" "$pinned_fields" && continue
    field_in_set "$field" "$plainsha_fields" && continue
    field_in_set "$field" "$fullcommit_fields" && continue
    field_in_set "$field" "$fmt_fields" && continue
    [[ "${actual[$field]}" == "${expected[$field]}" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
  done

  # MUT-001's own class: a scanner this card never locked, referenced from a
  # read_path this card only characterizes. read_paths are materialized
  # alongside `paths` by .board/bin/oci-run and by validate.sh, but their
  # absence in a fixture root that deliberately omits them is never this
  # program's own infrastructure failure — it is the correct, checked
  # nominal state (no scanner is invoked anywhere in this repository today).
  local read_target scanner_kw candidate_file
  for read_target in "$root/.github/workflows" "$root/Dockerfile" "$root/.docker/docs.Dockerfile"; do
    [[ -e "$read_target" ]] || continue
    if [[ -d "$read_target" ]]; then
      while IFS= read -r candidate_file || [[ -n "$candidate_file" ]]; do
        [[ -n "$candidate_file" ]] || continue
        [[ -f "$candidate_file" && ! -L "$candidate_file" ]] || continue
        for scanner_kw in $foreign_scanners; do
          grep -Fiq -- "$scanner_kw" "$candidate_file" 2>/dev/null &&
            { printf 'scanner-unlocked\n'; return 1; }
        done
      done < <(find "$read_target" -type f 2>/dev/null)
    else
      [[ ! -L "$read_target" ]] || continue
      for scanner_kw in $foreign_scanners; do
        grep -Fiq -- "$scanner_kw" "$read_target" 2>/dev/null &&
          { printf 'scanner-unlocked\n'; return 1; }
      done
    fi
  done

  printf 'pass\n'
  return 0
}

# ---------------------------------------------------------------------------
# Phase 1 — this card's own deliverable, the real lock, checked FIRST and on
# its own: absence and every field-level defect above are behavioral RED
# because every artifact checked is this card's own declared path
# (`.board/bootstrap/locks/scanners.yml`), and that must be provable with
# nothing else in this file's fixture machinery built yet (RED-before-GREEN:
# tests/bootstrap/locks/AUR-362/cases.tsv and its fixtures do not gate this
# phase). The real repository tree is also this program's only source for
# the MUT-001 read_paths cross-check (fixtures reproduce it independently in
# Phase 2 below, each with its own materialized read_path copy).
# ---------------------------------------------------------------------------
set +e
real_word="$(verify_scanners_dir "$repo_root")"
real_rc=$?
set -e
case "$real_rc" in
  0) ;;
  1) fail "$real_word" "$source_rel" ;;
  *) infra "$real_word" ;;
esac

# ---------------------------------------------------------------------------
# Phase 2 — only once the real lock itself verifies clean does this program
# trust itself to judge fixtures: it re-proves its own sensitivity against
# tests/bootstrap/locks/AUR-362/cases.tsv, which seals, per row: a case
# name, the fixture subdirectory under
# tests/bootstrap/locks/AUR-362/fixtures/, the SHA-256 of the fixture's own
# scanners.yml (or `-` when the case is deliberate absence), and the
# expected disposition word (`pass` or one of the typed codes above). A
# fixture whose recomputed digest diverges from what the row seals is this
# program's OWN setup being unsound: infra, never a verdict about the real
# tree that Phase 1 already accepted.
# ---------------------------------------------------------------------------
cases_file="$repo_root/$cases_rel"
[[ -e "$cases_file" ]] || infra 'cases_tsv_absent'
[[ -f "$cases_file" && ! -L "$cases_file" ]] || infra 'cases_tsv_unreadable'
[[ -r "$cases_file" ]] || infra 'cases_tsv_unreadable'
cases_bytes="$(wc -c <"$cases_file")" || infra 'cases_tsv_unreadable'
[[ "$cases_bytes" =~ ^[0-9]+$ ]] || infra 'cases_tsv_unreadable'
(( cases_bytes > 0 && cases_bytes <= max_cases_bytes )) || infra 'cases_tsv_out_of_bounds'

case_count=0
while IFS=$'\t' read -r c_name c_fixture c_input_sha c_expect || [[ -n "$c_name" ]]; do
  [[ -n "$c_name" ]] || continue
  [[ "${c_name:0:1}" != '#' ]] || continue
  case_count=$(( case_count + 1 ))
  (( case_count <= max_cases_rows )) || infra 'cases_tsv_exceeds_row_bound'

  fixture_root="$repo_root/$fixtures_rel/$c_fixture"
  [[ -d "$fixture_root" ]] || infra "fixture_directory_absent:$c_fixture"

  fixture_source="$fixture_root/$source_rel"
  if [[ "$c_input_sha" == '-' ]]; then
    [[ ! -e "$fixture_source" ]] || infra "fixture_expected_absent:$c_fixture"
  else
    [[ -e "$fixture_source" && -r "$fixture_source" ]] ||
      infra "fixture_source_unreadable:$c_fixture"
    fixture_digest_line="$(sha256sum -- "$fixture_source")" || infra "fixture_unhashable:$c_fixture"
    fixture_digest="${fixture_digest_line%% *}"
    [[ "$fixture_digest" == "$c_input_sha" ]] || infra "fixture_digest_diverges:$c_fixture"
  fi

  set +e
  case_word="$(verify_scanners_dir "$fixture_root")"
  case_rc=$?
  set -e
  case "$case_rc" in
    0|1) ;;
    *) infra "fixture_case_infra:$c_fixture/$case_word" ;;
  esac
  [[ "$case_word" == "$c_expect" ]] ||
    fail 'fixture_sensitivity_error' "$c_name expected=$c_expect got=$case_word"
done < "$cases_file"
(( case_count > 0 )) || infra 'cases_tsv_empty'

printf '{"card":"%s","scenario":"AC-001","selector":"%s","fixture_cases":%d,"result":"pass"}\n' \
  "$card" "$selector" "$case_count"
