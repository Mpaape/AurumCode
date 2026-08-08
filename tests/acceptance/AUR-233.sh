#!/usr/bin/env bash
# AUR-233 / AC-001 -- "Verificar lockset completo antes do bootstrap".
#
# This program is an aggregation gate over the six bootstrap lock children
# [AUR-359, AUR-360, AUR-361, AUR-362, AUR-363, AUR-364]. It never reads a
# verdict out of a repository file. `.board/README.md` is explicit: a
# repository value such as `authenticated: true` or `"proved": true` is
# untrusted data, and a claim is never a proof. Consequently:
#
#   * the expected child set is a LITERAL embedded in this script (see
#     `children` below). No repository file may widen, narrow or reorder it;
#   * every child verdict is obtained by RE-EXECUTING that child's own
#     acceptance program and observing its raw exit code -- never by reading
#     the string `pass` out of a manifest. The lockset index is forbidden from
#     even carrying a verdict token;
#   * re-execution alone is not enough: a program whose whole body is `exit 0`
#     also exits 0. So each child is first PROBED with an unknown selector and
#     must refuse it with 64, and the stdout of the accepted run must be a
#     record the child OWNS (`"card":"<child>"`). The exit code stays the
#     verdict; the probe and the record only prove the thing that exited 0 was
#     that child's acceptance program and not a stub;
#   * the artifact path of every child is the EMBEDDED one. The index cannot
#     rebind a child to another file, and six children cannot alias one file;
#   * every digest in the index is RECOMPUTED here over the CONTENT of the
#     referenced artifact. A digest a file declares about itself is not
#     evidence, and is rejected because the recomputation will not match it;
#   * the aggregate `CandidateIdentityV1` is recomputed from the sorted
#     (card id, recomputed artifact digest) pairs and compared to the declared
#     one; it is never copied out of the index.
#
# Lock domain contract, owned by this card (`.board/bootstrap/`):
#
#   locks.yml            index, regular file, <= 64 KiB, LC_ALL=C text
#     schema: bootstrap-lockset-v1                            exactly once
#     identity: sha256:<64 hex>                               exactly once
#     child: <AUR-nnn> <locks/<name>.yml> <sha256:<64 hex>>   exactly six times
#   locks/<name>.yml     the child-owned artifact, regular file, <= 4 MiB
#
#   Blank lines and `#` comments are ignored. Any other line, any token count
#   other than the one above, and any occurrence of a verdict word
#   (pass/fail/verdict/proved/authenticated/approved) rejects the domain.
#
# Exit codes, deliberately disjoint:
#   0   the promised behavior was observed; stdout carries one JSON object.
#   1   behavioral RED, message `AUR-233/AC-001/<code>: <detail>`, where <code>
#       is one of the five typed codes the card owns --
#       `lock_domain_missing`, `child_duplicate`, `child_digest_mismatch`,
#       `candidate_identity_mismatch`, `child_not_pass` -- plus
#       `gate_insensitive`, raised when this gate's own verifier fails to
#       refuse a mutant lockset.
#   64  unknown selector.
#   69  infrastructure / INCONCLUSIVE, message
#       `AUR-233/AC-001/infrastructure/<reason>`. A missing host utility, an
#       unusable temporary directory, a child that times out, a child that
#       rejects the selector (64), or a child that raised its own
#       infrastructure diagnosis (69) is inconclusive. Absent tooling is never
#       converted into behavioral RED.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-233'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR233|IntegrationAUR233|E2EAUR233) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"

# The authoritative child set. Embedded on purpose: it is the one input the
# coordinator can re-derive without trusting the tree under test.
readonly -a children=(AUR-359 AUR-360 AUR-361 AUR-362 AUR-363 AUR-364)
readonly -a child_locks=(
  locks/trust-root.yml
  locks/go.yml
  locks/actions.yml
  locks/scanners.yml
  locks/parsers.yml
  locks/docs.yml
)
readonly expected_count=6
readonly max_index_bytes=65536
readonly max_artifact_bytes=4194304
readonly max_index_lines=512
readonly child_deadline_seconds=30
readonly max_child_stdout_bytes=65536
readonly aggregate_deadline_seconds=360
readonly cases_file="$repo_root/tests/gates/bootstrap/AUR-233/cases.tsv"
case_names=(nominal missing duplicate tampered identity selfref handwritten aliased malformed symlink)
declare -A case_expected=()

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

