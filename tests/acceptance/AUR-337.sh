#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

# AUR-337 -- GuideClaimSuiteV1 gate over the three legacy guide-ownership leaves.
#
# THREAT MODEL THIS GATE IS WRITTEN AGAINST
#
# The previous revision read `tests/gates/legacy/AUR-337/suite.tsv` and accepted the
# literal string `pass` written in that file as the verdict of a child card, together
# with digest fields the file asserted about itself. Any writer of the repository could
# therefore mint three lines of TSV and turn the gate green without a single child card
# having run. That is the `"proved": true` pattern `.board/README.md` forbids: a claim
# stored in the repository is untrusted data, never proof.
#
# This gate now trusts nothing it cannot recompute:
#   * the expected child set is a literal embedded in THIS file, not read from anywhere;
#   * every child verdict comes from RE-EXECUTING the child's own acceptance program and
#     observing its raw exit status -- no file may assert a verdict on a child's behalf;
#   * every digest is recomputed by this gate over file CONTENT (sha256sum over the bytes),
#     never read from a field in which a file describes itself;
#   * the discovered child set under the read path must equal the embedded set exactly, so
#     a missing member, an extra member, or a diverging count is a failure;
#   * an externally pinned CandidateIdentityV1 (supplied by the coordinator through the
#     environment, i.e. outside the repository) is compared against the identity this gate
#     recomputes; the pin can only ever make the gate stricter, never green.
#
# EXIT CODES (a caller must be able to tell "not proven" from "wrong")
#   0   suite proven: every embedded child re-executed and returned pass
#   1   behavioural divergence: typed error code from the card on stderr
#   3   infrastructure: the harness itself is unusable (missing tool, unresolvable root)
#   64  unknown selector
#   79  inconclusive: a verdict could not be obtained (child timed out, child reported its
#       own infrastructure/inconclusive status, child output exceeded the bound). Never
#       converted into a behavioural red -- the board forbids that promotion.
#
# Typed behavioural codes (the five of AC-001):
#   guide_manifest_missing, child_duplicate, child_digest_mismatch,
#   candidate_identity_mismatch, child_not_pass

readonly card='AUR-337'
readonly scenario='AC-001'
readonly max_child_bytes=$((4 * 1024 * 1024))
readonly max_children=128
readonly child_deadline=8

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR337|IntegrationAUR337|E2EAUR337) ;;
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
inconclusive() {
  printf '%s/%s/inconclusive: %s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 79
}

for tool in awk grep sha256sum wc timeout mktemp; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

# ---------------------------------------------------------------------------------
# Embedded child set. This is the ONLY authority on which cards the suite governs.
# It travels inside this file precisely so that no repository artifact can add,
# remove, or re-order a member.
# ---------------------------------------------------------------------------------
readonly -a embedded_children=(AUR-383 AUR-384 AUR-385)
readonly embedded_count=3

(( ${#embedded_children[@]} == embedded_count )) || infra 'embedded child set disagrees with its own count'
(( embedded_count <= max_children )) || infra 'embedded child set exceeds the manifest bound'
{
  seen_embedded=''
  for child in "${embedded_children[@]}"; do
    [[ "$child" =~ ^AUR-[0-9]{3}$ ]] || infra "malformed embedded child id: $child"
    case " $seen_embedded " in
      *" $child "*) infra "duplicate embedded child id: $child" ;;
    esac
    seen_embedded="$seen_embedded $child"
  done
}

readonly claims_root='tests/characterization/legacy/guide-claims'

# ---------------------------------------------------------------------------------
# Set-exactness. The read path is INPUT TO BE VALIDATED, never an authority: it may
# only ever agree with the embedded list or make the gate fail.
# ---------------------------------------------------------------------------------
[[ -d "$claims_root" && ! -L "$claims_root" ]] \
  || fail guide_manifest_missing "claim root absent or not a directory: $claims_root"

shopt -s nullglob dotglob
discovered_paths=("$claims_root"/*)
shopt -u nullglob dotglob

(( ${#discovered_paths[@]} <= max_children )) \
  || fail child_duplicate "claim root holds ${#discovered_paths[@]} members, bound is $max_children"

discovered=''
discovered_count=0
for entry in "${discovered_paths[@]}"; do
  name="${entry##*/}"
  [[ -d "$entry" && ! -L "$entry" ]] \
    || fail child_duplicate "claim root member is not a plain child directory: $name"
  case " $discovered " in
    *" $name "*) fail child_duplicate "child directory repeated in claim root: $name" ;;
  esac
  case " ${embedded_children[*]} " in
    *" $name "*) ;;
    *) fail child_duplicate "claim root member is not an embedded child of $card: $name" ;;
  esac
  discovered="$discovered $name"
  discovered_count=$((discovered_count + 1))
done

for child in "${embedded_children[@]}"; do
  case " $discovered " in
    *" $child "*) ;;
    *) fail guide_manifest_missing "embedded child has no claim directory: $claims_root/$child" ;;
  esac
done

(( discovered_count == embedded_count )) \
  || fail child_duplicate "claim root holds $discovered_count children, embedded set declares $embedded_count"

# ---------------------------------------------------------------------------------
# Per-child proof. Nothing below reads a verdict; every verdict is produced by running
# the child's own acceptance program and reading its RAW exit status (no pipe, because a
# pipe swallows the producer's status).
# ---------------------------------------------------------------------------------
workdir="$(mktemp -d 2>/dev/null)" || infra 'temporary directory unavailable'
cleanup() { rm -rf -- "$workdir"; }
trap cleanup EXIT

