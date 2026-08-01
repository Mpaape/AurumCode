#!/usr/bin/env bash
#
# Acceptance program for card AUR-312, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `LegacyScriptSuiteV1` closes over exactly the twelve legacy script children
#   [AUR-365 .. AUR-376]. The suite verdict is produced HERE, by re-running each
#   child acceptance program and reading its raw exit status, and by recomputing
#   every digest over file CONTENT with sha256sum.
#
# WHAT THIS DELIBERATELY REFUSES TO TRUST
#
#   No verdict word, digest string or identity string stored anywhere in this
#   repository is an input to the decision. `.board/README.md` is explicit: a
#   repository value such as `authenticated: true` or `"proved": true` is
#   untrusted data, a claim is never a proof. The previous revision of this file
#   accepted the literal word `pass` typed by hand into
#   `tests/gates/legacy/AUR-312/suite.tsv`, so twelve hand-written rows closed
#   the gate with zero child executions. That is a forgery surface, not a gate.
#
#   The suite bundle is therefore OPTIONAL and never authoritative. When it is
#   present every one of its fields must equal the value this program has
#   already recomputed independently:
#     - the verdict column must equal the observed child exit status,
#     - the artifact column must equal sha256 of the child acceptance program,
#     - the spec column must equal sha256 of the child characterization
#       manifest,
#     - the identity column must equal the CandidateIdentityV1 digest computed
#       here over the twelve (id, verdict, spec, artifact) tuples.
#   A digest a file declares about itself can never satisfy the identity column,
#   because the identity is derived from child content this program hashes, and
#   the bundle is never part of its own preimage.
#
# SET DIVERGENCE is fatal in both directions: a missing child, an extra child,
# a duplicate row, or a characterization root holding anything other than the
# twelve embedded ids all fail closed. The expected set is a literal in this
# file, mirroring the Outcome of .board/cards/*/AUR-312.md; it is never read
# from a file that an author of the change under test can write.
#
# CHILD OUTPUT is untrusted and is never echoed, never parsed and never turned
# into a verdict. Only the child id and its integer exit status leave this
# program, so no child payload, secret or environment value can reach stdout,
# stderr or the suite manifest.
#
# EXIT CODES are disjoint on purpose:
#   0  = the promised property holds
#   1  = behavioral RED, reported with the card's typed error code
#   3  = harness or environment error, which is NEVER valid red evidence and is
#        never converted into a behavioral failure (a child that reports its own
#        environment error propagates as 3, not as a suite red)
#   64 = unknown scenario selector

set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-312'
readonly scenario='AC-001'
readonly rc_red=1
readonly rc_env=3

readonly expected_count=12
readonly max_program_bytes=1048576
readonly max_manifest_bytes=1048576
readonly max_bundle_bytes=4194304
readonly max_bundle_rows=128
readonly child_timeout_seconds=15

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR312|IntegrationAUR312|E2EAUR312) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

# Mirrors the Outcome set of .board/cards/*/AUR-312.md, in card order.
# `.board/cards` is a forbidden_path for this card, so the set is embedded here
# rather than parsed at run time.
readonly -a children=(
  'AUR-365' 'AUR-366' 'AUR-367' 'AUR-368' 'AUR-369' 'AUR-370'
  'AUR-371' 'AUR-372' 'AUR-373' 'AUR-374' 'AUR-375' 'AUR-376'
)

deliberate_exit=0
work_dir=''

cleanup() {
  if [[ -n "$work_dir" && -d "$work_dir" && ! -L "$work_dir" ]]; then
    rm -rf -- "$work_dir"
  fi
  return 0
}

env_error() {
  deliberate_exit=1
  printf '%s/%s/HARNESS-ERROR: %s\n' "$card" "$scenario" "$*" >&2
  exit "$rc_env"
}

red() {
  local code="$1"; shift
  deliberate_exit=1
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$code" "$*" >&2
  exit "$rc_red"
}

on_unexpected() {
  local status=$?
  (( deliberate_exit == 1 )) && return 0
  printf '%s/%s/HARNESS-ERROR: unexpected failure (status %d) near line %s\n' \
    "$card" "$scenario" "$status" "${BASH_LINENO[0]:-?}" >&2
  exit "$rc_env"
}

