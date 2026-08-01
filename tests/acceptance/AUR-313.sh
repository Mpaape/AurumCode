#!/usr/bin/env bash
#
# Acceptance program for card AUR-313, scenario AC-001 -- "Verificar claims dos
# seis documentos publicos" (`PublicClaimSuiteV1`).
#
# WHAT CHANGED AND WHY
#
# The previous revision of this file read `tests/gates/legacy/AUR-313/suite.tsv`
# and accepted the literal word `pass` written in column 2 of that file as the
# verdict of each child card. That is the `"proved": true` pattern the board
# forbids: a repository value is untrusted data, and a claim is never a proof.
# Anyone able to write six tab-separated lines could turn this gate green
# without a single child card ever running.
#
# This program consults NO verdict file. There is no path under
# `tests/gates/legacy/AUR-313/` on its read surface at all: nothing written
# there can make this gate pass, fail, or change its output in any way. The
# suite is closed only from facts the coordinator can recompute from the tree:
#
#   1. The expected child set is a LITERAL embedded in this file, mirroring the
#      card's Outcome. It cannot be widened or narrowed by any file.
#   2. Each child's acceptance program is RE-EXECUTED and its raw exit code is
#      observed. `pass` is an exit status this program produced, never a string
#      it read.
#   3. Every digest is recomputed by this program over FILE CONTENT
#      (`sha256sum` of the child programs and of the six public documents),
#      before the first child runs and again after the last one. No file is ever
#      allowed to declare its own digest, and a document rewritten between two
#      child runs breaks the seal even though every program is untouched.
#   4. `AUR-381` documents the pipeline whose credential slot `AUR-333` redacts,
#      so `AUR-333` is re-executed as well: an `AUR-381` pass that is not backed
#      by a passing redaction chain is rejected (MUT-001).
#
# WHAT IT DELIBERATELY DOES NOT DO: it does not re-derive the children's own
# claims, does not merge or inspect child payloads, and never re-interprets a
# child failure as partial success. Child stdout/stderr is captured to bounded
# temporary files and discarded; only the child ID, the recomputed digest, the
# observed exit code and a strictly pattern-matched typed error token ever reach
# this program's own output. `AURUM_SECRET_CANARY` and any other environment
# value therefore cannot transit this gate.
#
# EXIT CODES, deliberately disjoint:
#   0   the six children were re-executed and every one of them exited 0, under
#       one recomputed CandidateIdentityV1;
#   1   behavioral RED, message `AUR-313/AC-001/<error_code>: <detail>`, with
#       <error_code> drawn from the card's contract:
#         document_manifest_missing, child_duplicate, child_digest_mismatch,
#         candidate_identity_mismatch, child_not_pass,
#         redaction_dependency_missing;
#   64  unknown selector;
#   69  INCONCLUSIVE (environment/harness), message
#       `AUR-313/AC-001/infrastructure/<reason>`. A missing utility, an
#       unreadable file, a child that reported its own harness error, a timeout
#       or an unclassifiable child exit status is never valid red evidence and
#       is never converted into a behavioral verdict.
#
# BOUNDS: at most 6 children plus 1 dependency, 4 MiB of captured child output
# per stream, a 20 s wall-clock budget for the whole suite, no network, no
# installation, no credential resolution, and no write outside a private
# `mktemp -d`.

set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-313'
readonly scenario='AC-001'
readonly rc_red=1
readonly rc_selector=64
readonly rc_infra=69
readonly max_output_bytes=$((4 * 1024 * 1024))
readonly suite_budget_seconds=20
readonly child_budget_seconds=15

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR313|IntegrationAUR313|E2EAUR313) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit "$rc_selector" ;;
esac

fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit "$rc_red"
}
infra() {
  printf '%s/%s/infrastructure/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit "$rc_infra"
}

# ---------------------------------------------------------------------------
# Embedded contract. This is the ONLY source of the expected set.
# Mirrors the Outcome of .board/cards/*/AUR-313.md; `.board/cards` is in this
# card's forbidden_paths, so the card is not read at run time.
# ---------------------------------------------------------------------------
readonly -a expected_children=(
  'AUR-377'
  'AUR-378'
  'AUR-379'
  'AUR-380'
  'AUR-381'
  'AUR-382'
)
readonly redaction_dependent='AUR-381'
readonly redaction_dependency='AUR-333'
readonly expected_count="${#expected_children[@]}"

# The six public documents the suite is about. They are sealed into
# CandidateIdentityV1 so that a document rewritten BETWEEN child runs -- which
# leaves every child program byte-identical -- still breaks the suite.
readonly -a public_documents=(
  'README.md'
  'ACTION_USAGE.md'
  'LITELLM_QUICKSTART.md'
  'PAGES_SETUP.md'
  'RUN_DOCS_PIPELINE.md'
  'SETUP_GUIDE.md'
)