digest_of() {
  local out
  out="$(sha256sum -- "$1" 2>/dev/null)" || return 1
  printf '%s' "${out%% *}"
}

identity_stream="$workdir/identity.tsv"
: >"$identity_stream"
child_json=''

for child in "${embedded_children[@]}"; do
  prog="tests/acceptance/$child.sh"
  matrix="$claims_root/$child/claims.tsv"

  [[ -f "$prog" && ! -L "$prog" && -r "$prog" && -s "$prog" ]] \
    || fail guide_manifest_missing "child acceptance program absent, empty, or not a regular file: $prog"
  [[ -f "$matrix" && ! -L "$matrix" && -r "$matrix" && -s "$matrix" ]] \
    || fail guide_manifest_missing "child claim matrix absent, empty, or not a regular file: $matrix"

  prog_digest="$(digest_of "$prog")" || infra "digest unavailable for $prog"
  matrix_digest="$(digest_of "$matrix")" || infra "digest unavailable for $matrix"

  out_a="$workdir/$child.a.out"
  err_a="$workdir/$child.a.err"
  rc=0
  timeout -k 1 "$child_deadline" bash -- "$prog" AC-001 >"$out_a" 2>"$err_a" || rc=$?

  case "$rc" in
    0) ;;
    124|125|137) inconclusive child_timeout "$child did not finish within ${child_deadline}s" ;;
    3)  inconclusive child_infrastructure "$child reported its own harness unusable" ;;
    79) inconclusive child_inconclusive "$child could not reach a verdict" ;;
    *)  fail child_not_pass "$child re-execution exited $rc" ;;
  esac

  bytes="$(wc -c <"$out_a" | tr -d ' ')"
  (( bytes <= max_child_bytes )) \
    || inconclusive child_output_unbounded "$child emitted $bytes bytes, bound is $max_child_bytes"

  # A bare `exit 0` is not a verdict either: the child must emit its own pass record, and
  # the verdict token must sit in the SAME record as the card id. Two unanchored greps
  # accept a child that declares itself FAILED beside an unrelated line carrying "pass".
  grep -Fq "\"card\":\"$child\"" "$out_a" \
    || fail child_not_pass "$child exited 0 without emitting its own result record"
  awk -v c="\"card\":\"$child\"" -v p='"result":"pass"' \
    'index($0, c) { seen = 1; if (index($0, p)) ok = 1; else bad = 1 }
     END { exit((seen && ok && !bad) ? 0 : 1) }' "$out_a" \
    || fail child_not_pass "$child exited 0 without declaring result pass in its own record"

  # Determinism: re-run and recompute the digest over the CONTENT of both runs.
  out_b="$workdir/$child.b.out"
  err_b="$workdir/$child.b.err"
  rc=0
  timeout -k 1 "$child_deadline" bash -- "$prog" AC-001 >"$out_b" 2>"$err_b" || rc=$?
  case "$rc" in
    0) ;;
    124|125|137) inconclusive child_timeout "$child did not finish its replay within ${child_deadline}s" ;;
    3)  inconclusive child_infrastructure "$child reported its own harness unusable on replay" ;;
    79) inconclusive child_inconclusive "$child could not reach a verdict on replay" ;;
    *)  fail child_not_pass "$child replay exited $rc" ;;
  esac

  stdout_digest="$(digest_of "$out_a")" || infra "digest unavailable for $child stdout"
  replay_digest="$(digest_of "$out_b")" || infra "digest unavailable for $child replay stdout"
  [[ "$stdout_digest" == "$replay_digest" ]] \
    || fail child_digest_mismatch "$child stdout digest is not reproducible across runs"

  printf '%s\t%s\t%s\t%s\n' "$child" "$prog_digest" "$matrix_digest" "$stdout_digest" >>"$identity_stream"

  child_json="$child_json{\"card\":\"$child\",\"program\":\"sha256:$prog_digest\",\"matrix\":\"sha256:$matrix_digest\",\"stdout\":\"sha256:$stdout_digest\"},"
done

rows="$(wc -l <"$identity_stream" | tr -d ' ')"
(( rows == embedded_count )) || infra "identity stream holds $rows rows, expected $embedded_count"

identity="$(digest_of "$identity_stream")" || infra 'identity digest unavailable'

# The pin arrives from the coordinator through the environment, i.e. from outside the
# repository, and is only ever a tightening: absent it the gate still proves nothing less,
# present and wrong it refuses. A repository file can never supply it.
if [[ -n "${AUR337_CANDIDATE_IDENTITY:-}" ]]; then
  [[ "${AUR337_CANDIDATE_IDENTITY}" == "sha256:$identity" ]] \
    || fail candidate_identity_mismatch 'pinned CandidateIdentityV1 does not match the recomputed suite identity'
fi

ids=''
for child in "${embedded_children[@]}"; do
  ids="$ids\"$child\","
done

printf '{"card":"%s","scenario":"%s","selector":"%s","schema":"GuideClaimSuiteV1","children":%s,"child_ids":[%s],"child_digests":[%s],"candidate_identity":"sha256:%s","result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$embedded_count" "${ids%,}" "${child_json%,}" "$identity"
