#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

# AUR-337 -- GuideClaimSuiteV1 gate over the three legacy guide-ownership leaves.
#
# THREAT MODEL THIS GATE IS WRITTEN AGAINST
#
# The first revision read `tests/gates/legacy/AUR-337/suite.tsv` and accepted the literal
# string `pass` written in that file as the verdict of a child card, together with digest
# fields the file asserted about itself. Any writer of the repository could therefore mint
# three lines of TSV and turn the gate green without a single child card having run.
#
# The second revision re-executed each child, which killed the hand-written suite, but it
# still accepted whatever `tests/acceptance/<child>.sh` happened to print. Replacing that
# file with `printf '{"card":"AUR-384","result":"pass"}'` closed the suite with exit 0 and
# no work done. A child program that would print `pass` for any input is a mock that always
# agrees, and a mock is not a verdict.
#
# This gate now trusts nothing it cannot recompute or falsify:
#   * the expected child set is a literal embedded in THIS file, not read from anywhere;
#   * every child verdict comes from RE-EXECUTING the child's own acceptance program and
#     observing its raw exit status -- no file may assert a verdict on a child's behalf;
#   * every digest is recomputed by this gate over file CONTENT (sha256sum over the bytes),
#     never read from a field in which a file describes itself;
#   * every child is then re-run against a MUTATED copy of its own claim matrix inside a
#     private shadow root, and must go red. A child that still passes when the digests in
#     its evidence are replaced, or when its evidence is replaced by unrelated bytes, is
#     not proving anything and cannot contribute a pass to this suite. The pristine shadow
#     run is the positive control: without it the mutation would be vacuous;
#   * the discovered child set under the read path must equal the embedded set exactly, so
#     a missing member, an extra member, or a diverging count is a failure;
#   * an externally pinned CandidateIdentityV1 (supplied by the coordinator through the
#     environment, i.e. outside the repository) is compared against the identity this gate
#     recomputes; the pin can only ever make the gate stricter, never green.
#
# MATERIALIZATION NOTE (why a missing child program is code 3 and not code 1)
#   `tests/acceptance/<child>.sh` and the guide sources the children read live OUTSIDE this
#   card's `paths`/`read_paths`. A container that materializes only this card's allowlist
#   therefore cannot run a child at all. That is an environment condition -- the harness is
#   unusable -- and must never be reported as behavioural divergence of the card, because a
#   red produced by an unmaterialized input is indistinguishable from a real defect. The
#   card's `read_paths` has to grow to cover the child programs and their guide sources
#   before this gate can be closed inside the profile; until then it exits 3 there.
#
# EXIT CODES (a caller must be able to tell "not proven" from "wrong")
#   0   suite proven: every embedded child re-executed, returned pass, and went red under
#       mutation of its own evidence
#   1   behavioural divergence: typed error code from the card on stderr
#   3   infrastructure: the harness itself is unusable (missing tool, unresolvable root,
#       child program or child input not materialized, shadow control that will not build)
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
readonly max_sources=64
readonly max_source_rows=1024

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

for tool in awk grep sha256sum wc timeout mktemp cp ln env; do
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
readonly acceptance_root='tests/acceptance'

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

# Raw execution of a child. `env -u` stops BASH_ENV/ENV from injecting code into the
# child's shell before its first line. It is defence in depth only: an ambient environment
# able to set BASH_ENV has already hijacked THIS gate's own shell before line 1, and only
# the container profile can close that. It does close the narrower path where the gate's
# own environment carries the variable onward to the child.
run_child() {
  local prog="$1" out="$2" err="$3" rc=0
  timeout -k 1 "$child_deadline" env -u BASH_ENV -u ENV bash -- "$prog" AC-001 >"$out" 2>"$err" || rc=$?
  return "$rc"
}