work=''
cleanup() { [[ -z "$work" ]] || rm -rf -- "$work"; }
trap cleanup EXIT INT TERM HUP

for tool in sha256sum mktemp sort grep wc cp mkdir ln timeout awk; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-utility/$tool"
done
work="$(mktemp -d 2>/dev/null)" || infra 'mktemp-failed'
[[ -d "$work" && -w "$work" ]] || infra 'workdir-unusable'
run_started=$SECONDS
deadline_guard() {
  (( SECONDS - run_started <= aggregate_deadline_seconds )) || infra 'aggregate-timeout'
}

# digest_of <file> -> `sha256:<hex>`, recomputed over the file CONTENT.
digest_of() {
  local out
  out="$(sha256sum -- "$1" 2>/dev/null)" || return 1
  printf 'sha256:%s' "${out%% *}"
}

# flip_digest <sha256:hex> -> the same shape with a guaranteed different value.
flip_digest() {
  local d="$1" last="${1: -1}"
  if [[ "$last" == '0' ]]; then printf '%s1' "${d:0:${#d}-1}"; else printf '%s0' "${d:0:${#d}-1}"; fi
}

# VerifyBootstrapLocksetV1 <domain_root>
#   0 -> prints `ok <identity>`; 1 -> prints `<typed_code> <detail>`;
#   2 -> prints `infra <detail>`.
# Reads only the index and the artifacts it references, executes nothing, and
# accepts no field it cannot recompute.
VerifyBootstrapLocksetV1() {
  local root="$1"
  local index="$root/locks.yml"
  local locks_dir="$root/locks"
  local line id rel declared extra sz lineno=0
  local schema_seen=0 ident_seen=0 ident='' nchild=0
  local -a ids=() rels=() decs=()

  if [[ ! -e "$index" ]]; then
    printf 'lock_domain_missing lockset index absent at %s\n' "$index"; return 1
  fi
  if [[ -L "$index" || ! -f "$index" ]]; then
    printf 'lock_domain_missing lockset index is not a regular file\n'; return 1
  fi
  if [[ -L "$locks_dir" || ! -d "$locks_dir" ]]; then
    printf 'lock_domain_missing lock artifact directory absent at %s\n' "$locks_dir"; return 1
  fi
  sz="$(wc -c < "$index" 2>/dev/null)" || { printf 'infra cannot size %s\n' "$index"; return 2; }
  if (( sz == 0 || sz > max_index_bytes )); then
    printf 'lock_domain_missing lockset index out of bounds (%s bytes)\n' "$sz"; return 1
  fi
  # A verdict word inside the index is the exact "proved: true" pattern the
  # board forbids. Refuse before parsing, so no hand-written `pass` can matter.
  if grep -Eiq '(^|[^A-Za-z])(pass|passed|fail|failed|verdict|proved|authenticated|approved)([^A-Za-z]|$)' "$index"; then
    printf 'lock_domain_missing verdict claim present in index; a claim is never a proof\n'; return 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$(( lineno + 1 ))
    if (( lineno > max_index_lines )); then
      printf 'lock_domain_missing lockset index exceeds %s lines\n' "$max_index_lines"; return 1
    fi
    [[ -n "${line//[[:space:]]/}" ]] || continue
    [[ "${line:0:1}" != '#' ]] || continue
    if [[ "$line" == schema:* ]]; then
      if [[ "$line" != 'schema: bootstrap-lockset-v1' ]]; then
        printf 'lock_domain_missing unexpected schema at line %s\n' "$lineno"; return 1
      fi
      schema_seen=$(( schema_seen + 1 )); continue
    fi
    if [[ "$line" == identity:* ]]; then
      ident="${line#identity: }"
      if [[ ! "$ident" =~ ^sha256:[0-9a-f]{64}$ ]]; then
        printf 'candidate_identity_mismatch malformed identity at line %s\n' "$lineno"; return 1
      fi
      ident_seen=$(( ident_seen + 1 )); continue
    fi
    if [[ "$line" == 'child: '* ]]; then
      id=''; rel=''; declared=''; extra=''
      read -r id rel declared extra <<< "${line#child: }" || true
      if [[ -z "$id" || -z "$rel" || -z "$declared" || -n "$extra" ]]; then
        printf 'lock_domain_missing malformed child entry at line %s\n' "$lineno"; return 1
      fi
      local known=0 ci expected_rel=''
      for ci in "${!children[@]}"; do
        if [[ "${children[ci]}" == "$id" ]]; then known=1; expected_rel="${child_locks[ci]}"; fi
      done
      if (( known == 0 )); then
        printf 'lock_domain_missing child %s is not in the embedded set (line %s)\n' "$id" "$lineno"; return 1
      fi
      local prev
      for prev in ${ids[@]+"${ids[@]}"}; do
        if [[ "$prev" == "$id" ]]; then
          printf 'child_duplicate child %s declared twice (line %s)\n' "$id" "$lineno"; return 1
        fi
      done
      if [[ ! "$rel" =~ ^locks/[A-Za-z0-9][A-Za-z0-9._-]*\.yml$ ]]; then
        printf 'lock_domain_missing child %s has an unusable artifact path\n' "$id"; return 1
      fi
      # The path is embedded, not negotiated: the index may not rebind a child
      # to another artifact, and it may not alias several children onto one.
      if [[ "$rel" != "$expected_rel" ]]; then
        printf 'lock_domain_missing child %s must bind %s but the index binds %s\n' "$id" "$expected_rel" "$rel"; return 1
      fi
      if [[ ! "$declared" =~ ^sha256:[0-9a-f]{64}$ ]]; then
        printf 'child_digest_mismatch child %s declares a malformed digest\n' "$id"; return 1
      fi
      ids+=("$id"); rels+=("$rel"); decs+=("$declared")
      nchild=$(( nchild + 1 ))
      continue
    fi
    printf 'lock_domain_missing unparsable line %s in lockset index\n' "$lineno"; return 1
  done < "$index"

  if (( schema_seen != 1 )); then
    printf 'lock_domain_missing lockset index must declare the schema exactly once\n'; return 1
  fi
  if (( ident_seen != 1 )); then
    printf 'candidate_identity_mismatch lockset index must declare identity exactly once\n'; return 1
  fi
  if (( nchild != expected_count )); then
    printf 'lock_domain_missing child count %s diverges from the embedded set (%s)\n' "$nchild" "$expected_count"; return 1
  fi
  local want seen
  for want in "${children[@]}"; do
    seen=0
    for id in "${ids[@]}"; do [[ "$id" != "$want" ]] || seen=1; done
    if (( seen == 0 )); then
      printf 'lock_domain_missing child %s absent from the lockset index\n' "$want"; return 1
    fi
  done

  local canon="$work/canon.$$.$RANDOM"
  : > "$canon" || { printf 'infra cannot write canonical buffer\n'; return 2; }
  local i art recomputed
  for (( i = 0; i < nchild; i++ )); do
    art="$root/${rels[i]}"
    if [[ -L "$art" || ! -f "$art" ]]; then
      printf 'child_digest_mismatch artifact for %s absent at %s\n' "${ids[i]}" "${rels[i]}"; return 1
    fi
    sz="$(wc -c < "$art" 2>/dev/null)" || { printf 'infra cannot size %s\n' "$art"; return 2; }
    if (( sz == 0 || sz > max_artifact_bytes )); then
      printf 'child_digest_mismatch artifact for %s out of bounds (%s bytes)\n' "${ids[i]}" "$sz"; return 1
    fi
    recomputed="$(digest_of "$art")" || { printf 'infra cannot hash %s\n' "$art"; return 2; }
    if [[ "$recomputed" != "${decs[i]}" ]]; then
      printf 'child_digest_mismatch %s declares %s but its content hashes to %s\n' "${ids[i]}" "${decs[i]}" "$recomputed"; return 1
    fi
    printf '%s %s\n' "${ids[i]}" "$recomputed" >> "$canon"
  done

  local sorted="$canon.sorted" computed
  sort "$canon" > "$sorted" || { printf 'infra cannot sort canonical buffer\n'; return 2; }
  computed="$(digest_of "$sorted")" || { printf 'infra cannot hash canonical buffer\n'; return 2; }
  if [[ "$computed" != "$ident" ]]; then
    printf 'candidate_identity_mismatch index declares %s but the lockset recomputes to %s\n' "$ident" "$computed"; return 1
  fi
  printf 'ok %s\n' "$computed"
  return 0
}