# ---------------------------------------------------------------------------
# Preflight. Every condition here is an environment fact, never a card fact.
# ---------------------------------------------------------------------------

(( ${BASH_VERSINFO[0]:-0} >= 4 )) ||
  infra 'bash-version' 'bash 4 or newer is required for associative arrays'

if [[ -n "${AURUM_ACCEPTANCE_AUR313_ACTIVE:-}" ]]; then
  infra 'recursive-invocation' \
    'AUR-313 was re-entered from inside a child acceptance program; the suite cannot judge itself'
fi
export AURUM_ACCEPTANCE_AUR313_ACTIVE=1

for tool in sha256sum timeout mktemp wc head rm; do
  command -v "$tool" >/dev/null 2>&1 ||
    infra 'missing-tool' "required host utility is absent: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" ||
  infra 'repo-root' 'repository root could not be resolved from $0'
cd -- "$repo_root" || infra 'repo-root' "repository root is unreachable: $repo_root"

work=''
cleanup() { [[ -z "$work" ]] || rm -rf -- "$work"; }
trap cleanup EXIT INT TERM HUP
work="$(mktemp -d 2>/dev/null)" ||
  infra 'workdir' 'no writable temporary directory for bounded child output capture'

# ---------------------------------------------------------------------------
# 0. Self-consistency of the embedded set. A duplicated literal would let one
#    passing child stand in for two.
# ---------------------------------------------------------------------------
declare -A seen_id=()
for child in "${expected_children[@]}"; do
  [[ -z "${seen_id[$child]:-}" ]] ||
    fail 'child_duplicate' "embedded child set names $child more than once"
  seen_id["$child"]=1