trap cleanup EXIT
trap on_unexpected ERR

# --------------------------------------------------------------------------
# Environment
# --------------------------------------------------------------------------

(( ${BASH_VERSINFO[0]:-0} >= 4 )) || env_error "bash >= 4 required, found ${BASH_VERSION:-unknown}"

for tool in sha256sum awk wc mktemp; do
  command -v -- "$tool" >/dev/null 2>&1 || env_error "required tool unavailable: $tool"
done
have_timeout=0
command -v -- timeout >/dev/null 2>&1 && have_timeout=1

# A gate must never be reachable from inside a child it launched.
if [[ -n "${AURUM_ACCEPTANCE_DEPTH:-}" ]]; then
  env_error "re-entrant acceptance execution (AURUM_ACCEPTANCE_DEPTH=${AURUM_ACCEPTANCE_DEPTH})"
fi

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)" \
  || env_error 'cannot resolve repository root'
[[ -d "$repo_root/tests/acceptance" ]] || env_error "missing acceptance directory: $repo_root/tests/acceptance"

work_dir="$(mktemp -d 2>/dev/null)" || env_error 'cannot create temporary working directory'

# Self-consistency of the embedded list: a typo in this file is a harness fault,
# never a red verdict about the tree under test.
(( ${#children[@]} == expected_count )) \
  || env_error "embedded child list holds ${#children[@]} ids, expected $expected_count"
declare -A embedded_seen=()
for child in "${children[@]}"; do
  [[ "$child" =~ ^AUR-[0-9]{3}$ ]] || env_error "malformed embedded child id: $child"
  [[ -z "${embedded_seen[$child]:-}" ]] || env_error "duplicate id in embedded child list: $child"
  embedded_seen["$child"]=1
done

# --------------------------------------------------------------------------
# Pure comparators, self-tested below before any pass is claimed
# --------------------------------------------------------------------------

digest_of() {
  local path="$1" line
  line="$(sha256sum -- "$path")" || return 1
  printf 'sha256:%s' "${line%% *}"
}

# check_row <verdict> <identity> <spec> <artifact> \
#           <want_verdict> <want_identity> <want_spec> <want_artifact>
# Echoes the typed error code for the first divergence, or nothing when the row
# reproduces every independently recomputed value.
check_row() {
  [[ "$1" == "$5" ]] || { printf 'child_not_pass'; return 1; }
  [[ "$4" == "$8" ]] || { printf 'child_digest_mismatch'; return 1; }
  [[ "$3" == "$7" ]] || { printf 'child_digest_mismatch'; return 1; }
  [[ "$2" == "$6" ]] || { printf 'candidate_identity_mismatch'; return 1; }
  return 0
}

selftest_expect() {
  local want="$1"; shift
  local got=''
  got="$("$@" || true)"
  [[ "$got" == "$want" ]] \
    || env_error "comparator self-test failed: expected '${want:-<ok>}', got '${got:-<ok>}'"
}

readonly st_ok='sha256:0000000000000000000000000000000000000000000000000000000000000000'
readonly st_bad='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
# A conforming row is accepted, and each field the skeptic forged is rejected
# with its own code. This is the hand-written `pass` + `sha256:aaa...` attack
# executed in-process, so a green result cannot come from a comparator that
# stopped comparing.
selftest_expect ''                            check_row pass "$st_ok" "$st_ok" "$st_ok" pass "$st_ok" "$st_ok" "$st_ok"
selftest_expect 'child_not_pass'              check_row pass "$st_ok" "$st_ok" "$st_ok" fail "$st_ok" "$st_ok" "$st_ok"
selftest_expect 'child_digest_mismatch'       check_row pass "$st_ok" "$st_ok" "$st_bad" pass "$st_ok" "$st_ok" "$st_ok"
selftest_expect 'child_digest_mismatch'       check_row pass "$st_ok" "$st_bad" "$st_ok" pass "$st_ok" "$st_ok" "$st_ok"
selftest_expect 'candidate_identity_mismatch' check_row pass "$st_bad" "$st_ok" "$st_ok" pass "$st_ok" "$st_ok" "$st_ok"

# --------------------------------------------------------------------------
# Independent verification of every child
# --------------------------------------------------------------------------

run_child() {
  local program="$1" out="$2" status=0
  set +e
  (
    ulimit -f 4096 2>/dev/null || true
    export AURUM_ACCEPTANCE_DEPTH=1
    if (( have_timeout == 1 )); then
      exec timeout -k 2 "$child_timeout_seconds" bash -- "$program" AC-001
    fi
    exec bash -- "$program" AC-001
  ) >"$out" 2>&1
  status=$?
  set -e
  return "$status"
}

declare -A artifact_digest=()
declare -A spec_digest=()
declare -A observed_verdict=()

characterization_root="$repo_root/tests/characterization/legacy/script-files"

for child in "${children[@]}"; do
  program="$repo_root/tests/acceptance/$child.sh"
  [[ -e "$program" ]] \
    || red script_manifest_missing "child acceptance program absent: tests/acceptance/$child.sh"
  [[ ! -L "$program" ]] \
    || red script_manifest_missing "child acceptance program is a symlink: tests/acceptance/$child.sh"
  [[ -f "$program" && -s "$program" ]] \
    || red script_manifest_missing "child acceptance program is not a readable regular file: tests/acceptance/$child.sh"
  bytes="$(wc -c <"$program" | tr -d ' ')"
  (( bytes <= max_program_bytes )) \
    || red script_manifest_missing "child acceptance program exceeds $max_program_bytes bytes: tests/acceptance/$child.sh"

  artifact_digest["$child"]="$(digest_of "$program")" \
    || env_error "cannot hash tests/acceptance/$child.sh"

  manifest="$characterization_root/$child/manifest.tsv"
  [[ -e "$manifest" && ! -L "$manifest" && -f "$manifest" && -s "$manifest" ]] \
    || red script_manifest_missing "characterization manifest absent: tests/characterization/legacy/script-files/$child/manifest.tsv"
  bytes="$(wc -c <"$manifest" | tr -d ' ')"
  (( bytes <= max_manifest_bytes )) \
    || red script_manifest_missing "characterization manifest exceeds $max_manifest_bytes bytes for $child"
  spec_digest["$child"]="$(digest_of "$manifest")" || env_error "cannot hash manifest for $child"

  # A child program is not evidence about itself: bind the manifest to real
  # content this program hashes, so a stub child plus a junk manifest cannot pass.
  src_rel="$(awk -F '\t' 'NR==1 { print $1 }' "$manifest")"
  [[ "$src_rel" == [A-Za-z0-9_]* && "$src_rel" != *..* ]] || red child_digest_mismatch "manifest for $child names an unusable source path"
  [[ -f "$repo_root/$src_rel" && ! -L "$repo_root/$src_rel" ]] || env_error "characterized source unavailable here: $src_rel"
  want_src="$(digest_of "$repo_root/$src_rel")" || env_error "cannot hash $src_rel"
  [[ "sha256:$(awk -F '\t' 'NR==1 { print $2 }' "$manifest")" == "$want_src" ]] || red child_digest_mismatch "manifest for $child declares a digest that is not sha256 of $src_rel"

  # MUT-002: characterization must observe the legacy source, never run it.
  if ! awk '
      {
        line = $0
        while (match(line, /commands_executed=/)) {
          seen++
          rest = substr(line, RSTART + RLENGTH)
          if (rest ~ /^0/ && rest !~ /^0[0-9]/) zero++
          line = rest
        }
      }
      END { exit((seen > 0 && seen == zero) ? 0 : 1) }
    ' "$manifest"; then
    red source_executed "manifest for $child does not record commands_executed=0 on every row"
  fi

  # THE verdict: the child's own raw exit status, observed here. Nothing else.
  child_status=0
  run_child "$program" "$work_dir/child.out" || child_status=$?
  case "$child_status" in
    0) observed_verdict["$child"]='pass' ;;
    3) env_error "child $child reported its own environment error (exit 3); inconclusive is never suite red" ;;
    124|137) env_error "child $child exceeded ${child_timeout_seconds}s (exit $child_status)" ;;
    *) red child_not_pass "child $child re-execution exited $child_status (expected 0)" ;;
  esac
  : >"$work_dir/child.out"