# Mirror every entry of $1 into $2 as a symlink, except the protected names given after
# $2. Names the card declares forbidden are never mirrored at all.
readonly never_mirror=' .git .env credentials secrets .board '
readonly max_mirror_bytes=$((2 * 1024 * 1024))
link_siblings() {
  local src="$1" dst="$2"
  shift 2
  local protected=" $* "
  local entry name size
  mkdir -p -- "$dst" || return 1
  local -a entries=()
  shopt -s nullglob dotglob
  entries=("$src"/*)
  shopt -u nullglob dotglob
  for entry in "${entries[@]}"; do
    name="${entry##*/}"
    case "$protected" in *" $name "*) continue ;; esac
    case "$never_mirror" in *" $name "*) continue ;; esac
    [[ -e "$dst/$name" || -L "$dst/$name" ]] && continue
    # A regular file has to arrive as a real file: a child that refuses symlinked input
    # (as these children do) would otherwise fail the positive control for the wrong
    # reason. Directories stay symlinks, so nothing is copied recursively.
    if [[ -f "$entry" && ! -L "$entry" ]]; then
      size="$(wc -c <"$entry" 2>/dev/null)" || return 1
      size="${size//[[:space:]]/}"
      [[ "$size" =~ ^[0-9]+$ ]] || return 1
      if (( size <= max_mirror_bytes )); then
        cp -- "$entry" "$dst/$name" || return 1
        continue
      fi
    fi
    ln -s -- "$entry" "$dst/$name" || return 1
  done
  return 0
}

# A private root in which one child sees a real copy of its own program and a real copy of
# its own claim matrix, and symlinks to everything else. Only those two files can then
# explain a change in the child's behaviour.
build_shadow() {
  local child="$1" shadow="$2"
  mkdir -p -- "$shadow" || return 1
  link_siblings "$repo_root" "$shadow" tests || return 1
  link_siblings "$repo_root/tests" "$shadow/tests" acceptance characterization || return 1
  link_siblings "$repo_root/tests/characterization" "$shadow/tests/characterization" legacy || return 1
  link_siblings "$repo_root/tests/characterization/legacy" "$shadow/tests/characterization/legacy" guide-claims || return 1
  link_siblings "$repo_root/$claims_root" "$shadow/$claims_root" "$child" || return 1
  link_siblings "$repo_root/$claims_root/$child" "$shadow/$claims_root/$child" claims.tsv || return 1
  mkdir -p -- "$shadow/$acceptance_root" || return 1
  cp -- "$repo_root/$acceptance_root/$child.sh" "$shadow/$acceptance_root/$child.sh" || return 1
  return 0
}