done
(( ${#seen_id[@]} == expected_count )) ||
  fail 'child_duplicate' "embedded child set collapses to ${#seen_id[@]} distinct IDs, expected $expected_count"
(( ${#public_documents[@]} == expected_count )) ||
  fail 'document_manifest_missing' \
    "embedded document set has ${#public_documents[@]} entries, expected $expected_count"

program_of() { printf 'tests/acceptance/%s.sh' "$1"; }

# ---------------------------------------------------------------------------
# 1. Presence. A child whose acceptance program is absent has produced nothing
#    to aggregate; the suite cannot be closed over an incomplete set.
# ---------------------------------------------------------------------------
for child in "${expected_children[@]}"; do
  program="$(program_of "$child")"
  [[ -e "$program" || -L "$program" ]] ||
    fail 'document_manifest_missing' "child acceptance program absent: $program (child $child of $expected_count)"
  [[ -f "$program" ]] ||
    fail 'document_manifest_missing' "child acceptance program is not a regular file: $program"
  [[ -r "$program" ]] ||
    infra 'unreadable-child' "child acceptance program exists but is unreadable: $program"
done

dependency_program="$(program_of "$redaction_dependency")"
[[ -e "$dependency_program" || -L "$dependency_program" ]] ||
  fail 'redaction_dependency_missing' \
    "$redaction_dependent claims a redacted pipeline but the $redaction_dependency acceptance program is absent: $dependency_program"
[[ -f "$dependency_program" ]] ||
  fail 'redaction_dependency_missing' \
    "$redaction_dependency acceptance program is not a regular file: $dependency_program"
[[ -r "$dependency_program" ]] ||
  infra 'unreadable-child' "dependency acceptance program exists but is unreadable: $dependency_program"

# ---------------------------------------------------------------------------
# 2. Aliasing. Symlinking five children at the one that passes would make a
#    single green run look like six.
# ---------------------------------------------------------------------------
canonical_of() {
  local path="$1" dir base
  dir="$(dirname -- "$path")"
  base="$(basename -- "$path")"
  (CDPATH= cd -- "$dir" 2>/dev/null && printf '%s/%s' "$(pwd -P)" "$base") ||
    infra 'realpath' "path could not be canonicalized: $path"
}

declare -A owner_of_target=()
for child in "${expected_children[@]}" "$redaction_dependency"; do
  program="$(program_of "$child")"
  [[ ! -L "$program" ]] ||
    fail 'child_duplicate' "child acceptance program is a symlink and may alias another child: $program"
  target="$(canonical_of "$program")"
  previous="${owner_of_target[$target]:-}"
  [[ -z "$previous" ]] ||
    fail 'child_duplicate' "$child and $previous resolve to the same program: $target"
  owner_of_target["$target"]="$child"
done

# Documents. A document that is a symlink, or two documents that resolve to the
# same bytes-on-disk, would let one verified document stand in for another.
declare -A owner_of_document=()
for document in "${public_documents[@]}"; do
  [[ -e "$document" || -L "$document" ]] ||
    fail 'document_manifest_missing' "public document absent: $document"
  [[ ! -L "$document" ]] ||
    fail 'child_duplicate' "public document is a symlink and may alias another document: $document"
  [[ -f "$document" ]] ||
    fail 'document_manifest_missing' "public document is not a regular file: $document"
  [[ -r "$document" ]] ||
    infra 'unreadable-document' "public document exists but is unreadable: $document"
  target="$(canonical_of "$document")"
  previous="${owner_of_document[$target]:-}"
  [[ -z "$previous" ]] ||
    fail 'child_duplicate' "$document and $previous resolve to the same file: $target"
  owner_of_document["$target"]="$document"
done

# ---------------------------------------------------------------------------
# 3. Content digests, recomputed here over the bytes on disk. Nothing on disk
#    is permitted to state its own digest.
# ---------------------------------------------------------------------------
declare -A digest_before=()
digest_of() {
  local path="$1" out
  out="$(sha256sum -- "$path" 2>/dev/null)" ||
    infra 'digest' "sha256sum failed on $path"
  printf 'sha256:%s' "${out%% *}"
}
for child in "${expected_children[@]}" "$redaction_dependency"; do
  digest_before["$child"]="$(digest_of "$(program_of "$child")")"
done
declare -A document_digest_before=()
for document in "${public_documents[@]}"; do
  document_digest_before["$document"]="$(digest_of "$document")"
done

# CandidateIdentityV1 for this suite: one digest over the ordered set of child
# programs AND the six public documents, every component re-hashed from disk at
# the moment of the call. It is sealed before the first child runs and resealed
# after the last one, so anything rewritten between two child runs -- including
# a document that no child program touches -- breaks the seal.
identity_of() {
  local child document out
  out="$(
    {
      for child in "${expected_children[@]}" "$redaction_dependency"; do
        printf 'child\t%s\t%s\n' "$child" "$(digest_of "$(program_of "$child")")"
      done
      for document in "${public_documents[@]}"; do
        printf 'document\t%s\t%s\n' "$document" "$(digest_of "$document")"
      done
    } | sha256sum 2>/dev/null
  )" || infra 'digest' 'sha256sum failed while sealing CandidateIdentityV1'
  printf 'sha256:%s' "${out%% *}"
}
identity_before="$(identity_of)"

# ---------------------------------------------------------------------------
# 4. Re-execution. `pass` is an exit status this program observed.
# ---------------------------------------------------------------------------
sanitized_reason() {
  # Only a typed `AUR-nnn/<scenario>/<code>` token from the child's first
  # stderr line is ever quoted. Anything else is dropped, so arbitrary child
  # bytes (and any environment value they might carry) cannot reach stdout.
  local err_file="$1" line=''
  line="$(head -n 1 -- "$err_file" 2>/dev/null || true)"
  if [[ "$line" =~ (AUR-[0-9]{3}/[A-Za-z0-9_.-]{1,32}/[A-Za-z0-9_-]{1,64}) ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  else
    printf 'no typed error token on child stderr'
  fi
}

# Sets the globals `child_status` and `child_reason`. It must NOT be called
# inside a command substitution: `infra` has to be able to terminate this
# program, and both globals have to survive the call.
child_status=''
child_reason=''
run_child() {
  local child="$1"
  local program out err remaining status=0 out_bytes err_bytes
  program="$(program_of "$child")"
  out="$work/$child.out"
  err="$work/$child.err"
  remaining=$(( suite_budget_seconds - SECONDS ))
  (( remaining > 1 )) ||
    infra 'suite-deadline' "the ${suite_budget_seconds}s suite budget was exhausted before $child could run"
  if (( remaining > child_budget_seconds )); then
    remaining="$child_budget_seconds"
  fi
  timeout -k 2 "$remaining" bash -- "$program" AC-001 >"$out" 2>"$err" || status=$?
  out_bytes="$(wc -c <"$out" | tr -d ' ')"
  err_bytes="$(wc -c <"$err" | tr -d ' ')"
  (( out_bytes <= max_output_bytes && err_bytes <= max_output_bytes )) ||
    infra 'child-output-unbounded' "$child exceeded the ${max_output_bytes}-byte capture bound; output withheld"
  case "$status" in
    124|137) infra 'child-timeout' "$child did not terminate within ${remaining}s" ;;
    125|126|127) infra 'child-not-executable' "$child could not be launched (launcher status $status)" ;;
    64) infra 'child-selector' "$child rejected the AC-001 selector; the harness contract does not match" ;;
    3|69|79) infra 'child-inconclusive' "$child reported its own harness/environment error (status $status: $(sanitized_reason "$err"))" ;;
  esac
  child_reason="$(sanitized_reason "$err")"
  child_status="$status"
}

declare -A verdict_of=()
passed=0
for child in "${expected_children[@]}"; do
  run_child "$child"
  case "$child_status" in
    0) verdict_of["$child"]='pass'; passed=$(( passed + 1 )) ;;
    1) fail 'child_not_pass' "$child re-executed and exited 1 ($child_reason); a child failure is never a partial success" ;;
    *) infra 'child-unclassified-exit' "$child exited with unclassified status $child_status; that is not evidence either way" ;;
  esac