done

# Set divergence on the tree itself: the characterization root may hold the
# twelve embedded ids and nothing else.
if [[ -d "$characterization_root" ]]; then
  actual_entries=0
  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    actual_entries=$(( actual_entries + 1 ))
    [[ "$entry" =~ ^AUR-[0-9]{3}$ ]] \
      || red child_duplicate "characterization root holds unexpected entry '$entry' outside the embedded set"
    [[ -n "${embedded_seen[$entry]:-}" ]] \
      || red child_duplicate "characterization root holds unexpected child '$entry' outside the embedded set"
  done < <(ls -1 -- "$characterization_root" 2>/dev/null || true)
  (( actual_entries == expected_count )) \
    || red script_manifest_missing "characterization root holds $actual_entries children, embedded list requires $expected_count"
fi

# --------------------------------------------------------------------------
# CandidateIdentityV1, recomputed over child content only
# --------------------------------------------------------------------------

preimage="$work_dir/identity.preimage"
: >"$preimage"
for child in "${children[@]}"; do
  printf '%s\t%s\t%s\t%s\n' \
    "$child" "${observed_verdict[$child]}" "${spec_digest[$child]}" "${artifact_digest[$child]}" >>"$preimage"
done
suite_identity="$(digest_of "$preimage")" || env_error 'cannot hash suite identity preimage'