# Turn every directory component of a repo-relative FILE path into a real directory inside
# the shadow, so the leaf can be rewritten without the write travelling back through a
# symlink into the repository. Refuses anything that is not a plain relative path.
detach_source_path() {
  local shadow="$1" rel="$2"
  local -a parts=()
  IFS='/' read -r -a parts <<<"$rel"
  (( ${#parts[@]} >= 1 )) || return 1
  local i prefix='' target next
  for ((i = 0; i < ${#parts[@]} - 1; i++)); do
    prefix="${prefix:+$prefix/}${parts[i]}"
    next="${parts[i + 1]}"
    target="$shadow/$prefix"
    [[ -d "$repo_root/$prefix" && ! -L "$repo_root/$prefix" ]] || return 1
    if [[ -L "$target" ]]; then
      rm -f -- "$target" || return 1
    fi
    link_siblings "$repo_root/$prefix" "$target" "$next" || return 1
    [[ -d "$target" && ! -L "$target" ]] || return 1
  done
  [[ ! -L "$shadow/$rel" ]] || rm -f -- "$shadow/$rel" || return 1
  return 0
}

# Repo-relative path shapes this gate is willing to touch inside its own shadow.
plain_relative_path() {
  local rel="$1"
  [[ "$rel" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]] || return 1
  case "$rel" in
    */|*//*|*/../*|../*|*/..) return 1 ;;
  esac
  case "$never_mirror" in *" ${rel%%/*} "*) return 1 ;; esac
  return 0
}

# The shadow builder walks the claim root component by component. If the literal ever
# changes, the walk below is stale and the mutation would silently stop biting.
[[ "$claims_root" == 'tests/characterization/legacy/guide-claims' ]] \
  || infra 'shadow builder is out of step with the claim root literal'

identity_stream="$workdir/identity.tsv"
: >"$identity_stream"
child_json=''

for child in "${embedded_children[@]}"; do
  prog="$acceptance_root/$child.sh"
  matrix="$claims_root/$child/claims.tsv"

  # Outside this card's read_paths: absence is a materialization fact, not a card defect.
  [[ -e "$prog" || -L "$prog" ]] \
    || infra "child acceptance program not materialized (outside this card's read_paths): $prog"
  [[ -f "$prog" && ! -L "$prog" && -r "$prog" && -s "$prog" ]] \
    || infra "child acceptance program is not a readable regular file: $prog"

  # Inside this card's read_paths: absence is exactly what AC-001 types.
  [[ -f "$matrix" && ! -L "$matrix" && -r "$matrix" && -s "$matrix" ]] \
    || fail guide_manifest_missing "child claim matrix absent, empty, or not a regular file: $matrix"

  prog_digest="$(digest_of "$prog")" || infra "digest unavailable for $prog"
  matrix_digest="$(digest_of "$matrix")" || infra "digest unavailable for $matrix"

  out_a="$workdir/$child.a.out"
  err_a="$workdir/$child.a.err"
  rc=0
  run_child "$prog" "$out_a" "$err_a" || rc=$?

  case "$rc" in
    0) ;;
    124|125|137) inconclusive child_timeout "$child did not finish within ${child_deadline}s" ;;
    3)  inconclusive child_infrastructure "$child reported its own harness unusable" ;;
    79) inconclusive child_inconclusive "$child could not reach a verdict" ;;
    *)  fail child_not_pass "$child re-execution exited $rc" ;;
  esac

  bytes="$(wc -c <"$out_a")" || infra "stdout of $child could not be measured"
  bytes="${bytes//[[:space:]]/}"
  [[ "$bytes" =~ ^[0-9]+$ ]] || infra "stdout size of $child is unreadable"
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
  run_child "$prog" "$out_b" "$err_b" || rc=$?
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

  # -------------------------------------------------------------------------------
  # Falsification. A pass that survives the destruction of the evidence it claims to
  # rest on is not a pass. Build a shadow root, prove the child still passes there
  # (positive control), then damage ONLY its claim matrix and require it to go red.
  # -------------------------------------------------------------------------------
  shadow="$workdir/shadow/$child"
  build_shadow "$child" "$shadow" || infra "shadow control could not be built for $child"
  shadow_prog="$shadow/$acceptance_root/$child.sh"
  shadow_matrix="$shadow/$claims_root/$child/claims.tsv"

  cp -- "$matrix" "$shadow_matrix" || infra "shadow claim matrix could not be staged for $child"
  shadow_prog_digest="$(digest_of "$shadow_prog")" || infra "digest unavailable for the shadow copy of $prog"
  [[ "$shadow_prog_digest" == "$prog_digest" ]] \
    || infra "shadow copy of $prog does not match the program that was executed"

  rc=0
  run_child "$shadow_prog" "$workdir/$child.ctl.out" "$workdir/$child.ctl.err" || rc=$?
  (( rc == 0 )) \
    || infra "shadow control for $child did not reproduce its real pass (exit $rc); the harness, not the card, is unproven"
  grep -Fq "\"card\":\"$child\"" "$workdir/$child.ctl.out" \
    || infra "shadow control for $child produced no result record"

  # MUT-A: every recorded source digest replaced. An honest child binds its claims to the
  # bytes of the guide it describes and must reject this matrix.
  awk -F '\t' -v OFS='\t' -v d='0000000000000000000000000000000000000000000000000000000000000000' \
    'NF >= 2 { $2 = d } { print }' "$matrix" >"$shadow_matrix" \
    || infra "digest mutation could not be written for $child"
  mutant_digest="$(digest_of "$shadow_matrix")" || infra "digest unavailable for the mutated matrix of $child"
  [[ -s "$shadow_matrix" ]] || infra "digest mutation emptied the claim matrix of $child"
  if [[ "$mutant_digest" == "$matrix_digest" ]]; then
    infra "digest mutation left the claim matrix of $child unchanged"
  fi
  rc=0
  run_child "$shadow_prog" "$workdir/$child.mutA.out" "$workdir/$child.mutA.err" || rc=$?
  (( rc != 0 )) \
    || fail child_digest_mismatch "$child still returned pass with every source digest in its claim matrix replaced"

  # MUT-B: the matrix replaced by unrelated, non-empty bytes. A child that only checks that
  # the file exists, or that ignores it entirely, still passes here.
  printf 'AUR-337-mutant\tnot-a-digest\tno-disposition\n' >"$shadow_matrix" \
    || infra "content mutation could not be written for $child"
  rc=0
  run_child "$shadow_prog" "$workdir/$child.mutB.out" "$workdir/$child.mutB.err" || rc=$?
  (( rc != 0 )) \
    || fail child_not_pass "$child still returned pass with its claim matrix replaced by unrelated content"

  # MUT-C: the guide sources the matrix itself names are altered, so the digests the child
  # claims about them stop holding. The paths come from the child's own evidence, not from
  # a table in this file, so the aggregator still owns no implementation path of a child.
  # A child that passes here is verifying the identity of its inputs and nothing about the
  # relation between them.
  source_list="$workdir/$child.sources"
  rebind_list="$workdir/$child.rebind"
  : >"$rebind_list"
  awk -F '\t' 'NF >= 2 && $1 != "" && !seen[$1]++ { print $1 }' "$matrix" >"$source_list" \
    || infra "claim matrix of $child could not be scanned for source paths"

  cp -- "$matrix" "$shadow_matrix" || infra "shadow claim matrix could not be restored for $child"

  mutated_sources=0
  scanned_sources=0
  while IFS= read -r source_rel; do
    scanned_sources=$((scanned_sources + 1))
    (( scanned_sources <= max_source_rows )) || infra "claim matrix of $child holds more than $max_source_rows rows"
    (( mutated_sources < max_sources )) || infra "claim matrix of $child names more than $max_sources sources"
    plain_relative_path "$source_rel" || continue
    [[ -f "$repo_root/$source_rel" && ! -L "$repo_root/$source_rel" ]] || continue
    pristine_source_digest="$(digest_of "$repo_root/$source_rel")" || infra "digest unavailable for $source_rel"
    detach_source_path "$shadow" "$source_rel" \
      || infra "shadow could not be detached for $source_rel"
    cp -- "$repo_root/$source_rel" "$shadow/$source_rel" || infra "shadow copy of $source_rel failed"
    printf 'AUR-337-source-mutation\n' >>"$shadow/$source_rel" || infra "source mutation of $source_rel failed"
    mutant_source_digest="$(digest_of "$shadow/$source_rel")" || infra "digest unavailable for the mutated $source_rel"
    [[ "$mutant_source_digest" != "$pristine_source_digest" ]] \
      || infra "source mutation of $source_rel changed nothing"
    # The write must never have reached the repository through a surviving symlink.
    [[ "$(digest_of "$repo_root/$source_rel")" == "$pristine_source_digest" ]] \
      || infra "source mutation escaped the shadow and touched $source_rel"
    printf '%s\t%s\n' "$source_rel" "$mutant_source_digest" >>"$rebind_list"
    mutated_sources=$((mutated_sources + 1))
  done <"$source_list"

  (( mutated_sources > 0 )) \
    || infra "claim matrix of $child names no readable source file, so its binding cannot be falsified"

  rc=0
  run_child "$shadow_prog" "$workdir/$child.mutC.out" "$workdir/$child.mutC.err" || rc=$?
  (( rc != 0 )) \
    || fail child_digest_mismatch "$child still returned pass after the $mutated_sources source file(s) its own claim matrix binds were altered"

  # MUT-D: the same altered sources, but now the matrix records their NEW digests. The
  # evidence is internally consistent again, so a child that verifies the RELATION between
  # its matrix and its sources must return to pass. A child that merely pins the digests it
  # was born with cannot. This one is inconclusive, never red: a child that is stricter than
  # this gate knows (line ranges, byte counts) may legitimately refuse, and the board
  # forbids promoting "could not tell" into "wrong".
  awk -F '\t' -v OFS='\t' \
    'NR == FNR { nd[$1] = $2; next } NF >= 2 && ($1 in nd) { $2 = nd[$1] } { print }' \
    "$rebind_list" "$matrix" >"$shadow_matrix" \
    || infra "rebound claim matrix could not be written for $child"
  [[ -s "$shadow_matrix" ]] || infra "rebound claim matrix of $child is empty"
  rc=0
  run_child "$shadow_prog" "$workdir/$child.mutD.out" "$workdir/$child.mutD.err" || rc=$?
  (( rc == 0 )) \
    || inconclusive child_binding_unfalsifiable \
       "$child did not return to pass once its claim matrix was rebound to the altered sources (exit $rc); its verdict cannot be shown to depend on the relation between evidence and source"
  grep -Fq "\"result\":\"pass\"" "$workdir/$child.mutD.out" \
    || inconclusive child_binding_unfalsifiable "$child exited 0 on the rebound matrix without declaring pass"

  printf '%s\t%s\t%s\t%s\n' "$child" "$prog_digest" "$matrix_digest" "$stdout_digest" >>"$identity_stream"

  child_json="$child_json{\"card\":\"$child\",\"program\":\"sha256:$prog_digest\",\"matrix\":\"sha256:$matrix_digest\",\"stdout\":\"sha256:$stdout_digest\"},"
done

rows="$(wc -l <"$identity_stream")" || infra 'identity stream could not be measured'
rows="${rows//[[:space:]]/}"
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
