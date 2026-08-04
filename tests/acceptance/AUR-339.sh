#!/usr/bin/env bash
# AUR-339 / AC-001 -- "Verificar os quatro settings locais sem dispatch".
#
# WHY THIS PROGRAM LOOKS LIKE IT DOES
#
# The previous revision read `tests/gates/legacy/AUR-339/suite.tsv` and accepted the
# literal string `pass` written in column 2 of that file, plus three digests it never
# recomputed -- it only checked that column 3 was equal to ITSELF across rows. Four
# hand-written lines in a repository file were therefore a green suite. That is the
# `"proved": true` pattern `.board/README.md` forbids: a claim is never a proof.
#
# The verdict of a child is now DERIVED, never read:
#
#   * the expected child set is the literal list embedded below, not a set discovered
#     from a file that any writer of the tree controls;
#   * each child's verdict comes from re-executing the child's own acceptance program
#     and observing its RAW exit code, and from re-deriving the child's manifest
#     invariants over CONTENT (the source settings file is re-hashed here);
#   * each digest in the suite manifest is compared against a digest this program
#     recomputes over the bytes of the named artifact -- never against a digest the
#     artifact declares about itself;
#   * CandidateIdentityV1 is recomputed here from the four recomputed pairs, so an
#     identity column can no longer be "valid" merely by agreeing with itself;
#   * the gate proves its own sensitivity: it builds hostile variants of the suite it
#     just accepted (missing child, duplicate, tampered digest, divergent identity,
#     non-pass verdict, fully hand-written "pass") and fails if any of them survives;
#   * each child is probed for sensitivity as well. A child that passes once has proven
#     nothing -- a child reduced to `exit 0` also passes. So the child is re-run in a
#     private shadow tree against inputs this gate deliberately corrupted, and a child
#     that still reports pass is not a check: the suite is refused. The tree under test
#     is never written to.
#
# An input this card never materializes -- a child's own acceptance program, a settings
# source outside `paths`/`read_paths` -- is an environment gap, so it exits 79. Turning a
# missing file into exit 1 would be indistinguishable from a real divergence.
#
# Exit codes, deliberately disjoint:
#   0   the promised behavior was observed;
#   1   behavioral RED, message `AUR-339/AC-001/<code>`;
#   64  unknown selector;
#   79  inconclusive: a harness file, a utility or a child's own environment made the
#       observation impossible. Never valid red evidence, never converted into one.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-339'
readonly scenario='AC-001'
readonly max_rows=128
readonly max_bytes=$((4 * 1024 * 1024))
readonly child_deadline=5
# The card bounds the whole gate at 20 s and the probe multiplies child runs, so the
# elapsed budget is checked between children. Running out of time is an environment
# condition, never a verdict.
readonly time_budget=18
readonly zero_digest='0000000000000000000000000000000000000000000000000000000000000000'

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR339|IntegrationAUR339|E2EAUR339) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit 1
}
# Environment failure is not behavioral divergence. It exits with its own code so a
# caller can tell "not proven" from "wrong".
inconclusive() {
  printf '%s/%s/inconclusive/%s\n' "$card" "$scenario" "$1" >&2
  exit 79
}

for tool in sha256sum wc mktemp rm timeout bash; do
  command -v "$tool" >/dev/null 2>&1 || inconclusive "missing-utility:$tool"
done
(( BASH_VERSINFO[0] >= 4 )) || inconclusive "bash-too-old:${BASH_VERSINFO[0]}"

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" \
  || inconclusive 'repo-root-unresolved'

work=''
cleanup() { [[ -z "$work" ]] || rm -rf -- "$work"; }
trap cleanup EXIT INT TERM HUP
work="$(mktemp -d 2>/dev/null)" || inconclusive 'mktemp-failed'

# ---------------------------------------------------------------------------------
# Embedded child set. This is the gate's own copy of what AUR-339 governs; it does
# not come from the tree under test. Each row: <card> <settings source> .
# The manifest of a child may never describe itself: the target path checked below is
# this literal, so a self-referential digest can never satisfy the join.
# ---------------------------------------------------------------------------------
readonly children=(AUR-386 AUR-387 AUR-388 AUR-389)
declare -A child_source=(
  [AUR-386]='.cursor/mcp.json'
  [AUR-387]='.gemini/settings.json'
  [AUR-388]='.mcp.json'
  [AUR-389]='.claude/settings.local.json'
)

digest_of() {
  local path="$1" out
  out="$(sha256sum -- "$path" 2>/dev/null)" || return 1
  printf '%s' "${out%% *}"
}

digest_of_string() {
  local out
  out="$(printf '%s' "$1" | sha256sum)" || return 1
  printf '%s' "${out%% *}"
}