# --------------------------------------------------------------------------
# Optional suite bundle: audited against the recomputation, never trusted
# --------------------------------------------------------------------------

bundle="$repo_root/tests/gates/legacy/AUR-312/suite.tsv"
bundle_rows=0
if [[ -e "$bundle" ]]; then
  [[ ! -L "$bundle" ]] || red suite_manifest_invalid "suite bundle is a symlink: tests/gates/legacy/AUR-312/suite.tsv"
  [[ -f "$bundle" && -s "$bundle" ]] || red suite_manifest_invalid "suite bundle is not a readable regular file"
  bytes="$(wc -c <"$bundle" | tr -d ' ')"
  (( bytes <= max_bundle_bytes )) || red suite_manifest_invalid "suite bundle exceeds $max_bundle_bytes bytes"

  declare -A bundle_seen=()
  line_no=0
  while IFS=$'\t' read -r b_id b_verdict b_identity b_spec b_artifact b_extra || [[ -n "${b_id:-}" ]]; do
    line_no=$(( line_no + 1 ))
    (( line_no <= max_bundle_rows )) || red suite_manifest_invalid "suite bundle exceeds $max_bundle_rows rows"
    [[ -n "$b_id" ]] || red suite_manifest_invalid "suite bundle line $line_no is empty"
    [[ -z "${b_extra:-}" && -n "$b_artifact" ]] \
      || red suite_manifest_invalid "suite bundle line $line_no does not hold exactly 5 tab-separated fields"
    [[ "$b_id" =~ ^AUR-[0-9]{3}$ ]] \
      || red suite_manifest_invalid "suite bundle line $line_no holds a malformed card id"
    [[ -n "${embedded_seen[$b_id]:-}" ]] \
      || red suite_manifest_invalid "suite bundle line $line_no names '$b_id', outside the embedded set"
    [[ -z "${bundle_seen[$b_id]:-}" ]] \
      || red child_duplicate "suite bundle repeats $b_id at line $line_no"
    bundle_seen["$b_id"]=1
    bundle_rows=$(( bundle_rows + 1 ))

    code=''
    code="$(check_row "$b_verdict" "$b_identity" "$b_spec" "$b_artifact" \
      "${observed_verdict[$b_id]}" "$suite_identity" "${spec_digest[$b_id]}" "${artifact_digest[$b_id]}" || true)"
    if [[ -n "$code" ]]; then
      red "$code" "suite bundle line $line_no for $b_id contradicts the value recomputed here from file content"
    fi
  done <"$bundle"

  (( bundle_rows == expected_count )) \
    || red script_manifest_missing "suite bundle holds $bundle_rows rows, embedded list requires $expected_count"
  for child in "${children[@]}"; do
    [[ -n "${bundle_seen[$child]:-}" ]] || red script_manifest_missing "suite bundle omits $child"
  done
fi

printf '{"card":"%s","scenario":"AC-001","selector":"%s","children":%d,"bundle_rows":%d,"suite_identity":"%s","verdict_source":"child-reexecution","result":"pass"}\n' \
  "$card" "$selector" "$expected_count" "$bundle_rows" "$suite_identity"