done

(( passed == expected_count )) ||
  fail 'document_manifest_missing' "only $passed of $expected_count children were observed passing"

# ---------------------------------------------------------------------------
# 5. MUT-001. AUR-381 documents RUN_DOCS_PIPELINE.md, whose credential slot is
#    redacted by AUR-333. An AUR-381 pass with no passing redaction chain is
#    exactly the mutation this card must refuse.
# ---------------------------------------------------------------------------
run_child "$redaction_dependency"
case "$child_status" in
  0) ;;
  1) fail 'redaction_dependency_missing' \
       "$redaction_dependent is pass but its redaction dependency $redaction_dependency re-executed and exited 1 ($child_reason)" ;;
  *) infra 'child-unclassified-exit' "$redaction_dependency exited with unclassified status $child_status" ;;
esac

# ---------------------------------------------------------------------------
# 6. Post-run integrity. A child that rewrote itself or another child during
#    the run must not be able to launder that into the suite.
# ---------------------------------------------------------------------------
for child in "${expected_children[@]}" "$redaction_dependency"; do
  after="$(digest_of "$(program_of "$child")")"
  [[ "$after" == "${digest_before[$child]}" ]] ||
    fail 'child_digest_mismatch' "$child program content changed during the run: ${digest_before[$child]} -> $after"
done
identity_after="$(identity_of)"
if [[ "$identity_after" != "$identity_before" ]]; then
  # Programs are byte-identical (checked just above), so name the document that
  # moved: a claim was verified against bytes that no longer exist.
  for document in "${public_documents[@]}"; do
    after="$(digest_of "$document")"
    [[ "$after" == "${document_digest_before[$document]}" ]] ||
      fail 'candidate_identity_mismatch' \
        "$document changed during the run: ${document_digest_before[$document]} -> $after; the child verdicts were observed against a different tree"
  done
  fail 'candidate_identity_mismatch' "CandidateIdentityV1 changed during the run: $identity_before -> $identity_after"
fi

# ---------------------------------------------------------------------------
# 7. Observation only. IDs, recomputed digests, counts and verdicts. No child
#    payload, no evidence write, no verdict of approval.
# ---------------------------------------------------------------------------
suite_digest() {
  local child doc out
  out="$(
    {
      printf 'PublicClaimSuiteV1\t%s\n' "$identity_after"
      for child in "${expected_children[@]}"; do
        printf '%s\t%s\t%s\n' "$child" "${digest_before[$child]}" "${verdict_of[$child]}"
      done
      for doc in "${public_documents[@]}"; do
        printf 'document\t%s\t%s\n' "$doc" "${document_digest_before[$doc]}"
      done
      printf '%s\t%s\tdependency-pass\n' "$redaction_dependency" "${digest_before[$redaction_dependency]}"
    } | sha256sum 2>/dev/null
  )" || infra 'digest' 'sha256sum failed while sealing the suite digest'
  printf 'sha256:%s' "${out%% *}"
}

children_json=''
for child in "${expected_children[@]}"; do
  [[ -z "$children_json" ]] || children_json+=','
  children_json+="$(printf '{"card":"%s","program_digest":"%s","verdict":"%s","observed_exit":0}' \
    "$child" "${digest_before[$child]}" "${verdict_of[$child]}")"
done

printf '{"card":"%s","scenario":"%s","selector":"%s","schema":"PublicClaimSuiteV1","children":%d,"children_reexecuted":%d,"candidate_identity":"%s","suite_digest":"%s","redaction_dependency":{"card":"%s","program_digest":"%s","observed_exit":0},"child_manifests":[%s],"verdict_file_consulted":false,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$expected_count" "$passed" \
  "$identity_after" "$(suite_digest)" \
  "$redaction_dependency" "${digest_before[$redaction_dependency]}" \
  "$children_json"