regular_file() {
  [[ -f "$1" && ! -L "$1" ]]
}

bounded_file() {
  local path="$1" bytes
  bytes="$(wc -c <"$path" 2>/dev/null)" || return 1
  bytes="${bytes//[^0-9]/}"
  (( bytes > 0 && bytes <= max_bytes ))
}

# No pipe and no command substitution around the child: `$?` is read directly, because a
# filter's exit status would silently replace the producer's.
child_rc=0
run_child() {
  child_rc=0
  timeout -- "$child_deadline" bash -- "$1" AC-001 >/dev/null 2>&1 || child_rc=$?
}

# A shadow tree holding exactly the three inputs a settings child consumes. The probe
# corrupts these copies, never the tree under test. Copies are made writable because the
# materialized inputs are staged read-only.
shadow_build() {
  local child="$1" root="$2" rel="${child_source[$child]}"
  local prog="$root/tests/acceptance/$child.sh"
  local man="$root/tests/characterization/legacy/agent-settings/$child/manifest.tsv"
  rm -rf -- "$root" || return 1
  mkdir -p -- "$root/tests/acceptance" "$root/$(dirname -- "$rel")" \
    "$root/tests/characterization/legacy/agent-settings/$child" || return 1
  cp -- "$repo_root/tests/acceptance/$child.sh" "$prog" || return 1
  cp -- "$repo_root/$rel" "$root/$rel" || return 1
  cp -- "$repo_root/tests/characterization/legacy/agent-settings/$child/manifest.tsv" \
    "$man" || return 1
  chmod 0700 -- "$prog" || return 1
  chmod 0600 -- "$root/$rel" "$man" || return 1
}

# Copies the manifest with the digest column of the <rel> row replaced by a digest that
# describes nothing, so a child that really re-hashes its source must reject it.
mutate_manifest_digest() {
  local src="$1" dst="$2" rel="$3" line f1 f2 rest
  : >"$dst" || return 1
  while IFS= read -r line || [[ -n "$line" ]]; do
    IFS=$'\t' read -r f1 f2 rest <<<"$line"
    if [[ "$f1" == "$rel" ]]; then
      if [[ -n "$rest" ]]; then
        line="$f1"$'\t'"$zero_digest"$'\t'"$rest"
      else
        line="$f1"$'\t'"$zero_digest"
      fi
    fi
    printf '%s\n' "$line" >>"$dst" || return 1
  done <"$src"
}

# ---------------------------------------------------------------------------------
# Re-derive each child from content, then re-execute the child's own program.
# ---------------------------------------------------------------------------------
declare -A obs_verdict=() obs_spec=() obs_artifact=()

