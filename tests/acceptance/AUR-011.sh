#!/usr/bin/env bash
#
# Acceptance program for card AUR-011, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `tests/fixtures/repos/git-demo` is a deterministic synthetic Git history:
#   the same `history.spec` plus the same `content/` overlay always recreate the
#   same commits, byte for byte, and the tree carries synthetic secrets only.
#
#   Two independent programs are executed against each other:
#
#     * the ENCODER, `tests/fixtures/repos/git-demo/build-fixture.sh`, is run
#       twice into two fresh directories under two different temporary roots.
#       Its output must be byte-identical to the checked-in `repo.git`, and
#       identical between the two runs. That is the determinism claim,
#       measured rather than asserted.
#     * the DECODER, implemented in this program and sharing no code with the
#       encoder, reads the checked-in loose objects with no help from `git`:
#       it re-frames each zlib stream, recomputes each object's SHA-1 from the
#       inflated bytes, walks HEAD -> ref -> commit chain -> trees -> blobs,
#       enforces Git's tree entry ordering rule, proves the object store is
#       exactly the reachable set, and compares every blob against the
#       `content/` overlay version the manifest says introduced it.
#
#   A fixture that merely looks right cannot pass both: the encoder proves the
#   bytes are reproducible, the decoder proves the bytes are a real Git history
#   that says exactly what the manifest claims.
#
# WHY NO `git` IS INVOKED
#
#   The card's sealed profile is `bootstrap-readonly-v1`. Its pinned image
#   carries bash and a Go toolchain but NO `git` binary, and the worker runs
#   read-only with no network. Shelling out to `git` here would make the card
#   unprovable in the only environment that gates it, so the Git object format
#   is decoded directly. Host-side confirmation that these very bytes pass
#   `git fsck --strict` and `git clone` is recorded in `docs/specs/AUR-011.md`;
#   that is documentation, never this program's evidence.
#
# EXIT CODES are disjoint on purpose:
#   0  = the promised property holds
#   1  = behavioral RED: the fixture is absent, irreproducible, corrupt, or
#        says something other than what the manifest claims
#   64 = unknown scenario selector
#   79 = inconclusive environment, which is never valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-011'
readonly scenario='AC-001'
readonly rc_red=1
selector="${1:-AC-001}"

case "$selector" in
  AC-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit "$rc_red"
}

infra() {
  printf '%s/%s/inconclusive/%s\n' "$card" "$scenario" "$1" >&2
  exit 79
}

declare -a finding_texts=()
readonly max_reported=48

finding() { finding_texts+=("$1"); }

for tool in awk cat cmp cut find grep head mkdir mktemp od printf rm sed sha1sum \
  sha256sum sort stat tail timeout tr wc xxd; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root

readonly fixture_rel='tests/fixtures/repos/git-demo'
readonly spec_rel="$fixture_rel/history.spec"
readonly builder_rel="$fixture_rel/build-fixture.sh"
readonly manifest_rel="$fixture_rel/manifest.json"
readonly repo_rel="$fixture_rel/repo.git"
readonly content_rel="$fixture_rel/content"
readonly cases_rel='tests/specs/AUR-011/cases.yaml'
readonly docs_rel='docs/specs/AUR-011.md'

readonly fixture_dir="$repo_root/$fixture_rel"
readonly spec_path="$repo_root/$spec_rel"
readonly builder_path="$repo_root/$builder_rel"
readonly manifest_path="$repo_root/$manifest_rel"
readonly repo_dir="$repo_root/$repo_rel"
readonly content_dir="$repo_root/$content_rel"

# The shape gate, mirrored from the OCI runner. The expression is a pattern:
# none of its own bytes match it.
readonly credential_pattern='(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})'

# ---------------------------------------------------------------------------
# The RED boundary. With the sealed test and vectors in place, an absent
# fixture must reach here and be reported as a missing behavior -- never as a
# missing tool, loader or engine.
# ---------------------------------------------------------------------------
[[ -f "$repo_root/$cases_rel" && ! -L "$repo_root/$cases_rel" ]] || infra "sealed_vectors_missing:$cases_rel"
for required in "$spec_rel" "$builder_rel" "$manifest_rel" "$docs_rel"; do
  [[ -f "$repo_root/$required" && ! -L "$repo_root/$required" && -r "$repo_root/$required" ]] ||
    fail 'behavior-missing'
done
for required in "$repo_rel" "$content_rel" "$repo_rel/objects" "$repo_rel/refs/heads"; do
  [[ -d "$repo_root/$required" && ! -L "$repo_root/$required" ]] || fail 'behavior-missing'
done
[[ -f "$repo_dir/HEAD" && -f "$repo_dir/config" ]] || fail 'behavior-missing'

if [[ -n "${AURUM_SECRET_CANARY:-}" ]] && grep -Rqs -F -- "$AURUM_SECRET_CANARY" "$fixture_dir"; then
  fail 'secret-canary-in-fixture'
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a011.XXXXXX")" || infra mktemp
cleanup() { rm -rf -- "$work" 2>/dev/null || true; }
trap cleanup EXIT INT TERM HUP
mkdir -p "$work/objects" "$work/runs" || infra workspace

# ---------------------------------------------------------------------------
# Sealed vectors. The bounds below are the harness contract: at most 64
# vectors, at most 4 MiB of vector input in total, and a deadline of at most
# 30 s. They are enforced here, not merely written down.
# ---------------------------------------------------------------------------
declare -a vec_name=() vec_kind=() vec_input_digest=() vec_exit=() vec_code=()
declare -a vec_effects=() vec_artifact=() vec_bytes=()
deadline_seconds=0
vector_count=0

