#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

# AUR-203 / AC-001 -- ReleaseCandidateClosureV1.
#
# This is a gate: it exists to prove that every child of the release candidate
# actually passed. Two rules follow from that and drive the whole design.
#
#   1. The expected child set is embedded literally below. It is NOT read from
#      `.board/cards/**`: that path is in this card's own `forbidden_paths` and
#      in neither `paths` nor `read_paths`, so inside the pinned container it is
#      simply not materialised. Reading it turned every containerised run into a
#      red that was really a materialisation failure, and it pinned the card to
#      the `backlog/` directory, so the acceptance broke the moment the card
#      changed state. A literal list has neither problem and, unlike a file any
#      writer can edit, it cannot be shrunk to make the closure pass.
#
#   2. A verdict is never read from a file. The previous revision accepted the
#      string `pass` and a self-declared digest out of `suite.tsv`; writing that
#      file by hand was enough to close the release candidate with zero children
#      executed. That is the `"proved": true` anti-pattern `.board/README.md`
#      forbids. Here the gate re-executes each child acceptance program, reads
#      the RAW exit code, and recomputes every digest over real file CONTENT.
#      `suite.tsv` is demoted to a sealed cross-check: each of its fields must
#      equal what this run observed, and it can only ever make the gate stricter.
#
# Exit codes -- a claim of red must never be manufactured out of a broken bench:
#   0  = closure proven: full set present, every child observed exit 0, every
#        recomputed digest and the recomputed identity match the sealed suite.
#   1  = behavioural divergence, typed with this card's error codes.
#   3  = infrastructure error (missing tool, no writable temp, re-entry).
#   64 = unknown selector.
#   79 = inconclusive: a child neither passed nor produced a typed red, or a
#        bound was hit. Never laundered into a behavioural red.

readonly card='AUR-203'
readonly scenario='AC-001'

# Bounds from the card: at most 128 manifest rows / 4 MiB of manifest.
readonly max_suite_bytes=$((4 * 1024 * 1024))
readonly max_children=128
readonly max_child_bytes=$((1024 * 1024))
readonly child_timeout_s=20
readonly closure_deadline_s=300

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR203|IntegrationAUR203|E2EAUR203) ;;
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
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 79
}

(( ${BASH_VERSINFO[0]:-0} >= 4 )) || infra 'bash 4 or newer required'
for tool in awk sha256sum wc sort mktemp; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

# A closure gate runs other acceptance programs, some of which are closure gates
# themselves. The marker makes a dependency cycle a loud infrastructure error
# instead of an unbounded recursion. Children that do not know it ignore it.
case ":${AURUM_ACCEPT_STACK:-}:" in
  *":$card:"*) infra "re-entrant acceptance for $card" ;;
esac
export AURUM_ACCEPT_STACK="${AURUM_ACCEPT_STACK:+${AURUM_ACCEPT_STACK}:}$card"

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"

# The sealed child set of AUR-203, embedded so no writer can redefine it.
expected_csv='AUR-022,AUR-076,AUR-193,AUR-194,AUR-195,AUR-196,AUR-197,AUR-198,AUR-199,AUR-200'
expected_csv+=',AUR-201,AUR-202,AUR-221,AUR-226,AUR-281,AUR-282,AUR-283,AUR-284,AUR-285,AUR-286'
expected_csv+=',AUR-287,AUR-288,AUR-289,AUR-290,AUR-294,AUR-295,AUR-302,AUR-303,AUR-304,AUR-305'
expected_csv+=',AUR-306,AUR-307,AUR-308,AUR-309,AUR-310,AUR-311,AUR-312,AUR-313,AUR-314,AUR-315'
expected_csv+=',AUR-316,AUR-317,AUR-318,AUR-319,AUR-333,AUR-334,AUR-335,AUR-336,AUR-337,AUR-338'
expected_csv+=',AUR-339,AUR-340,AUR-341,AUR-342,AUR-343,AUR-344,AUR-345,AUR-346,AUR-347,AUR-348'
expected_csv+=',AUR-349,AUR-350,AUR-351,AUR-352,AUR-353,AUR-354,AUR-355,AUR-356,AUR-357,AUR-358'
expected_csv+=',AUR-365,AUR-366,AUR-367,AUR-368,AUR-369,AUR-370,AUR-371,AUR-372,AUR-373,AUR-374'
expected_csv+=',AUR-375,AUR-376,AUR-377,AUR-378,AUR-379,AUR-380,AUR-381,AUR-382,AUR-383,AUR-384'
expected_csv+=',AUR-385,AUR-386,AUR-387,AUR-388,AUR-389,AUR-390,AUR-391,AUR-412'
# Declared width of the closure. Quietly deleting an id from the literal above
# would silently shrink the set, so the derived count is checked against it.
readonly declared_count=98