for child in "${children[@]}"; do
  program="$repo_root/tests/acceptance/$child.sh"
  manifest="$repo_root/tests/characterization/legacy/agent-settings/$child/manifest.tsv"
  source_rel="${child_source[$child]}"
  source_abs="$repo_root/$source_rel"

  # The child's program is outside this card's `paths`/`read_paths`, so it is absent from
  # a correctly materialized container. Absence is an environment gap, not a red verdict.
  regular_file "$program" || inconclusive "child-program-unmaterialized:$child"
  regular_file "$manifest" || fail "setting_manifest_missing:$child:manifest"
  bounded_file "$manifest" || fail "setting_manifest_missing:$child:manifest-bounds"

  # The settings file is the only thing whose bytes can anchor the child's digest.
  # If it cannot be read the observation is impossible, which is not a red verdict.
  regular_file "$source_abs" || inconclusive "source-unreadable:$child"
  source_sha="$(digest_of "$source_abs")" || inconclusive "source-unreadable:$child"

  rows=0
  target_seen=0
  digest_ok=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    rows=$(( rows + 1 ))
    (( rows <= max_rows )) || fail "setting_manifest_missing:$child:manifest-bounds"
    [[ -n "$line" ]] || continue
    IFS=$'\t' read -r f_target f_digest _rest <<<"$line"
    [[ "$f_target" == "$source_rel" ]] || continue
    target_seen=1

    [[ "$line" =~ commands_executed=([0-9]+) ]] \
      || fail "setting_manifest_missing:$child:commands_executed"
    (( BASH_REMATCH[1] == 0 )) || fail "mcp_executed:$child"

    [[ "$line" =~ secret_values=([0-9]+) ]] \
      || fail "setting_manifest_missing:$child:secret_values"
    (( BASH_REMATCH[1] == 0 )) || fail "secret_value_present:$child"

    # Recomputed over the bytes of the settings file, not read as a claim.
    [[ "$f_digest" == "$source_sha" ]] || fail "child_digest_mismatch:$child:source"
    digest_ok=1
  done < "$manifest"

  (( target_seen == 1 )) || fail "setting_manifest_missing:$child:$source_rel"
  (( digest_ok == 1 )) || fail "child_digest_mismatch:$child:source"

  # Re-execution: the child's exit code is observed raw, with no pipe in between.
  run_child "$program"
  rc="$child_rc"
  case "$rc" in
    0) ;;
    64|69|79|124|125|126|127) inconclusive "child-unobservable:$child:rc$rc" ;;
    *) fail "child_not_pass:$child:rc$rc" ;;
  esac

  # Sensitivity of the child itself. A green run only means the child did not object; a
  # child that objects to nothing is decoration. Both bytes the child claims to compare
  # are corrupted in turn, and the child must refuse each time.
  probe="$work/probe/$child"
  shadow_build "$child" "$probe" || inconclusive "probe-setup:$child"
  probe_program="$probe/tests/acceptance/$child.sh"
  probe_manifest="$probe/tests/characterization/legacy/agent-settings/$child/manifest.tsv"

  # Baseline in the shadow: if the child cannot pass on faithful copies, the probe is not
  # a valid instrument and its later red would not be attributable to the mutation.
  run_child "$probe_program"
  (( child_rc == 0 )) || inconclusive "probe-baseline:$child:rc$child_rc"

  printf 'x' >>"$probe/$source_rel" || inconclusive "probe-setup:$child"
  run_child "$probe_program"
  case "$child_rc" in
    0) fail "child_insensitive:$child:source" ;;
    124|125|126|127) inconclusive "probe-unobservable:$child:rc$child_rc" ;;
  esac
  cp -- "$source_abs" "$probe/$source_rel" || inconclusive "probe-setup:$child"
  chmod 0600 -- "$probe/$source_rel" || inconclusive "probe-setup:$child"

  mutate_manifest_digest "$manifest" "$probe_manifest" "$source_rel" \
    || inconclusive "probe-setup:$child"
  run_child "$probe_program"
  case "$child_rc" in
    0) fail "child_insensitive:$child:manifest" ;;
    124|125|126|127) inconclusive "probe-unobservable:$child:rc$child_rc" ;;
  esac

  (( SECONDS <= time_budget )) || inconclusive "time-budget-exceeded:${SECONDS}s"

  obs_spec[$child]="sha256:$(digest_of "$program")"
  obs_artifact[$child]="sha256:$(digest_of "$manifest")"
  obs_verdict[$child]='pass'
done

# CandidateIdentityV1: recomputed here from the recomputed pairs. A suite row can no
# longer establish an identity by agreeing with the other rows of the same file.
identity_input=''
for child in "${children[@]}"; do
  identity_input+="$child"$'\t'"${obs_spec[$child]}"$'\t'"${obs_artifact[$child]}"$'\n'
done
identity="sha256:$(digest_of_string "$identity_input")"