# ObserveChildVerdictV1 <child_id> <program> <scratch_dir>
#   0 -> prints `ok`; 1 -> prints `<typed_code> <detail>`; 2 -> `infra <detail>`.
# The child's raw exit code is the verdict. Nothing is read out of a file, and
# the two extra assertions exist only to refuse a program that answers 0 to
# everything. A program the container never materialized is INCONCLUSIVE (2),
# never behavioral RED: the card may read only `.board/bootstrap/locks`, so an
# absent sibling program is a materialization fact, not a divergence.
ObserveChildVerdictV1() {
  local child="$1" prog="$2" dir="$3"
  local out="$dir/$child.out" err="$dir/$child.err" rc probe_rc bytes remaining child_timeout

  if [[ -L "$prog" || ! -f "$prog" || ! -r "$prog" ]]; then
    printf 'infra child-program-unavailable/%s\n' "$child"; return 2
  fi
  if [[ ! -s "$prog" ]]; then
    printf 'infra child-program-empty/%s\n' "$child"; return 2
  fi

  # Sensitivity probe. Every acceptance program on this board refuses an
  # unknown selector with 64; `exit 0` answers 0 to that too, and that is the
  # exact forgery this probe is here to catch.
  deadline_guard
  remaining=$((aggregate_deadline_seconds - (SECONDS - run_started)))
  (( remaining > 0 )) || { printf 'infra aggregate-timeout\n'; return 2; }
  child_timeout="$child_deadline_seconds"
  (( remaining < child_timeout )) && child_timeout="$remaining"
  set +e
  BASH_ENV= ENV= timeout -k 0 "$child_timeout" bash --noprofile --norc -- "$prog" __AUR233_probe__ \
    >"$dir/$child.probe.out" 2>"$dir/$child.probe.err"
  probe_rc=$?
  set -e
  case "$probe_rc" in
    64) ;;
    0) printf 'child_not_pass %s answered 0 to an unknown selector; it does not discriminate\n' "$child"; return 1 ;;
    69) printf 'infra child-infrastructure/%s/probe\n' "$child"; return 2 ;;
    124|137) printf 'infra child-timeout/%s/probe\n' "$child"; return 2 ;;
    *) printf 'child_not_pass %s must refuse an unknown selector with 64, it exited %s\n' "$child" "$probe_rc"; return 1 ;;
  esac

  # Raw status inside the `if`-free straight line: no pipe, no filter, nothing
  # that could swallow the producer's exit code.
  deadline_guard
  remaining=$((aggregate_deadline_seconds - (SECONDS - run_started)))
  (( remaining > 0 )) || { printf 'infra aggregate-timeout\n'; return 2; }
  child_timeout="$child_deadline_seconds"
  (( remaining < child_timeout )) && child_timeout="$remaining"
  set +e
  BASH_ENV= ENV= timeout -k 0 "$child_timeout" bash --noprofile --norc -- "$prog" AC-001 >"$out" 2>"$err"
  rc=$?
  set -e
  case "$rc" in
    0) ;;
    1|3) printf 'child_not_pass %s was re-executed and exited %s\n' "$child" "$rc"; return 1 ;;
    64) printf 'infra child-selector-rejected/%s\n' "$child"; return 2 ;;
    69) printf 'infra child-infrastructure/%s\n' "$child"; return 2 ;;
    124|137) printf 'infra child-timeout/%s\n' "$child"; return 2 ;;
    *) printf 'infra child-inconclusive/%s/exit-%s\n' "$child" "$rc"; return 2 ;;
  esac

  bytes="$(wc -c < "$out" 2>/dev/null)" || { printf 'infra cannot size child stdout for %s\n' "$child"; return 2; }
  if (( bytes == 0 || bytes > max_child_stdout_bytes )); then
    printf 'child_not_pass %s exited 0 but its record is out of bounds (%s bytes)\n' "$child" "$bytes"; return 1
  fi
  [[ "$(wc -l < "$out")" == 1 ]] || {
    printf 'child_not_pass %s emitted more than one record\n' "$child"; return 1
  }
  local record pattern
  record="$(< "$out")"
  if ! awk '
    {
      text = $0
      while (match(text, /"[a-z_]+":/)) {
        key = substr(text, RSTART + 1, RLENGTH - 3)
        count[key]++
        if (count[key] > 1) exit 1
        text = substr(text, RSTART + RLENGTH)
      }
    }
  ' "$out"; then
    printf 'child_not_pass %s emitted duplicate JSON keys\n' "$child"; return 1
  fi
  pattern='^\{"card":"'"$child"'","scenario":"AC-001"(,"[a-z_]+":("[A-Za-z0-9_.:-]+"|[0-9]+))*\,"result":"pass"\}$'
  if [[ ! "$record" =~ $pattern ]]; then
    printf 'child_not_pass %s emitted a non-canonical pass record\n' "$child"; return 1
  fi
  printf 'ok\n'
  return 0
}