mapfile -t expected_ids < <(printf '%s\n' "$expected_csv" | tr ',' '\n')
expected_count=${#expected_ids[@]}
(( expected_count == declared_count )) \
  || infra "embedded child set is $expected_count ids, declared $declared_count"
(( expected_count <= max_children )) \
  || infra "embedded child set exceeds $max_children manifests"
[[ "$(printf '%s\n' "${expected_ids[@]}" | sort -u | wc -l)" -eq $expected_count ]] \
  || infra 'embedded child set contains duplicates'

suite="$repo_root/tests/acceptance/gates/AUR-203/suite.tsv"
[[ -e "$suite" ]] || fail required_manifest_missing "$suite"
[[ ! -L "$suite" && -f "$suite" ]] || fail required_manifest_missing "$suite is not a regular file"
[[ -s "$suite" ]] || fail required_manifest_missing "$suite is empty"
suite_bytes="$(wc -c <"$suite")"
(( suite_bytes <= max_suite_bytes )) \
  || inconclusive bounds-exceeded "suite manifest is $suite_bytes bytes"

# Structural pass over the sealed manifest. It proves the manifest describes
# exactly the embedded set with well formed digests; it proves nothing about
# whether any child passed, which is decided further down by execution.
suite_rc=0
suite_report="$(awk -F '\t' -v expected="$expected_csv" -v expected_count="$expected_count" '
  BEGIN {
    n = split(expected, ids, ",")
    for (i = 1; i <= n; i++) want[ids[i]] = 1
    sha = "^sha256:[0-9a-f]{64}$"
  }
  NF != 5 {
    print "required_manifest_missing\tmalformed row " NR; bad = 1; exit 1
  }
  !($1 in want) {
    print "required_manifest_missing\tunexpected child " $1 " on row " NR; bad = 1; exit 1
  }
  seen[$1]++ {
    print "child_duplicate\t" $1 " repeated on row " NR; bad = 1; exit 1
  }
  $3 !~ sha {
    print "candidate_identity_mismatch\tmalformed identity for " $1; bad = 1; exit 1
  }
  $4 !~ sha || $5 !~ sha {
    print "child_digest_mismatch\tmalformed digest for " $1; bad = 1; exit 1
  }
  $2 != "pass" {
    print "child_not_pass\t" $1 " sealed as " $2; bad = 1; exit 1
  }
  identity == "" { identity = $3 }
  identity != $3 {
    print "candidate_identity_mismatch\t" $1 " carries a second identity"; bad = 1; exit 1
  }
  END {
    if (bad) exit 1
    if (NR != expected_count) {
      print "required_manifest_missing\texpected " expected_count " rows, found " NR
      exit 1
    }
    for (id in want)
      if (!seen[id]) { print "required_manifest_missing\t" id " absent from the suite"; exit 1 }
  }
' "$suite")" || suite_rc=$?
if (( suite_rc != 0 )); then
  [[ -n "$suite_report" ]] || infra 'suite parser produced no diagnosis'
  fail "${suite_report%%$'\t'*}" "${suite_report#*$'\t'}"
fi

declare -A sealed_identity=() sealed_spec=() sealed_artifact=()
while IFS=$'\t' read -r row_id row_verdict row_identity row_spec row_artifact || [[ -n ${row_id:-} ]]; do
  [[ -n "$row_id" ]] || continue
  sealed_identity["$row_id"]="$row_identity"
  sealed_spec["$row_id"]="$row_spec"
  sealed_artifact["$row_id"]="$row_artifact"
done <"$suite"

# Completeness before execution: a closure missing a child is red without any
# child being run, and no child is executed on a set that is already incomplete.
for id in "${expected_ids[@]}"; do
  child="$repo_root/tests/acceptance/$id.sh"
  [[ -e "$child" ]] || fail required_manifest_missing "child acceptance program $child is absent"
  [[ ! -L "$child" && -f "$child" ]] \
    || fail required_manifest_missing "child acceptance program $child is not a regular file"
  [[ -s "$child" ]] || fail required_manifest_missing "child acceptance program $child is empty"
done

work="$(mktemp -d 2>/dev/null)" || infra 'no writable temporary directory'
cleanup() { rm -rf -- "$work"; }
trap cleanup EXIT INT TERM

digest_of() {
  local sum
  sum="$(sha256sum -- "$1")" || infra "sha256sum failed on $1"
  printf 'sha256:%s' "${sum%% *}"
}

runner=(); command -v timeout >/dev/null 2>&1 && runner=(timeout -k 1 "$child_timeout_s")
identity_stream="$work/identity.stream"
: >"$identity_stream"

for id in "${expected_ids[@]}"; do
  (( SECONDS <= closure_deadline_s )) \
    || inconclusive bounds-exceeded "closure exceeded ${closure_deadline_s}s at $id"

  child="$repo_root/tests/acceptance/$id.sh"
  out="$work/$id.out"
  err="$work/$id.err"

  # The verdict of the child is its raw exit status, observed here. No pipe:
  # a pipeline would report the exit status of its last stage, not the child's.
  child_rc=0
  "${runner[@]}" bash "$child" AC-001 >"$out" 2>"$err" || child_rc=$?

  case "$child_rc" in
    0) ;;
    1) fail child_not_pass "$id exited 1" ;;
    *) inconclusive child-inconclusive "$id exited $child_rc, which is neither pass nor typed red" ;;
  esac

  out_bytes="$(wc -c <"$out")"
  (( out_bytes <= max_child_bytes )) \
    || inconclusive bounds-exceeded "$id emitted $out_bytes bytes"

  # Digests are computed here over real content: the child program as it exists
  # on disk, and the transcript this run just produced. A manifest cannot assert
  # either of them into existence, and nothing is ever hashed against itself.
  spec_digest="$(digest_of "$child")"
  artifact_digest="$(digest_of "$out")"

  [[ "${sealed_spec[$id]}" == "$spec_digest" ]] \
    || fail child_digest_mismatch "$id spec digest sealed ${sealed_spec[$id]}, recomputed $spec_digest"
  [[ "${sealed_artifact[$id]}" == "$artifact_digest" ]] \
    || fail child_digest_mismatch "$id artifact digest sealed ${sealed_artifact[$id]}, recomputed $artifact_digest"

  printf '%s\t%s\t%s\n' "$id" "$spec_digest" "$artifact_digest" >>"$identity_stream"
done

# CandidateIdentityV1 is derived from the ordered set of recomputed digests, so
# it too is content, not a claim. Every sealed row must carry exactly this value.
candidate_identity="$(digest_of "$identity_stream")"
for id in "${expected_ids[@]}"; do
  [[ "${sealed_identity[$id]}" == "$candidate_identity" ]] \
    || fail candidate_identity_mismatch \
        "$id sealed ${sealed_identity[$id]}, recomputed $candidate_identity"
done

printf '{"card":"%s","scenario":"AC-001","selector":"%s","children":%s,"identity":"%s","result":"pass"}\n' \
  "$card" "$selector" "$expected_count" "$candidate_identity"