validate_cases() {
  local f="$repo_root/$cases_rel" bytes count total declared i
  local seen_nominal=0 seen_invalid=0 seen_boundary=0
  bytes="$(wc -c <"$f")" || infra cases_unreadable
  (( bytes > 0 && bytes <= 262144 )) || fail 'cases-file-size'
  head -n 1 "$f" | grep -qx 'schema: aurum.git-demo-fixture-cases' || fail 'cases-schema'
  sed -n '2p' "$f" | grep -qx 'version: 1' || fail 'cases-version'
  grep -qx '  max_vectors: 64' "$f" || fail 'cases-limit-vectors'
  grep -qx '  max_total_bytes: 4194304' "$f" || fail 'cases-limit-bytes'
  grep -Eqx '  deadline_seconds: ([1-9]|[12][0-9]|30)' "$f" || fail 'cases-limit-deadline'
  deadline_seconds="$(sed -n 's/^  deadline_seconds: //p' "$f")"
  [[ "$deadline_seconds" =~ ^[0-9]+$ ]] || fail 'cases-limit-deadline'
  (( deadline_seconds >= 1 && deadline_seconds <= 30 )) || fail 'cases-limit-deadline'

  while IFS=$'\t' read -r n k d e c ef a b; do
    [[ -n "$n" ]] || continue
    vec_name+=("$n"); vec_kind+=("$k"); vec_input_digest+=("$d"); vec_exit+=("$e")
    vec_code+=("$c"); vec_effects+=("$ef"); vec_artifact+=("$a"); vec_bytes+=("$b")
  done < <(awk '
    function flush() {
      if (name != "")
        print name "\t" kind "\t" digest "\t" xexit "\t" code "\t" effects "\t" artifact "\t" bytes
    }
    /^  - name: "/ {
      flush()
      name = $0; sub(/^  - name: "/, "", name); sub(/"$/, "", name)
      kind = ""; digest = ""; xexit = ""; code = ""; effects = ""; artifact = ""; bytes = ""
      next
    }
    /^    kind: / { kind = substr($0, 11); next }
    /^    input_digest: "/ { digest = $0; sub(/^    input_digest: "/, "", digest); sub(/"$/, "", digest); next }
    /^    input_bytes: / { bytes = substr($0, 18); next }
    /^    expected_exit: / { xexit = substr($0, 20); next }
    /^    expected_code: "/ { code = $0; sub(/^    expected_code: "/, "", code); sub(/"$/, "", code); next }
    /^    effects: / { effects = substr($0, 14); next }
    /^    artifact_digest: "/ { artifact = $0; sub(/^    artifact_digest: "/, "", artifact); sub(/"$/, "", artifact); next }
    END { flush() }
  ' "$f")

  count="${#vec_name[@]}"
  (( count >= 3 && count <= 64 )) || fail 'cases-vector-count'
  declared="$(grep -c '^  - name: "' "$f" || true)"
  (( declared == count )) || fail 'cases-vector-parse'
  vector_count="$count"

  total=0
  for (( i = 0; i < count; i++ )); do
    [[ "${vec_name[i]}" =~ ^[a-z][a-z0-9-]{2,48}$ ]] || fail "cases-vector-name:${vec_name[i]}"
    case "${vec_kind[i]}" in
      nominal) seen_nominal=1 ;;
      invalid) seen_invalid=1 ;;
      boundary) seen_boundary=1 ;;
      *) fail "cases-vector-kind:${vec_name[i]}" ;;
    esac
    [[ "${vec_input_digest[i]}" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "cases-vector-input-digest:${vec_name[i]}"
    [[ "${vec_exit[i]}" =~ ^[0-9]+$ ]] || fail "cases-vector-exit:${vec_name[i]}"
    [[ "${vec_code[i]}" =~ ^[a-z][a-z-]{2,40}$ ]] || fail "cases-vector-code:${vec_name[i]}"
    [[ "${vec_effects[i]}" =~ ^[0-9]+$ ]] || fail "cases-vector-effects:${vec_name[i]}"
    [[ "${vec_artifact[i]}" =~ ^(none|sha256:[0-9a-f]{64})$ ]] || fail "cases-vector-artifact:${vec_name[i]}"
    [[ "${vec_bytes[i]}" =~ ^[0-9]+$ ]] || fail "cases-vector-bytes:${vec_name[i]}"
    total=$(( total + vec_bytes[i] ))
  done
  (( total <= 4194304 )) || fail 'cases-total-bytes'
  (( seen_nominal == 1 && seen_invalid == 1 && seen_boundary == 1 )) || fail 'cases-missing-kind'
}
validate_cases

# ---------------------------------------------------------------------------
# Shared primitives.
# ---------------------------------------------------------------------------
sha256_of() { sha256sum <"$1" | cut -d' ' -f1; }

adler32() {
  od -An -v -tu1 -- "$1" |
    awk 'BEGIN { a = 1; b = 0 }
         { for (i = 1; i <= NF; i++) { a = (a + $i) % 65521; b = (b + a) % 65521 } }
         END { printf "%04x%04x", b, a }'
}

tree_listing() {
  local root="$1" rel
  ( cd -- "$root" && find . -type f -print | sed 's|^\./||' | sort ) |
    while IFS= read -r rel; do
      printf '%s\t%s\n' "$rel" "$(sha256_of "$root/$rel")"
    done
}

tree_digest() { tree_listing "$1" | sha256sum | cut -d' ' -f1; }

# hex_upto_nul <hex string>: the even-aligned prefix that precedes the first NUL
# byte. Scanning for the literal pair "00" is NOT the same thing: a byte whose
# low nibble is zero -- '0' (0x30), '@', 'P', 'p', ' ' -- immediately followed by
# the terminator spells "00" starting at an odd offset, which would cut the
# field one nibble short. Every Git header and tree entry ends in exactly such a
# byte often enough that this must be done by index, not by pattern.
hex_upto_nul() {
  local hex="$1" i=0
  while (( i < ${#hex} )); do
    if [[ "${hex:i:2}" == '00' ]]; then
      break
    fi
    i=$(( i + 2 ))
  done
  printf '%s' "${hex:0:i}"
}

tree_bytes() {
  local root="$1" rel total=0
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    total=$(( total + $(stat -c '%s' -- "$root/$rel") ))
  done < <(cd -- "$root" && find . -type f -print | sed 's|^\./||' | sort)
  printf '%s' "$total"
}

source_digest_of() {
  local spec="$1" content="$2"
  {
    printf 'history.spec\t%s\n' "$(sha256_of "$spec")"
    tree_listing "$content" | sed 's|^|content/|'
  } | sha256sum | cut -d' ' -f1
}

source_bytes_of() {
  local spec="$1" content="$2"
  printf '%s' "$(( $(stat -c '%s' -- "$spec") + $(tree_bytes "$content") ))"
}

# ---------------------------------------------------------------------------
# Manifest reader. The manifest is line-oriented on purpose, so every field is
# matched by an exact expression; a manifest that drifts from this shape stops
# parsing and is reported, never silently skipped.
# ---------------------------------------------------------------------------
manifest_head=''; manifest_branch=''; manifest_author=''; manifest_committer=''
manifest_commit_count=''; manifest_object_count=''
manifest_source_digest=''; manifest_fixture_digest=''
declare -a mc_index=() mc_id=() mc_tree=() mc_parent=() mc_ts=() mc_tz=() mc_msg=() mc_ops=()
declare -a mo_id=() mo_type=() mo_raw_bytes=() mo_raw_sha=() mo_stored_sha=()
declare -a manifest_secrets=()

read_manifest() {
  local f="$manifest_path" tab=$'\t' i a b c d e f2 g h x
  head -n 1 "$f" | grep -qx '{' || fail 'manifest-shape'
  grep -qx '  "schema": "aurum.git-demo-fixture",' "$f" || fail 'manifest-schema'
  grep -qx '  "version": 1,' "$f" || fail 'manifest-version'
  manifest_branch="$(sed -n 's/^  "branch": "\([a-z][a-z0-9-]*\)",$/\1/p' "$f")"
  manifest_author="$(sed -n 's/^  "author": "\(.*\)",$/\1/p' "$f")"
  manifest_committer="$(sed -n 's/^  "committer": "\(.*\)",$/\1/p' "$f")"
  manifest_head="$(sed -n 's/^  "head": "sha1:\([0-9a-f]\{40\}\)",$/\1/p' "$f")"
  manifest_commit_count="$(sed -n 's/^  "commit_count": \([0-9][0-9]*\),$/\1/p' "$f")"
  manifest_object_count="$(sed -n 's/^  "object_count": \([0-9][0-9]*\),$/\1/p' "$f")"
  manifest_source_digest="$(sed -n 's/^  "source_digest": "sha256:\([0-9a-f]\{64\}\)",$/\1/p' "$f")"
  manifest_fixture_digest="$(sed -n 's/^  "fixture_digest": "sha256:\([0-9a-f]\{64\}\)"$/\1/p' "$f")"
  [[ "$manifest_branch" =~ ^[a-z][a-z0-9-]*$ ]] || fail 'manifest-branch'
  [[ -n "$manifest_author" && -n "$manifest_committer" ]] || fail 'manifest-identity'
  [[ "$manifest_head" =~ ^[0-9a-f]{40}$ ]] || fail 'manifest-head'
  [[ "$manifest_commit_count" =~ ^[0-9]+$ ]] || fail 'manifest-commit-count'
  [[ "$manifest_object_count" =~ ^[0-9]+$ ]] || fail 'manifest-object-count'
  [[ "$manifest_source_digest" =~ ^[0-9a-f]{64}$ ]] || fail 'manifest-source-digest'
  [[ "$manifest_fixture_digest" =~ ^[0-9a-f]{64}$ ]] || fail 'manifest-fixture-digest'

  while IFS="$tab" read -r a b c d e f2 g h; do
    [[ -n "$a" ]] || continue
    mc_index+=("$a"); mc_id+=("$b"); mc_tree+=("$c"); mc_parent+=("$d")
    mc_ts+=("$e"); mc_tz+=("$f2"); mc_msg+=("$g"); mc_ops+=("$h")
  done < <(sed -n 's/^    {"index":\([0-9][0-9]*\),"id":"sha1:\([0-9a-f]\{40\}\)","tree":"sha1:\([0-9a-f]\{40\}\)","parent":"\([a-z0-9:]*\)","timestamp":\([0-9][0-9]*\),"timezone":"\([+-][0-9]\{4\}\)","message":"\([^"]*\)","operations":\[\(.*\)\]},\{0,1\}$/\1\'"$tab"'\2\'"$tab"'\3\'"$tab"'\4\'"$tab"'\5\'"$tab"'\6\'"$tab"'\7\'"$tab"'\8/p' "$f")
  (( ${#mc_id[@]} == manifest_commit_count )) || fail 'manifest-commit-parse'

  while IFS="$tab" read -r a b c d e; do
    [[ -n "$a" ]] || continue
    mo_id+=("$a"); mo_type+=("$b"); mo_raw_bytes+=("$c"); mo_raw_sha+=("$d"); mo_stored_sha+=("$e")
  done < <(sed -n 's/^    {"id":"sha1:\([0-9a-f]\{40\}\)","type":"\([a-z][a-z]*\)","raw_bytes":\([0-9][0-9]*\),"raw_sha256":"sha256:\([0-9a-f]\{64\}\)","stored_sha256":"sha256:\([0-9a-f]\{64\}\)"},\{0,1\}$/\1\'"$tab"'\2\'"$tab"'\3\'"$tab"'\4\'"$tab"'\5/p' "$f")
  (( ${#mo_id[@]} == manifest_object_count )) || fail 'manifest-object-parse'

  while IFS= read -r x; do
    [[ -n "$x" ]] || continue
    manifest_secrets+=("$x")
  done < <(sed -n 's/^    "\(AURUM-FAKE-[A-Z0-9-]*\)",\{0,1\}$/\1/p' "$f")
  (( ${#manifest_secrets[@]} >= 1 )) || fail 'manifest-secrets'

  for (( i = 0; i < ${#mc_index[@]}; i++ )); do
    (( mc_index[i] == i + 1 )) || fail "manifest-commit-index:${mc_index[i]}"
  done
}
read_manifest

# ---------------------------------------------------------------------------
# DECODER. No `git`, no encoder code: the loose object format is read directly.
#
#   loose object := zlib( "<type> <size>\0<payload>" ) at objects/<id0:2>/<id2:>
#   zlib stream  := 78 01 <stored blocks> adler32_be32
#   stored block := <BFINAL|BTYPE=00> LEN_le16 NLEN_le16 <LEN raw bytes>
#
# Check order is deliberate: framing, then SHA-1 identity, then the Adler-32
# trailer. That keeps the invalid vectors distinguishable instead of collapsing
# every corruption into one code.
#
# No function below reports through a command substitution, because a
# `decode_error` set inside a subshell would be lost and a real divergence
# would then be reported under the wrong code.
# ---------------------------------------------------------------------------
decode_error=''
declare -A obj_type=() obj_payload=() reachable=()

inflate_stored() {
  local src="$1" dest="$2" size hdr off blockhdr bfinal len nlen
  decode_error=''
  size="$(stat -c '%s' -- "$src")" || { decode_error='zlib-unreadable'; return 1; }
  (( size >= 11 )) || { decode_error='zlib-truncated'; return 1; }
  hdr="$(head -c 2 -- "$src" | xxd -p)"
  [[ "$hdr" == '7801' ]] || { decode_error='zlib-header'; return 1; }
  : >"$dest"
  off=2
  while :; do
    (( off + 5 <= size )) || { decode_error='zlib-truncated'; return 1; }
    blockhdr="$(tail -c "+$((off + 1))" -- "$src" | head -c 5 | xxd -p)"
    bfinal="${blockhdr:0:2}"
    [[ "$bfinal" == '00' || "$bfinal" == '01' ]] || { decode_error='zlib-block-type'; return 1; }
    len=$(( 16#${blockhdr:4:2} * 256 + 16#${blockhdr:2:2} ))
    nlen=$(( 16#${blockhdr:8:2} * 256 + 16#${blockhdr:6:2} ))
    (( (len ^ 65535) == nlen )) || { decode_error='zlib-length-complement'; return 1; }
    off=$(( off + 5 ))
    (( off + len <= size )) || { decode_error='zlib-truncated'; return 1; }
    if (( len > 0 )); then
      tail -c "+$((off + 1))" -- "$src" | head -c "$len" >>"$dest"
    fi
    off=$(( off + len ))
    if [[ "$bfinal" == '01' ]]; then
      break
    fi
  done
  (( off + 4 == size )) || { decode_error='zlib-trailing-bytes'; return 1; }
  return 0
}

check_adler() {
  local src="$1" raw="$2" trailer computed
  trailer="$(tail -c 4 -- "$src" | xxd -p)"
  computed="$(adler32 "$raw")"
  [[ "$trailer" == "$computed" ]] || { decode_error='zlib-adler-mismatch'; return 1; }
  return 0
}

decode_repository() {
  local root="$1" scratch="$2" file id dir base raw hexhead prefix header type size
  local -a files=()
  decode_error=''
  unset obj_type obj_payload
  declare -gA obj_type=() obj_payload=()
  rm -rf -- "$scratch" || { decode_error='scratch'; return 1; }
  mkdir -p "$scratch" || { decode_error='scratch'; return 1; }
  mapfile -t files < <(cd -- "$root/objects" && find . -type f -print | sed 's|^\./||' | sort)
  (( ${#files[@]} >= 1 )) || { decode_error='no-objects'; return 1; }
  for file in "${files[@]}"; do
    [[ "$file" =~ ^([0-9a-f]{2})/([0-9a-f]{38})$ ]] || { decode_error='object-path'; return 1; }
    dir="${BASH_REMATCH[1]}"; base="${BASH_REMATCH[2]}"; id="$dir$base"
    raw="$scratch/raw.$id"
    inflate_stored "$root/objects/$file" "$raw" || return 1
    [[ "$(sha1sum <"$raw" | cut -d' ' -f1)" == "$id" ]] || { decode_error='object-id-mismatch'; return 1; }
    check_adler "$root/objects/$file" "$raw" || return 1
    hexhead="$(head -c 32 -- "$raw" | xxd -p | tr -d '\n')"
    prefix="$(hex_upto_nul "$hexhead")"
    (( ${#prefix} >= 2 && ${#prefix} < ${#hexhead} )) || { decode_error='object-header'; return 1; }
    header="$(printf '%s' "$prefix" | xxd -r -p)"
    [[ "$header" =~ ^(blob|tree|commit)\ ([0-9]+)$ ]] || { decode_error='object-header'; return 1; }
    type="${BASH_REMATCH[1]}"; size="${BASH_REMATCH[2]}"
    (( $(stat -c '%s' -- "$raw") == ${#header} + 1 + size )) || { decode_error='object-size'; return 1; }
    tail -c "+$(( ${#header} + 2 ))" -- "$raw" >"$scratch/payload.$id"
    obj_type[$id]="$type"
    obj_payload[$id]="$scratch/payload.$id"
  done
  return 0
}

# parse_tree <tree id> <scratch> <entries file>: writes "<mode>\t<name>\t<id>"
# per entry, and refuses an ordering Git itself would never emit.
parse_tree() {
  local id="$1" scratch="$2" out="$3" hex head_hex rest entry mode name child keys
  hex="$(xxd -p <"${obj_payload[$id]}" | tr -d '\n')"
  keys="$scratch/keys.$id"
  : >"$keys"
  : >"$out"
  while [[ -n "$hex" ]]; do
    head_hex="$(hex_upto_nul "$hex")"
    (( ${#head_hex} >= 2 && ${#head_hex} < ${#hex} )) || { decode_error='tree-entry'; return 1; }
    rest="${hex:${#head_hex} + 2}"
    child="${rest:0:40}"
    [[ "$child" =~ ^[0-9a-f]{40}$ ]] || { decode_error='tree-entry'; return 1; }
    hex="${rest:40}"
    entry="$(printf '%s' "$head_hex" | xxd -r -p)"
    [[ "$entry" =~ ^(100644|40000)\ (.+)$ ]] || { decode_error='tree-mode'; return 1; }
    mode="${BASH_REMATCH[1]}"; name="${BASH_REMATCH[2]}"
    [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || { decode_error='tree-name'; return 1; }
    if [[ "$mode" == '40000' ]]; then
      printf '%s/\n' "$name" >>"$keys"
    else
      printf '%s\n' "$name" >>"$keys"
    fi
    printf '%s\t%s\t%s\n' "$mode" "$name" "$child" >>"$out"
  done
  sort -c -u "$keys" 2>/dev/null || { decode_error='tree-order'; return 1; }
  return 0
}

# walk_tree <tree id> <prefix> <scratch>: appends "<path>\t<blob id>" to stdout
# and marks every tree it crosses reachable.
walk_tree() {
  local id="$1" prefix="$2" scratch="$3" mode name child entries
  [[ "${obj_type[$id]:-}" == 'tree' ]] || { decode_error='tree-missing'; return 1; }
  reachable[$id]=1
  entries="$scratch/entries.$id"
  parse_tree "$id" "$scratch" "$entries" || return 1
  while IFS=$'\t' read -r mode name child; do
    [[ -n "$mode" ]] || continue
    [[ -n "${obj_type[$child]:-}" ]] || { decode_error='dangling-tree-entry'; return 1; }
    if [[ "$mode" == '40000' ]]; then
      walk_tree "$child" "$prefix$name/" "$scratch" || return 1
    else
      [[ "${obj_type[$child]}" == 'blob' ]] || { decode_error='tree-entry-type'; return 1; }
      printf '%s%s\t%s\n' "$prefix" "$name" "$child"
    fi
  done <"$entries"
  return 0
}

# ---------------------------------------------------------------------------
# verify_repository: the full behavioral check of a decoded repository against
# the manifest that claims to describe it. Sets decode_error and returns 1 on
# the first divergence.
# ---------------------------------------------------------------------------
verify_repository() {
  local root="$1" scratch="$2"
  local head_line ref_target ref_file cur i n payload parents id tree
  local parent_field author_line committer_line message paths_file op verb rest version path blob
  local -a chain=() prev_paths=()
  decode_repository "$root" "$scratch" || return 1
  unset reachable
  declare -gA reachable=()

  head_line="$(cat -- "$root/HEAD")"
  [[ "$head_line" == "ref: refs/heads/$manifest_branch" ]] || { decode_error='head-ref'; return 1; }
  ref_file="$root/refs/heads/$manifest_branch"
  [[ -f "$ref_file" ]] || { decode_error='branch-missing'; return 1; }
  ref_target="$(cat -- "$ref_file")"
  [[ "$ref_target" =~ ^[0-9a-f]{40}$ ]] || { decode_error='ref-shape'; return 1; }
  [[ -n "${obj_type[$ref_target]:-}" ]] || { decode_error='dangling-ref'; return 1; }
  [[ "${obj_type[$ref_target]}" == 'commit' ]] || { decode_error='ref-type'; return 1; }
  [[ "$ref_target" == "$manifest_head" ]] || { decode_error='head-divergence'; return 1; }

  # The parent chain from HEAD must be linear and exactly as long as the
  # manifest claims, in exactly the manifest's order.
  cur="$ref_target"
  while :; do
    chain=("$cur" "${chain[@]}")
    (( ${#chain[@]} <= 1024 )) || { decode_error='chain-too-long'; return 1; }
    payload="${obj_payload[$cur]}"
    parents="$(sed -n 's/^parent \([0-9a-f]\{40\}\)$/\1/p' "$payload")"
    if [[ -z "$parents" ]]; then
      break
    fi
    (( $(printf '%s\n' "$parents" | wc -l) == 1 )) || { decode_error='non-linear-history'; return 1; }
    [[ -n "${obj_type[$parents]:-}" ]] || { decode_error='dangling-parent'; return 1; }
    [[ "${obj_type[$parents]}" == 'commit' ]] || { decode_error='parent-type'; return 1; }
    cur="$parents"
  done
  n="${#chain[@]}"
  (( n == manifest_commit_count )) || { decode_error='commit-count-divergence'; return 1; }

  for (( i = 0; i < n; i++ )); do
    id="${chain[i]}"
    [[ "$id" == "${mc_id[i]}" ]] || { decode_error='commit-order-divergence'; return 1; }
    reachable[$id]=1
    payload="${obj_payload[$id]}"
    tree="$(sed -n 's/^tree \([0-9a-f]\{40\}\)$/\1/p' "$payload" | head -n 1)"
    [[ "$tree" == "${mc_tree[i]}" ]] || { decode_error='commit-tree-divergence'; return 1; }
    parent_field="$(sed -n 's/^parent \([0-9a-f]\{40\}\)$/sha1:\1/p' "$payload" | head -n 1)"
    [[ -n "$parent_field" ]] || parent_field='none'
    [[ "$parent_field" == "${mc_parent[i]}" ]] || { decode_error='commit-parent-divergence'; return 1; }
    author_line="$(awk '/^author /{ sub(/^author /, ""); print; exit }' "$payload")"
    committer_line="$(awk '/^committer /{ sub(/^committer /, ""); print; exit }' "$payload")"
    [[ "$author_line" == "$manifest_author ${mc_ts[i]} ${mc_tz[i]}" ]] ||
      { decode_error='commit-author-divergence'; return 1; }
    [[ "$committer_line" == "$manifest_committer ${mc_ts[i]} ${mc_tz[i]}" ]] ||
      { decode_error='commit-committer-divergence'; return 1; }
    message="$(awk 'blank { print; exit } /^$/ { blank = 1 }' "$payload")"
    [[ "$message" == "${mc_msg[i]}" ]] || { decode_error='commit-message-divergence'; return 1; }

    paths_file="$scratch/paths.$i"
    walk_tree "$tree" '' "$scratch" >"$paths_file" || return 1

    while IFS= read -r op; do
      [[ -n "$op" ]] || continue
      verb="${op%%:*}"; rest="${op#*:}"; version="${rest%%:*}"; path="${rest#*:}"
      [[ -n "$verb" && -n "$version" && -n "$path" ]] || { decode_error="operation-shape:$op"; return 1; }
      case "$verb" in
        add|modify)
          [[ "$version" =~ ^[0-9]{2}$ ]] || { decode_error="operation-version:$op"; return 1; }
          blob="$(awk -F'\t' -v p="$path" '$1 == p { print $2 }' "$paths_file")"
          [[ "$blob" =~ ^[0-9a-f]{40}$ ]] || { decode_error="path-missing:$path"; return 1; }
          if ! cmp -s -- "${obj_payload[$blob]}" "$content_dir/$version/$path"; then
            decode_error="content-divergence:$version/$path"
            return 1
          fi
          ;;
        delete)
          if awk -F'\t' -v p="$path" '$1 == p { found = 1 } END { exit found ? 0 : 1 }' "$paths_file"; then
            decode_error="delete-ineffective:$path"
            return 1
          fi
          printf '%s\n' "${prev_paths[@]:-}" | grep -qxF -- "$path" ||
            { decode_error="delete-unfounded:$path"; return 1; }
          ;;
        *) decode_error="unknown-operation:$verb"; return 1 ;;
      esac
    done < <(printf '%s\n' "${mc_ops[i]}" | tr ',' '\n' | sed 's/^"//; s/"$//')

    # Every blob in this commit's tree stays reachable even when the commit did
    # not touch it, so the orphan check below is exact rather than optimistic.
    while IFS=$'\t' read -r path blob; do
      [[ -n "$blob" ]] || continue
      reachable[$blob]=1
    done <"$paths_file"
    mapfile -t prev_paths < <(cut -f1 "$paths_file")
  done

  # The object store must be exactly the reachable set: no orphan left behind,
  # nothing referenced but absent.
  for id in "${!obj_type[@]}"; do
    [[ -n "${reachable[$id]:-}" ]] || { decode_error="orphan-object:$id"; return 1; }
  done
  (( ${#obj_type[@]} == manifest_object_count )) || { decode_error='object-count-divergence'; return 1; }
  return 0
}

# ---------------------------------------------------------------------------
# Behavioral checks on the sealed fixture.
# ---------------------------------------------------------------------------
if ! verify_repository "$repo_dir" "$work/objects"; then
  finding "fixture-decode:$decode_error"
fi

# Only synthetic secrets. Every declared token must actually be planted, and no
# credential shape may appear anywhere in the fixture.
for token in "${manifest_secrets[@]}"; do
  [[ "$token" =~ ^AURUM-FAKE-[A-Z0-9-]+$ ]] || finding "synthetic-secret-shape:$token"
  grep -Rqs -F -- "$token" "$content_dir" || finding "synthetic-secret-absent:$token"
done
if grep -REqs -- "$credential_pattern" "$fixture_dir"; then
  finding 'non-synthetic-secret-in-fixture'
fi

observed_source_digest="$(source_digest_of "$spec_path" "$content_dir")"
[[ "$observed_source_digest" == "$manifest_source_digest" ]] || finding 'source-digest-divergence'
observed_fixture_digest="$(tree_digest "$repo_dir")"
[[ "$observed_fixture_digest" == "$manifest_fixture_digest" ]] || finding 'fixture-digest-divergence'

# The documented example is the sealed fixture itself, so the doc cannot drift
# from the behavior without failing this acceptance.
grep -Fq "sha1:$manifest_head" "$repo_root/$docs_rel" || finding 'docs-head-missing'
grep -Fq "sha256:$manifest_fixture_digest" "$repo_root/$docs_rel" || finding 'docs-digest-missing'

# ---------------------------------------------------------------------------
# Vector sweep.
# ---------------------------------------------------------------------------
builder_rc=0; builder_code=''; builder_effects=0

run_builder() {
  local spec="$1" dest="$2" tmp="${3:-${TMPDIR:-/tmp}}" rc out
  set +e
  out="$(TMPDIR="$tmp" timeout "${deadline_seconds}s" bash "$builder_path" "$spec" "$dest" 2>&1)"
  rc=$?
  set -e
  builder_rc="$rc"
  builder_code="$(printf '%s\n' "$out" | head -n 1 | sed -n 's|^git-demo/\([a-z-]*\).*|\1|p')"
  [[ -n "$builder_code" ]] || builder_code='none'
  if [[ -d "$dest" ]]; then builder_effects=1; else builder_effects=0; fi
}

synth_spec() {
  local dir="$1" commits="$2" bytes="$3" token="$4" i idx
  mkdir -p "$dir" || infra synth
  {
    printf 'schema aurum.git-demo-history\n'
    printf 'version 1\n'
    printf 'branch main\n'
    printf 'author Synthetic Vector Author <vector-author@aurum.invalid>\n'
    printf 'committer Synthetic Vector Bot <vector-bot@aurum.invalid>\n'
    printf 'timezone +0000\n'
    printf 'limit max-commits 16\n'
    printf 'limit max-files-per-commit 16\n'
    printf 'limit max-object-bytes 65536\n'
    printf 'limit max-total-bytes 4194304\n'
    printf 'content-root content\n'
    for (( i = 1; i <= commits; i++ )); do
      idx="$(printf '%02d' "$i")"
      printf 'commit %s %d bounded synthetic vector commit %s\n' "$idx" $(( 1700000000 + i )) "$idx"
      printf 'add %s file%s.txt\n' "$idx" "$idx"
    done
  } >"$dir/history.spec"
  for (( i = 1; i <= commits; i++ )); do
    idx="$(printf '%02d' "$i")"
    mkdir -p "$dir/content/$idx" || infra synth
    if (( i == 1 && bytes > 0 )); then
      head -c "$bytes" /dev/zero | tr '\0' 'A' >"$dir/content/$idx/file$idx.txt"
    else
      printf 'bounded synthetic payload %s\n' "$idx" >"$dir/content/$idx/file$idx.txt"
    fi
  done
  if [[ -n "$token" ]]; then
    printf 'token=%s\n' "$token" >>"$dir/content/01/file01.txt"
  fi
}

# Mutated copies of the sealed repository, for the invalid vectors. Each one
# changes bytes, never the checker.
copy_repo() {
  local dest="$1" rel
  mkdir -p "$dest" || infra copy
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    case "$rel" in */*) mkdir -p "$dest/${rel%/*}" || infra copy ;; esac
    cat -- "$repo_dir/$rel" >"$dest/$rel"
  done < <(cd -- "$repo_dir" && find . -type f -print | sed 's|^\./||' | sort)
}

object_file_for() { printf '%s/objects/%s/%s' "$1" "${2:0:2}" "${2:2}"; }

reframe_stored() {
  local raw="$1" dest="$2" len off chunk last nlen
  len="$(stat -c '%s' -- "$raw")"
  printf '7801' | xxd -r -p >"$dest"
  off=0
  while :; do
    chunk=$(( len - off )); last=1
    if (( chunk > 65535 )); then chunk=65535; last=0; fi
    nlen=$(( (~chunk) & 65535 ))
    printf '%02x%02x%02x%02x%02x' "$last" $(( chunk & 255 )) $(( (chunk >> 8) & 255 )) \
      $(( nlen & 255 )) $(( (nlen >> 8) & 255 )) | xxd -r -p >>"$dest"
    if (( chunk > 0 )); then tail -c "+$((off + 1))" -- "$raw" | head -c "$chunk" >>"$dest"; fi
    off=$(( off + chunk ))
    if (( last == 1 )); then break; fi
  done
  adler32 "$raw" | xxd -r -p >>"$dest"
}

# Corrupt the object's identity while keeping its container valid, so the
# decoder must report the SHA-1 divergence and nothing weaker.
mutate_object_payload() {
  local root="$1" id="$2" file hex mutated
  file="$(object_file_for "$root" "$id")"
  inflate_stored "$file" "$work/mut.raw" || infra mutate
  hex="$(xxd -p <"$work/mut.raw" | tr -d '\n')"
  mutated="${hex:0:${#hex} - 2}$(printf '%02x' $(( 16#${hex: -2} ^ 1 )))"
  printf '%s' "$mutated" | xxd -r -p >"$work/mut.raw2"
  reframe_stored "$work/mut.raw2" "$file"
}

# Corrupt only the Adler-32 trailer: identity still holds, the container does not.
mutate_object_trailer() {
  local root="$1" id="$2" file size trailer
  file="$(object_file_for "$root" "$id")"
  size="$(stat -c '%s' -- "$file")"
  head -c "$(( size - 4 ))" -- "$file" >"$work/mut.body"
  trailer="$(tail -c 4 -- "$file" | xxd -p)"
  cat -- "$work/mut.body" >"$file"
  printf '%08x' $(( 16#$trailer ^ 1 )) | xxd -r -p >>"$file"
}

# 65536 payload bytes cannot fit one DEFLATE stored block, so a correct encoder
# must have emitted at least two and the decoder must read them back.
has_multiblock_object() {
  local root="$1" file size off blockhdr bfinal len blocks
  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    size="$(stat -c '%s' -- "$file")"
    off=2; blocks=0
    while (( off + 5 <= size )); do
      blockhdr="$(tail -c "+$((off + 1))" -- "$file" | head -c 5 | xxd -p)"
      bfinal="${blockhdr:0:2}"
      len=$(( 16#${blockhdr:4:2} * 256 + 16#${blockhdr:2:2} ))
      blocks=$(( blocks + 1 ))
      off=$(( off + 5 + len ))
      if [[ "$bfinal" == '01' ]]; then break; fi
    done
    if (( blocks >= 2 )); then return 0; fi
  done < <(find "$root/objects" -type f -print | sort)
  return 1
}

classify_divergence() {
  local generated="$1" sealed_msgs generated_msgs
  sealed_msgs="$(printf '%s\n' "${mc_msg[@]}")"
  generated_msgs="$(sed -n 's/^.*"message":"\([^"]*\)".*$/\1/p' "$generated")"
  if [[ "$sealed_msgs" != "$generated_msgs" ]] &&
     [[ "$(printf '%s\n' "$sealed_msgs" | sort)" == "$(printf '%s\n' "$generated_msgs" | sort)" ]]; then
    finding 'MUT-001'
    return 0
  fi
  finding 'recreation-divergence:manifest.json'
}

observations="$work/observations.tsv"
: >"$observations"
gen1_digest=''
gen2_digest=''

run_vector() {
  local name="$1" kind="$2" dir out_exit out_code out_effects out_artifact out_input out_bytes
  dir="$work/runs/$name"
  out_artifact='none'
  case "$name" in
    nominal-recreate)
      out_input="sha256:$(source_digest_of "$spec_path" "$content_dir")"
      out_bytes="$(source_bytes_of "$spec_path" "$content_dir")"
      run_builder "$spec_path" "$dir/out"
      out_exit="$builder_rc"; out_code="$builder_code"; out_effects="$builder_effects"
      if (( out_exit == 0 )); then
        gen1_digest="$(tree_digest "$dir/out/repo.git")"
        out_artifact="sha256:$gen1_digest"
        if ! cmp -s -- "$dir/out/manifest.json" "$manifest_path"; then
          classify_divergence "$dir/out/manifest.json"
        elif [[ "$gen1_digest" != "$observed_fixture_digest" ]]; then
          finding 'recreation-divergence:repo.git'
        fi
      fi
      ;;
    nominal-replay)
      out_input="sha256:$(source_digest_of "$spec_path" "$content_dir")"
      out_bytes="$(source_bytes_of "$spec_path" "$content_dir")"
      mkdir -p "$dir/tmp" || infra replay
      run_builder "$spec_path" "$dir/out" "$dir/tmp"
      out_exit="$builder_rc"; out_code="$builder_code"; out_effects="$builder_effects"
      if (( out_exit == 0 )); then
        gen2_digest="$(tree_digest "$dir/out/repo.git")"
        out_artifact="sha256:$gen2_digest"
        [[ "$gen2_digest" == "$gen1_digest" ]] || finding 'replay-digest-divergence'
      fi
      ;;
    invalid-object-id-mismatch|invalid-zlib-adler|invalid-dangling-ref)
      copy_repo "$dir/repo.git"
      case "$name" in
        invalid-object-id-mismatch) mutate_object_payload "$dir/repo.git" "$manifest_head" ;;
        invalid-zlib-adler) mutate_object_trailer "$dir/repo.git" "$manifest_head" ;;
        invalid-dangling-ref)
          printf '%s\n' '0123456789abcdef0123456789abcdef01234567' \
            >"$dir/repo.git/refs/heads/$manifest_branch" ;;
      esac
      out_input="sha256:$(tree_digest "$dir/repo.git")"
      out_bytes="$(tree_bytes "$dir/repo.git")"
      out_exit=0; out_code='none'; out_effects=0
      if verify_repository "$dir/repo.git" "$dir/scratch"; then
        finding "detector-missed:$name"
      else
        out_exit=1; out_code="$decode_error"
      fi
      # The sealed fixture must still verify after the mutated copy was read,
      # so a stateful detector cannot pass by remembering the first answer.
      verify_repository "$repo_dir" "$work/objects" || finding "detector-not-restored:$name"
      ;;
    invalid-non-synthetic-secret)
      # The shape is assembled here, never written down: a literal in a tracked
      # file would be refused by the runner's own input scanner.
      synth_spec "$dir/src" 1 0 "AKIA${synthetic_shape_tail}"
      out_input="sha256:$(source_digest_of "$dir/src/history.spec" "$dir/src/content")"
      out_bytes="$(source_bytes_of "$dir/src/history.spec" "$dir/src/content")"
      run_builder "$dir/src/history.spec" "$dir/out"
      out_exit="$builder_rc"; out_code="$builder_code"; out_effects="$builder_effects"
      ;;
    boundary-max-object-bytes|boundary-object-too-large|boundary-max-commits|boundary-too-many-commits)
      case "$name" in
        boundary-max-object-bytes) synth_spec "$dir/src" 1 65536 '' ;;
        boundary-object-too-large) synth_spec "$dir/src" 1 65537 '' ;;
        boundary-max-commits) synth_spec "$dir/src" 16 0 '' ;;
        boundary-too-many-commits) synth_spec "$dir/src" 17 0 '' ;;
      esac
      out_input="sha256:$(source_digest_of "$dir/src/history.spec" "$dir/src/content")"
      out_bytes="$(source_bytes_of "$dir/src/history.spec" "$dir/src/content")"
      run_builder "$dir/src/history.spec" "$dir/out"
      out_exit="$builder_rc"; out_code="$builder_code"; out_effects="$builder_effects"
      if (( out_exit == 0 )); then
        out_artifact="sha256:$(tree_digest "$dir/out/repo.git")"
        if [[ "$name" == 'boundary-max-object-bytes' ]] && ! has_multiblock_object "$dir/out/repo.git"; then
          finding 'boundary-single-block'
        fi
      fi
      ;;
    *)
      finding "unknown-vector:$name"
      return 0
      ;;
  esac
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$name" "$kind" "$out_exit" "$out_code" "$out_effects" "$out_artifact" "$out_input" "$out_bytes" \
    >>"$observations"
}

# Assembled at run time from two halves so no tracked file carries a
# credential-shaped literal.
synthetic_shape_tail="$(printf 'ABCDEFGH'; printf 'IJKLMNOP')"

for (( v = 0; v < vector_count; v++ )); do
  run_vector "${vec_name[v]}" "${vec_kind[v]}"
done

observed_rows="$(wc -l <"$observations" | tr -d ' ')"
(( observed_rows == vector_count )) || finding "observation-count:$observed_rows"

measured_total=0
for (( v = 0; v < vector_count; v++ )); do
  row="$(sed -n "$((v + 1))p" "$observations")"
  IFS=$'\t' read -r o_name o_kind o_exit o_code o_effects o_artifact o_input o_bytes <<<"$row"
  [[ "$o_name" == "${vec_name[v]}" ]] || { finding "vector-order:${vec_name[v]}"; continue; }
  [[ "$o_kind" == "${vec_kind[v]}" ]] || finding "vector-kind:${vec_name[v]}"
  [[ "$o_exit" == "${vec_exit[v]}" ]] || finding "vector-exit:${vec_name[v]}:want=${vec_exit[v]}:got=$o_exit"
  [[ "$o_code" == "${vec_code[v]}" ]] || finding "vector-code:${vec_name[v]}:want=${vec_code[v]}:got=$o_code"
  [[ "$o_effects" == "${vec_effects[v]}" ]] || finding "vector-effects:${vec_name[v]}:want=${vec_effects[v]}:got=$o_effects"
  [[ "$o_artifact" == "${vec_artifact[v]}" ]] || finding "vector-artifact:${vec_name[v]}:want=${vec_artifact[v]}:got=$o_artifact"
  [[ "$o_input" == "${vec_input_digest[v]}" ]] || finding "vector-input:${vec_name[v]}:want=${vec_input_digest[v]}:got=$o_input"
  [[ "$o_bytes" == "${vec_bytes[v]}" ]] || finding "vector-bytes:${vec_name[v]}:want=${vec_bytes[v]}:got=$o_bytes"
  measured_total=$(( measured_total + o_bytes ))
done
(( measured_total <= 4194304 )) || finding "measured-total-bytes:$measured_total"

if [[ -n "${AURUM_SECRET_CANARY:-}" ]] && grep -Rqs -F -- "$AURUM_SECRET_CANARY" "$work"; then
  fail 'secret-canary-exposed'
fi

if (( ${#finding_texts[@]} > 0 )); then
  printf '%s/%s: %d finding(s) over %d sealed vector(s)\n' \
    "$card" "$scenario" "${#finding_texts[@]}" "$vector_count" >&2
  reported=0
  for text in "${finding_texts[@]}"; do
    if (( reported >= max_reported )); then
      printf '%s/%s: %d further finding(s) suppressed\n' \
        "$card" "$scenario" $(( ${#finding_texts[@]} - max_reported )) >&2
      break
    fi
    printf '%s/%s/%s\n' "$card" "$scenario" "$text" >&2
    reported=$(( reported + 1 ))
  done
  exit "$rc_red"
fi

sweep_digest="$(sha256sum <"$observations" | cut -d' ' -f1)"
vectors_json="$(awk -F'\t' '
  BEGIN { printf "[" }
  {
    if (n++) printf ","
    printf "{\"name\":\"%s\",\"kind\":\"%s\",\"exit_code\":%d,\"code\":\"%s\",\"effects\":%d,\"artifact_digest\":\"%s\",\"input_bytes\":%d}", $1, $2, $3, $4, $5, $6, $8
  }
  END { printf "]" }' "$observations")"
if [[ "$gen1_digest" == "$gen2_digest" && -n "$gen1_digest" ]]; then
  determinism='true'
else
  determinism='false'
fi

printf '{"schema":"aurum.git-demo-acceptance","version":1,"card":"%s","scenario":"%s","candidate_identity_v1":{"fixture_tree_sha256":"sha256:%s","source_sha256":"sha256:%s","vectors_tree_sha256":"sha256:%s"},"head":"sha1:%s","branch":"%s","commit_count":%d,"object_count":%d,"synthetic_secret_count":%d,"sweep_sha256":"sha256:%s","vector_count":%d,"vectors":%s,"determinism":{"first_run":"sha256:%s","second_run":"sha256:%s","identical":%s},"result":"pass"}\n' \
  "$card" "$scenario" "$observed_fixture_digest" "$observed_source_digest" \
  "$(tree_digest "$repo_root/tests/specs/AUR-011")" \
  "$manifest_head" "$manifest_branch" "$manifest_commit_count" "$manifest_object_count" \
  "${#manifest_secrets[@]}" "$sweep_digest" "$vector_count" "$vectors_json" \
  "$gen1_digest" "$gen2_digest" "$determinism"