check_case_matrix() {
  [[ -f "$cases_file" && ! -L "$cases_file" ]] || infra 'case-matrix-missing'
  declare -A seen_cases=()
  declare -A expected_results=(
    [nominal]=ok
    [missing]=lock_domain_missing
    [duplicate]=child_duplicate
    [tampered]=child_digest_mismatch
    [identity]=candidate_identity_mismatch
    [selfref]=child_digest_mismatch
    [handwritten]=lock_domain_missing
    [aliased]=lock_domain_missing
    [malformed]=lock_domain_missing
    [symlink]=child_digest_mismatch
  )
  local line name expected mutation extra count=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "${line//[[:space:]]/}" || "${line:0:1}" == '#' ]] && continue
    [[ "$line" == *$'\t'* ]] || fail 'gate_insensitive: case matrix is not tab-separated'
    IFS=$'\t' read -r name expected mutation extra <<< "$line"
    [[ -n "${name:-}" && -n "${expected:-}" && -n "${mutation:-}" && -z "${extra:-}" ]] ||
      fail 'gate_insensitive: malformed case matrix row'
    [[ -n "${expected_results[$name]+x}" && "${expected_results[$name]}" == "$expected" ]] ||
      fail "gate_insensitive: unexpected expected result for $name"
    [[ -z "${seen_cases[$name]+x}" ]] || fail "gate_insensitive: duplicate case matrix row $name"
    seen_cases[$name]="$expected"
    case_expected[$name]="$expected"
    count=$((count + 1))
  done < "$cases_file"
  (( count == ${#case_names[@]} )) || fail "gate_insensitive: case matrix has $count rows"
  for name in "${case_names[@]}"; do
    [[ -n "${seen_cases[$name]+x}" ]] || fail "gate_insensitive: case matrix misses $name"
  done
}

# ---------------------------------------------------------------------------
# Phase 1 -- prove this gate refuses mutants before it is allowed to accept
# anything. A gate that never refuses cannot certify the cards it approved.
# ---------------------------------------------------------------------------
check_case_matrix
write_index() { # write_index <out> <identity> <row>...
  local out="$1" identity="$2" row
  shift 2
  {
    printf 'schema: bootstrap-lockset-v1\n'
    printf 'identity: %s\n' "$identity"
    for row in "$@"; do printf 'child: %s\n' "$row"; done
  } > "$out"
}

mut="$work/mutants"
mkdir -p "$mut/nominal/locks" || infra 'cannot-build-fixture'
declare -a rows=()
declare -a digs=()
for i in "${!children[@]}"; do
  art="$mut/nominal/${child_locks[i]}"
  printf 'schema: bootstrap-lock-v1\ncard: %s\nsource: synthetic gate fixture\n' "${children[i]}" > "$art" \
    || infra 'cannot-build-fixture'
  digs[i]="$(digest_of "$art")" || infra 'cannot-hash-fixture'
  rows[i]="${children[i]} ${child_locks[i]} ${digs[i]}"
done
canon="$mut/canon"
: > "$canon" || infra 'cannot-build-fixture'
for i in "${!children[@]}"; do
  printf '%s %s\n' "${children[i]}" "${digs[i]}" >> "$canon" || infra 'cannot-build-fixture'
done
sort "$canon" > "$canon.sorted" || infra 'cannot-build-fixture'
fix_identity="$(digest_of "$canon.sorted")" || infra 'cannot-hash-fixture'
write_index "$mut/nominal/locks.yml" "$fix_identity" "${rows[@]}" || infra 'cannot-build-fixture'

derive() { cp -a "$mut/nominal" "$mut/$1" || infra 'cannot-build-fixture'; }

# MUT: a required child dropped from the index.
derive missing
declare -a rows_missing=()
for i in "${!children[@]}"; do
  [[ "${children[i]}" == 'AUR-363' ]] || rows_missing+=("${rows[i]}")
done
write_index "$mut/missing/locks.yml" "$fix_identity" "${rows_missing[@]}" || infra 'cannot-build-fixture'

# MUT: a child declared twice.
derive duplicate
write_index "$mut/duplicate/locks.yml" "$fix_identity" "${rows[@]}" "${rows[0]}" || infra 'cannot-build-fixture'

# MUT: one artifact digest tampered with in the index.
derive tampered
declare -a rows_tampered=()
for i in "${!children[@]}"; do
  if [[ "${children[i]}" == 'AUR-362' ]]; then
    rows_tampered+=("${children[i]} ${child_locks[i]} $(flip_digest "${digs[i]}")")
  else
    rows_tampered+=("${rows[i]}")
  fi
done
write_index "$mut/tampered/locks.yml" "$fix_identity" "${rows_tampered[@]}" || infra 'cannot-build-fixture'

# MUT: divergent aggregate identity.
derive identity
write_index "$mut/identity/locks.yml" "$(flip_digest "$fix_identity")" "${rows[@]}" || infra 'cannot-build-fixture'

# MUT: the skeptic's self-referential digest -- the index declares, for one
# child, the digest of the index file itself.
derive selfref
selfdig="$(digest_of "$mut/selfref/locks.yml")" || infra 'cannot-hash-fixture'
declare -a rows_selfref=()
for i in "${!children[@]}"; do
  if [[ "${children[i]}" == 'AUR-361' ]]; then
    rows_selfref+=("${children[i]} ${child_locks[i]} $selfdig")
  else
    rows_selfref+=("${rows[i]}")
  fi
done
write_index "$mut/selfref/locks.yml" "$fix_identity" "${rows_selfref[@]}" || infra 'cannot-build-fixture'

# MUT: the skeptic's hand-written verdict table -- six `pass` rows, no artifact.
mkdir -p "$mut/handwritten/locks" || infra 'cannot-build-fixture'
declare -a rows_hand=()
for i in "${!children[@]}"; do
  rows_hand+=("${children[i]} pass sha256:$(printf '%064d' 0)")
done
write_index "$mut/handwritten/locks.yml" "sha256:$(printf '%064d' 0)" "${rows_hand[@]}" \
  || infra 'cannot-build-fixture'

# MUT: the index rebinds every child onto ONE artifact, at a path the embedded
# set does not assign to them. Digests and identity are internally consistent,
# so only the embedded path binding can refuse it.
mkdir -p "$mut/aliased/locks" || infra 'cannot-build-fixture'
cp -a "$mut/nominal/${child_locks[0]}" "$mut/aliased/${child_locks[0]}" || infra 'cannot-build-fixture'
alias_canon="$mut/alias-canon"
: > "$alias_canon" || infra 'cannot-build-fixture'
declare -a rows_alias=()
for i in "${!children[@]}"; do
  rows_alias+=("${children[i]} ${child_locks[0]} ${digs[0]}")
  printf '%s %s\n' "${children[i]}" "${digs[0]}" >> "$alias_canon" || infra 'cannot-build-fixture'
done
sort "$alias_canon" > "$alias_canon.sorted" || infra 'cannot-build-fixture'
alias_identity="$(digest_of "$alias_canon.sorted")" || infra 'cannot-hash-fixture'
write_index "$mut/aliased/locks.yml" "$alias_identity" "${rows_alias[@]}" || infra 'cannot-build-fixture'

# MUT: an otherwise well-shaped index contains an unknown line.
derive malformed
printf 'unexpected: value\n' >> "$mut/malformed/locks.yml" || infra 'cannot-build-fixture'

# MUT: an artifact is a symlink, not a regular child-owned file.
derive symlink
rm "$mut/symlink/locks/trust-root.yml" || infra 'cannot-build-fixture'
ln -s "$mut/nominal/locks/trust-root.yml" "$mut/symlink/locks/trust-root.yml" || infra 'cannot-build-fixture'

matrix=''
for name in "${case_names[@]}"; do matrix+=" $name:${case_expected[$name]}"; done
for entry in $matrix; do
  name="${entry%%:*}"
  expect="${entry##*:}"
  set +e
  out="$(VerifyBootstrapLocksetV1 "$mut/$name")"
  rc=$?
  set -e
  got="${out%% *}"
  if (( rc == 2 )); then infra "fixture-verify/$name/${out#infra }"; fi
  if [[ "$expect" == 'ok' ]]; then
    { (( rc == 0 )) && [[ "$got" == 'ok' ]]; } || \
      fail "gate_insensitive: the nominal lockset must verify, got rc=$rc code=$got"
  else
    { (( rc == 1 )) && [[ "$got" == "$expect" ]]; } || \
      fail "gate_insensitive: mutant '$name' must be refused with $expect, got rc=$rc code=$got"
  fi
done

# ---------------------------------------------------------------------------
# Phase 1b -- prove the child observer refuses a program that exits 0 without
# doing anything. Synthetic children only; the real tree is never touched here.
# ---------------------------------------------------------------------------
cmut="$work/childmut"
mkdir -p "$cmut/run" || infra 'cannot-build-fixture'
probe_child="${children[0]}"

emit_child() { # emit_child <dir> <body...>
  local d="$cmut/$1"; shift
  mkdir -p "$d" || infra 'cannot-build-fixture'
  { printf '#!/usr/bin/env bash\n'; printf '%s\n' "$@"; } > "$d/$probe_child.sh" \
    || infra 'cannot-build-fixture'
}

# A faithful stand-in: refuses an unknown selector with 64 and owns its record.
emit_child honest \
  "case \"\${1:-AC-001}\" in AC-001) ;; *) exit 64 ;; esac" \
  "printf '{\"card\":\"$probe_child\",\"scenario\":\"AC-001\",\"result\":\"pass\"}\\n'"
# The forgery this phase exists for: the whole body is `exit 0`.
emit_child stub "exit 0"
# The subtler forgery: it prints a perfectly shaped record it does not earn and
# answers 0 to every selector. Only the probe can tell this one apart.
emit_child promiscuous \
  "printf '{\"card\":\"$probe_child\",\"result\":\"pass\"}\\n'"
# Exits 0, refuses the selector correctly, but prints nothing at all.
emit_child silent \
  "case \"\${1:-AC-001}\" in AC-001) ;; *) exit 64 ;; esac" \
  "exit 0"
# Exits 0 printing a record that belongs to another card.
emit_child foreign \
  "case \"\${1:-AC-001}\" in AC-001) ;; *) exit 64 ;; esac" \
  "printf '{\"card\":\"AUR-000\",\"result\":\"pass\"}\\n'"
# Exits 0 owning the record but never claiming a pass result.
emit_child unresolved \
  "case \"\${1:-AC-001}\" in AC-001) ;; *) exit 64 ;; esac" \
  "printf '{\"card\":\"$probe_child\",\"result\":\"skip\"}\\n'"
# An honest RED child.
emit_child red \
  "case \"\${1:-AC-001}\" in AC-001) ;; *) exit 64 ;; esac" \
  "exit 1"
# An honest INCONCLUSIVE child: must never be rendered as behavioral RED.
emit_child inconclusive \
  "case \"\${1:-AC-001}\" in AC-001) ;; *) exit 64 ;; esac" \
  "exit 69"

child_matrix='honest:0:ok'
child_matrix+=' stub:1:child_not_pass'
child_matrix+=' promiscuous:1:child_not_pass'
child_matrix+=' silent:1:child_not_pass'
child_matrix+=' foreign:1:child_not_pass'
child_matrix+=' unresolved:1:child_not_pass'
child_matrix+=' red:1:child_not_pass'
child_matrix+=' inconclusive:2:infra'
child_matrix+=' absent:2:infra'
for entry in $child_matrix; do
  name="${entry%%:*}"
  rest="${entry#*:}"
  want_rc="${rest%%:*}"
  want_code="${rest##*:}"
  set +e
  cout="$(ObserveChildVerdictV1 "$probe_child" "$cmut/$name/$probe_child.sh" "$cmut/run")"
  crc=$?
  set -e
  cgot="${cout%% *}"
  if (( crc != want_rc )) || [[ "$cgot" != "$want_code" ]]; then
    fail "gate_insensitive: child mutant '$name' must yield rc=$want_rc/$want_code, got rc=$crc/$cgot"
  fi
done

# ---------------------------------------------------------------------------
# Phase 2 -- the real lock domain, verified by the same recomputation.
# ---------------------------------------------------------------------------
set +e
domain_out="$(VerifyBootstrapLocksetV1 "$repo_root/.board/bootstrap")"
domain_rc=$?
set -e
domain_code="${domain_out%% *}"
domain_detail="${domain_out#* }"
case "$domain_rc" in
  0) ;;
  2) infra "lock-domain/$domain_detail" ;;
  *) fail "$domain_code: $domain_detail" ;;
esac
lockset_identity="${domain_out#ok }"

# ---------------------------------------------------------------------------
# Phase 3 -- every child verdict is RE-EXECUTED, never read. No pipes: the raw
# exit code of the child program is the only verdict this gate accepts.
# ---------------------------------------------------------------------------
observed=0
mkdir -p "$work/children" || infra 'cannot-build-scratch'
for child in "${children[@]}"; do
  set +e
  child_out="$(ObserveChildVerdictV1 "$child" "$repo_root/tests/acceptance/$child.sh" "$work/children")"
  child_rc=$?
  set -e
  case "$child_rc" in
    0) observed=$(( observed + 1 )) ;;
    2) infra "${child_out#infra }" ;;
    *) fail "${child_out%% *}: ${child_out#* }" ;;
  esac
done
if (( observed != expected_count )); then
  fail "lock_domain_missing: observed $observed passing children, the embedded set requires $expected_count"
fi

printf '{"card":"%s","scenario":"%s","selector":"%s","children":%s,"identity":"%s","verdict_source":"re-execution","result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$expected_count" "$lockset_identity"