# ---------------------------------------------------------------------------------
# verify_suite <file> -- pure function of the file and the observations above.
# Prints one error code on stdout and returns 1, or returns 0 for a conformant suite.
# ---------------------------------------------------------------------------------
verify_suite() {
  local file="$1"
  local -A seen=()
  local rows=0 line id verdict ident spec artifact extra

  regular_file "$file" || { printf 'setting_manifest_missing:suite'; return 1; }
  bounded_file "$file" || { printf 'setting_manifest_missing:suite-bounds'; return 1; }

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" ]] || continue
    rows=$(( rows + 1 ))
    (( rows <= max_rows )) || { printf 'setting_manifest_missing:suite-bounds'; return 1; }

    IFS=$'\t' read -r id verdict ident spec artifact extra <<<"$line"
    if [[ -z "$id" || -z "$verdict" || -z "$ident" || -z "$spec" || -z "$artifact" || -n "$extra" ]]; then
      printf 'setting_manifest_missing:suite-row-%s' "$rows"; return 1
    fi
    if [[ -z "${obs_verdict[$id]+x}" ]]; then
      printf 'setting_manifest_missing:unexpected-child'; return 1
    fi
    if [[ -n "${seen[$id]+x}" ]]; then
      printf 'child_duplicate:%s' "$id"; return 1
    fi
    seen[$id]=1
    if [[ "$verdict" != "${obs_verdict[$id]}" ]]; then
      printf 'child_not_pass:%s' "$id"; return 1
    fi
    if [[ "$spec" != "${obs_spec[$id]}" || "$artifact" != "${obs_artifact[$id]}" ]]; then
      printf 'child_digest_mismatch:%s' "$id"; return 1
    fi
    if [[ "$ident" != "$identity" ]]; then
      printf 'candidate_identity_mismatch:%s' "$id"; return 1
    fi
  done < "$file"

  (( rows == ${#children[@]} )) || { printf 'setting_manifest_missing:count'; return 1; }
  local child
  for child in "${children[@]}"; do
    [[ -n "${seen[$child]+x}" ]] || { printf 'setting_manifest_missing:%s' "$child"; return 1; }
  done
  return 0
}

suite="$repo_root/tests/gates/legacy/AUR-339/suite.tsv"
reason="$(verify_suite "$suite" || true)"
[[ -z "$reason" ]] || fail "$reason"

# ---------------------------------------------------------------------------------
# Sensitivity. A gate that accepts a mutant has approved nothing, so the accepted
# suite is mutated here and every variant must be rejected with its own code.
# ---------------------------------------------------------------------------------
readonly mutants=(
  'child_missing:setting_manifest_missing'
  'child_duplicate:child_duplicate'
  'digest_tampered:child_digest_mismatch'
  'identity_divergent:candidate_identity_mismatch'
  'verdict_not_pass:child_not_pass'
  'handwritten_pass:child_digest_mismatch'
)
readonly fake_digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'

for spec_row in "${mutants[@]}"; do
  name="${spec_row%%:*}"
  want="${spec_row#*:}"
  variant="$work/$name.tsv"
  : >"$variant"
  case "$name" in
    child_missing)
      for child in "${children[@]:1}"; do
        printf '%s\tpass\t%s\t%s\t%s\n' "$child" "$identity" "${obs_spec[$child]}" "${obs_artifact[$child]}" >>"$variant"
      done ;;
    child_duplicate)
      for child in "${children[@]}"; do
        printf '%s\tpass\t%s\t%s\t%s\n' "$child" "$identity" "${obs_spec[$child]}" "${obs_artifact[$child]}" >>"$variant"
      done
      first="${children[0]}"
      printf '%s\tpass\t%s\t%s\t%s\n' "$first" "$identity" "${obs_spec[$first]}" "${obs_artifact[$first]}" >>"$variant" ;;
    digest_tampered)
      for child in "${children[@]}"; do
        art="${obs_artifact[$child]}"
        [[ "$child" != "${children[1]}" ]] || art="$fake_digest"
        printf '%s\tpass\t%s\t%s\t%s\n' "$child" "$identity" "${obs_spec[$child]}" "$art" >>"$variant"
      done ;;
    identity_divergent)
      for child in "${children[@]}"; do
        ident="$identity"
        [[ "$child" != "${children[2]}" ]] || ident="$fake_digest"
        printf '%s\tpass\t%s\t%s\t%s\n' "$child" "$ident" "${obs_spec[$child]}" "${obs_artifact[$child]}" >>"$variant"
      done ;;
    verdict_not_pass)
      for child in "${children[@]}"; do
        verdict='pass'
        [[ "$child" != "${children[3]}" ]] || verdict='fail'
        printf '%s\t%s\t%s\t%s\t%s\n' "$child" "$verdict" "$identity" "${obs_spec[$child]}" "${obs_artifact[$child]}" >>"$variant"
      done ;;
    handwritten_pass)
      # The exact forgery the skeptic reproduced: rows typed by hand, verdict `pass`,
      # digests invented and self-consistent across the file.
      for child in "${children[@]}"; do
        printf '%s\tpass\t%s\t%s\t%s\n' "$child" "$fake_digest" "$fake_digest" "$fake_digest" >>"$variant"
      done ;;
  esac

  got="$(verify_suite "$variant" || true)"
  [[ -n "$got" ]] || fail "suite_mutant_accepted:$name"
  [[ "${got%%:*}" == "$want" ]] || fail "suite_mutant_miscoded:$name:${got%%:*}"
done

# ---------------------------------------------------------------------------------
# AgentSettingSuiteV1 on stdout. Only IDs, digests, verdicts and counts leave here.
# ---------------------------------------------------------------------------------
printf '{"card":"%s","scenario":"%s","selector":"%s","schema":"AgentSettingSuiteV1"' \
  "$card" "$scenario" "$selector"
printf ',"candidate_identity":"%s","children":%s,"entries":[' "$identity" "${#children[@]}"
sep=''
for child in "${children[@]}"; do
  printf '%s{"card":"%s","verdict":"%s","spec_digest":"%s","artifact_digest":"%s"}' \
    "$sep" "$child" "${obs_verdict[$child]}" "${obs_spec[$child]}" "${obs_artifact[$child]}"
  sep=','
done
printf '],"result":"pass"}\n'
