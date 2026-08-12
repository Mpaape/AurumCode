#!/usr/bin/env bash
#
# build-fixture.sh -- deterministic builder for the AurumCode demo Git fixture.
#
#   usage: build-fixture.sh <history-spec> <output-directory>
#
# WHAT IT DOES
#
#   Reads a `history.spec` plus its `content/` overlay and emits a complete,
#   loose-object bare Git repository under `<output-directory>/repo.git`, plus
#   `<output-directory>/manifest.json` describing exactly what it emitted.
#
# WHY IT DOES NOT CALL GIT
#
#   The card's sealed acceptance profile (`bootstrap-readonly-v1`) carries bash
#   and a Go toolchain but no `git` binary, and it runs read-only with no
#   network. A fixture that could only be rebuilt or checked by invoking `git`
#   would therefore be unprovable in the only environment that gates this card.
#   So the object format is written directly: a Git loose object is
#   `zlib(<type> <size>\0<payload>)` stored at `objects/<id[0:2]>/<id[2:]>`,
#   where `<id>` is the SHA-1 of the *uncompressed* bytes.
#
# WHY THE ZLIB STREAMS ARE UNCOMPRESSED
#
#   Determinism is the whole product of this card. Actual DEFLATE output is a
#   property of whichever zlib build produced it, so two correct implementations
#   may legitimately disagree byte for byte. DEFLATE's *stored* block type is
#   the one encoding every implementation must produce identically, so every
#   object here is a zlib stream of stored blocks:
#
#     78 01 | (BFINAL|BTYPE=00) LEN_le16 NLEN_le16 <raw bytes> |* adler32_be32
#
#   `git` inflates these exactly like any other object; they are simply larger.
#   The fixture is a few kilobytes, so that trade is free, and it is what lets a
#   git-less bash program in a sealed container decode and re-verify the tree.
#
# EXIT CODES
#   0  = the fixture was written
#   1  = typed refusal, reported as `git-demo/<code>` on stderr, no output tree
#   3  = harness or environment error, which is never a behavioral result
#   64 = wrong invocation
#
# Every typed refusal happens before the output directory is created, so a
# refused build leaves no effect behind.
set -Eeuo pipefail
export LC_ALL=C
umask 022

readonly tool_list='awk cat cut head mkdir mktemp od printf rm sha1sum sha256sum sort stat tail tr xxd'

die_typed() {
  printf 'git-demo/%s\n' "$1" >&2
  exit 1
}

die_infra() {
  printf 'git-demo/inconclusive/%s\n' "$1" >&2
  exit 3
}

for tool in $tool_list; do
  command -v "$tool" >/dev/null 2>&1 || die_infra "missing_tool:$tool"
done

(( $# == 2 )) || { printf 'usage: build-fixture.sh <history-spec> <output-directory>\n' >&2; exit 64; }

spec_file="$1"
out_dir="$2"

[[ -f "$spec_file" && ! -L "$spec_file" && -r "$spec_file" ]] || die_typed 'spec-unreadable'
[[ -n "$out_dir" && "$out_dir" != /  ]] || die_typed 'output-invalid'
[[ ! -e "$out_dir" ]] || die_typed 'output-exists'

case "$spec_file" in
  */*) spec_dir="${spec_file%/*}" ;;
  *) spec_dir='.' ;;
esac

# ---------------------------------------------------------------------------
# Credential shape gate. The fixture may plant synthetic tokens; it may never
# carry anything shaped like a real one. This mirrors the shape set the OCI
# runner screens materialized inputs with. The expression below is a pattern,
# not a value: none of its own bytes match it.
# ---------------------------------------------------------------------------
readonly credential_pattern='(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})'

# ---------------------------------------------------------------------------
# Spec parsing. Unknown directives, duplicated singletons and malformed values
# are refusals, not defaults.
# ---------------------------------------------------------------------------
schema='' version='' branch='' author='' committer='' timezone='' content_root=''
max_commits='' max_files_per_commit='' max_object_bytes='' max_total_bytes=''

declare -a commit_index=() commit_epoch=() commit_message=()
declare -a commit_ops=()   # newline separated "<verb> <path>" per commit
commit_count=0

set_once() {
  local current="$1" name="$2"
  [[ -z "$current" ]] || die_typed "duplicate-directive:$name"
}

lineno=0
while IFS= read -r line || [[ -n "$line" ]]; do
  lineno=$((lineno + 1))
  (( lineno <= 4096 )) || die_typed 'spec-too-long'
  [[ "$line" != *$'\t'* ]] || die_typed "spec-tab:$lineno"
  case "$line" in
    ''|'#'*) continue ;;
  esac
  [[ "$line" != ' '* && "$line" != *' ' ]] || die_typed "spec-padding:$lineno"
  directive="${line%% *}"
  rest="${line#"$directive"}"
  rest="${rest# }"
  case "$directive" in
    schema) set_once "$schema" schema; schema="$rest" ;;
    version) set_once "$version" version; version="$rest" ;;
    branch) set_once "$branch" branch; branch="$rest" ;;
    author) set_once "$author" author; author="$rest" ;;
    committer) set_once "$committer" committer; committer="$rest" ;;
    timezone) set_once "$timezone" timezone; timezone="$rest" ;;
    content-root) set_once "$content_root" content-root; content_root="$rest" ;;
    limit)
      limit_name="${rest%% *}"
      limit_value="${rest#"$limit_name"}"
      limit_value="${limit_value# }"
      [[ "$limit_value" =~ ^[1-9][0-9]{0,9}$ ]] || die_typed "limit-value:$limit_name"
      case "$limit_name" in
        max-commits) set_once "$max_commits" max-commits; max_commits="$limit_value" ;;
        max-files-per-commit) set_once "$max_files_per_commit" max-files-per-commit; max_files_per_commit="$limit_value" ;;
        max-object-bytes) set_once "$max_object_bytes" max-object-bytes; max_object_bytes="$limit_value" ;;
        max-total-bytes) set_once "$max_total_bytes" max-total-bytes; max_total_bytes="$limit_value" ;;
        *) die_typed "unknown-limit:$limit_name" ;;
      esac
      ;;
    commit)
      idx="${rest%% *}"; rest="${rest#"$idx"}"; rest="${rest# }"
      epoch="${rest%% *}"; message="${rest#"$epoch"}"; message="${message# }"
      [[ "$idx" =~ ^[0-9]{2}$ ]] || die_typed "commit-index:$idx"
      [[ "$epoch" =~ ^[1-9][0-9]{8,9}$ ]] || die_typed "commit-epoch:$idx"
      [[ -n "$message" ]] || die_typed "commit-message:$idx"
      commit_index+=("$idx")
      commit_epoch+=("$epoch")
      commit_message+=("$message")
      commit_ops+=('')
      commit_count=$((commit_count + 1))
      ;;
    add|modify|delete)
      (( commit_count > 0 )) || die_typed "operation-before-commit:$lineno"
      commit_ops[commit_count - 1]+="$directive $rest"$'\n'
      ;;
    *) die_typed "unknown-directive:$directive" ;;
  esac
done <"$spec_file"

[[ "$schema" == 'aurum.git-demo-history' ]] || die_typed 'spec-schema'
[[ "$version" == '1' ]] || die_typed 'spec-version'
[[ "$branch" =~ ^[a-z][a-z0-9-]{0,38}$ ]] || die_typed 'spec-branch'
[[ "$author" =~ ^[A-Za-z][A-Za-z\ .-]{0,60}\<[a-z0-9._-]+@[a-z0-9.-]+\>$ ]] || die_typed 'spec-author'
[[ "$committer" =~ ^[A-Za-z][A-Za-z\ .-]{0,60}\<[a-z0-9._-]+@[a-z0-9.-]+\>$ ]] || die_typed 'spec-committer'
[[ "$timezone" =~ ^[+-][0-9]{4}$ ]] || die_typed 'spec-timezone'
[[ "$content_root" =~ ^[a-z][a-z0-9-]{0,30}$ ]] || die_typed 'spec-content-root'
for limit_name in max_commits max_files_per_commit max_object_bytes max_total_bytes; do
  [[ -n "${!limit_name}" ]] || die_typed "missing-limit:$limit_name"
done
(( max_object_bytes <= max_total_bytes )) || die_typed 'limit-order'

(( commit_count >= 1 )) || die_typed 'no-commits'
(( commit_count <= max_commits )) || die_typed 'too-many-commits'

# ---------------------------------------------------------------------------
# Semantic validation. The full accumulated tree is replayed here so that every
# refusal is raised before a single byte is written.
# ---------------------------------------------------------------------------

# parse_operation <commit index> <operation line> -> verb, version, path.
# `add` and `modify` name their content version explicitly, so a content file
# belongs to the change that introduces it rather than to the position of the
# commit that carries it. That is what makes two independent commit bodies
# swappable without touching the overlay.
verb='' version='' path=''
parse_operation() {
  local idx="$1" op="$2" rest
  verb="${op%% *}"
  rest="${op#"$verb"}"; rest="${rest# }"
  case "$verb" in
    add|modify)
      [[ "$rest" == *' '* ]] || die_typed "operation-shape:$idx:$op"
      version="${rest%% *}"
      path="${rest#"$version"}"; path="${path# }"
      [[ "$version" =~ ^[0-9]{2}$ ]] || die_typed "content-version:$idx:$version"
      ;;
    delete)
      version='-'
      path="$rest"
      ;;
    *) die_typed "unknown-operation:$verb" ;;
  esac
  [[ -n "$path" ]] || die_typed "operation-shape:$idx:$op"
}

declare -A live_path=()
declare -A source_of=()      # "<commit-index>/<path>" -> content file
total_bytes=0
previous_epoch=0
previous_index=0

for (( c = 0; c < commit_count; c++ )); do
  idx="${commit_index[c]}"
  epoch="${commit_epoch[c]}"
  message="${commit_message[c]}"
  (( 10#$idx == previous_index + 1 )) || die_typed "commit-order:$idx"
  previous_index=$((10#$idx))
  (( epoch > previous_epoch )) || die_typed "commit-epoch-order:$idx"
  previous_epoch="$epoch"
  (( ${#message} >= 8 && ${#message} <= 72 )) || die_typed "commit-message-length:$idx"
  [[ "$message" =~ ^[A-Za-z0-9][A-Za-z0-9\ ,.:_-]*[A-Za-z0-9.]$ ]] || die_typed "commit-message-charset:$idx"

  op_count=0
  while IFS= read -r op; do
    [[ -n "$op" ]] || continue
    op_count=$((op_count + 1))
    parse_operation "$idx" "$op"
    [[ "$path" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$ ]] || die_typed "path-charset:$idx:$path"
    [[ "$path" != *//* && "$path" != */ && "$path" != *..* ]] || die_typed "path-shape:$idx:$path"
    case "$verb" in
      add)
        [[ -z "${live_path[$path]+x}" ]] || die_typed "add-existing:$idx:$path"
        ;;
      modify|delete)
        [[ -n "${live_path[$path]+x}" ]] || die_typed "${verb}-missing:$idx:$path"
        ;;
    esac
    if [[ "$verb" == 'delete' ]]; then
      unset 'live_path[$path]'
      continue
    fi
    src="$spec_dir/$content_root/$version/$path"
    [[ -f "$src" && ! -L "$src" && -r "$src" ]] || die_typed "content-missing:$version:$path"
    size="$(stat -c '%s' -- "$src")" || die_infra 'stat'
    [[ "$size" =~ ^[0-9]+$ ]] || die_infra 'stat'
    (( size >= 1 )) || die_typed "content-empty:$version:$path"
    (( size <= max_object_bytes )) || die_typed "object-too-large:$version:$path"
    total_bytes=$((total_bytes + size))
    (( total_bytes <= max_total_bytes )) || die_typed 'total-too-large'
    if grep -Eq -- "$credential_pattern" "$src"; then
      die_typed "non-synthetic-secret:$version:$path"
    fi
    live_path[$path]=1
    source_of["$version/$path"]="$src"
  done <<<"${commit_ops[c]}"
  (( op_count >= 1 )) || die_typed "empty-commit:$idx"
  (( op_count <= max_files_per_commit )) || die_typed "too-many-files:$idx"
done
(( ${#live_path[@]} >= 1 )) || die_typed 'empty-head-tree'

# ---------------------------------------------------------------------------
# Emission. Nothing above this line has written to the output tree.
# ---------------------------------------------------------------------------
work="$(mktemp -d "${TMPDIR:-/tmp}/git-demo-build.XXXXXX")" || die_infra 'mktemp'
cleanup() { rm -rf -- "$work" 2>/dev/null || true; }
trap cleanup EXIT INT TERM HUP

mkdir -p "$out_dir/repo.git/objects" "$out_dir/repo.git/refs/heads" || die_infra 'mkdir'
objects_dir="$out_dir/repo.git/objects"
object_log="$work/objects.tsv"
: >"$object_log"

hex_of() { printf '%s' "$1" | xxd -p | tr -d '\n'; }

adler32() {
  od -An -v -tu1 -- "$1" |
    awk 'BEGIN { a = 1; b = 0 }
         { for (i = 1; i <= NF; i++) { a = (a + $i) % 65521; b = (b + a) % 65521 } }
         END { printf "%04x%04x", b, a }'
}

# zlib_store <raw-file> <destination>: stored-block zlib stream, as documented
# in the header of this file.
zlib_store() {
  local raw="$1" dest="$2" len off chunk last nlen
  len="$(stat -c '%s' -- "$raw")" || die_infra 'stat'
  printf '7801' | xxd -r -p >"$dest" || die_infra 'write'
  off=0
  while :; do
    chunk=$((len - off))
    last=1
    if (( chunk > 65535 )); then
      chunk=65535
      last=0
    fi
    nlen=$(( (~chunk) & 65535 ))
    printf '%02x%02x%02x%02x%02x' \
      "$last" $(( chunk & 255 )) $(( (chunk >> 8) & 255 )) \
      $(( nlen & 255 )) $(( (nlen >> 8) & 255 )) | xxd -r -p >>"$dest"
    if (( chunk > 0 )); then
      tail -c "+$((off + 1))" -- "$raw" | head -c "$chunk" >>"$dest"
    fi
    off=$((off + chunk))
    if (( last == 1 )); then
      break
    fi
  done
  adler32 "$raw" | xxd -r -p >>"$dest"
}

# store_object <type> <payload-file>: prints the object id.
store_object() {
  local type="$1" payload="$2" size raw id dir dest raw_digest stored_digest
  size="$(stat -c '%s' -- "$payload")" || die_infra 'stat'
  raw="$(mktemp "$work/raw.XXXXXX")" || die_infra 'mktemp'
  { printf '%s %s\000' "$type" "$size"; cat -- "$payload"; } >"$raw" || die_infra 'write'
  id="$(sha1sum <"$raw" | cut -d' ' -f1)" || die_infra 'sha1'
  [[ "$id" =~ ^[0-9a-f]{40}$ ]] || die_infra 'sha1'
  dir="$objects_dir/${id:0:2}"
  dest="$dir/${id:2}"
  if [[ ! -f "$dest" ]]; then
    mkdir -p "$dir" || die_infra 'mkdir'
    zlib_store "$raw" "$dest"
    raw_digest="$(sha256sum <"$raw" | cut -d' ' -f1)" || die_infra 'sha256'
    stored_digest="$(sha256sum <"$dest" | cut -d' ' -f1)" || die_infra 'sha256'
    printf '%s\t%s\t%s\t%s\t%s\n' "$id" "$type" "$(stat -c '%s' -- "$raw")" \
      "$raw_digest" "$stored_digest" >>"$object_log"
  fi
  rm -f -- "$raw"
  printf '%s' "$id"
}

# build_tree <prefix>: prints the tree object id for the accumulated paths that
# live under <prefix> ('' for the root tree).
build_tree() {
  local prefix="$1" entries names name rest kind key path sub_id mode target
  entries="$(mktemp "$work/tree.XXXXXX")" || die_infra 'mktemp'
  : >"$entries"
  for path in "${sorted_paths[@]}"; do
    [[ "$path" == "$prefix"* ]] || continue
    rest="${path#"$prefix"}"
    [[ -n "$rest" ]] || continue
    if [[ "$rest" == */* ]]; then
      name="${rest%%/*}"
      kind='tree'
      key="$name/"
    else
      name="$rest"
      kind='blob'
      key="$name"
    fi
    printf '%s\t%s\t%s\n' "$key" "$kind" "$name" >>"$entries"
  done
  names="$(sort -u <"$entries")" || die_infra 'sort'
  local content_hex=''
  while IFS=$'\t' read -r key kind name; do
    [[ -n "$name" ]] || continue
    if [[ "$kind" == 'tree' ]]; then
      mode='40000'
      sub_id="$(build_tree "$prefix$name/")"
    else
      mode='100644'
      sub_id="${blob_id[$prefix$name]}"
    fi
    [[ "$sub_id" =~ ^[0-9a-f]{40}$ ]] || die_infra 'object-id'
    content_hex+="$(hex_of "$mode $name")00$sub_id"
  done <<<"$names"
  target="$(mktemp "$work/treeobj.XXXXXX")" || die_infra 'mktemp'
  printf '%s' "$content_hex" | xxd -r -p >"$target" || die_infra 'write'
  store_object tree "$target"
  rm -f -- "$target" "$entries"
}

declare -A blob_id=()
declare -a sorted_paths=()
declare -a head_commits=()
declare -a head_trees=()
parent=''
manifest_commits=''

for (( c = 0; c < commit_count; c++ )); do
  idx="${commit_index[c]}"
  epoch="${commit_epoch[c]}"
  message="${commit_message[c]}"
  declare -a touched=()
  while IFS= read -r op; do
    [[ -n "$op" ]] || continue
    parse_operation "$idx" "$op"
    if [[ "$verb" == 'delete' ]]; then
      unset 'blob_id[$path]'
    else
      blob_id[$path]="$(store_object blob "${source_of["$version/$path"]}")"
    fi
    touched+=("$verb:$version:$path")
  done <<<"${commit_ops[c]}"

  mapfile -t sorted_paths < <(printf '%s\n' "${!blob_id[@]}" | sort)
  tree_id="$(build_tree '')"

  commit_payload="$(mktemp "$work/commit.XXXXXX")" || die_infra 'mktemp'
  {
    printf 'tree %s\n' "$tree_id"
    [[ -z "$parent" ]] || printf 'parent %s\n' "$parent"
    printf 'author %s %s %s\n' "$author" "$epoch" "$timezone"
    printf 'committer %s %s %s\n' "$committer" "$epoch" "$timezone"
    printf '\n%s\n' "$message"
  } >"$commit_payload" || die_infra 'write'
  commit_id="$(store_object commit "$commit_payload")"
  rm -f -- "$commit_payload"

  touched_json=''
  for entry in "${touched[@]}"; do
    [[ -z "$touched_json" ]] || touched_json+=','
    touched_json+="\"$entry\""
  done
  if [[ -z "$parent" ]]; then parent_json='none'; else parent_json="sha1:$parent"; fi
  manifest_commits+="$(printf '{"index":%d,"id":"sha1:%s","tree":"sha1:%s","parent":"%s","timestamp":%s,"timezone":"%s","message":"%s","operations":[%s]}' \
    "$((10#$idx))" "$commit_id" "$tree_id" "$parent_json" \
    "$epoch" "$timezone" "$message" "$touched_json")"$'\n'

  head_commits+=("$commit_id")
  head_trees+=("$tree_id")
  parent="$commit_id"
  unset touched
done

head_commit="${head_commits[commit_count - 1]}"

mkdir -p "$out_dir/repo.git/refs/heads" || die_infra 'mkdir'
printf '%s\n' "$head_commit" >"$out_dir/repo.git/refs/heads/$branch" || die_infra 'write'
printf 'ref: refs/heads/%s\n' "$branch" >"$out_dir/repo.git/HEAD" || die_infra 'write'
{
  printf '[core]\n'
  printf '\trepositoryformatversion = 0\n'
  printf '\tfilemode = false\n'
  printf '\tbare = true\n'
} >"$out_dir/repo.git/config" || die_infra 'write'

# fixture_digest: the canonical digest of the emitted repository, and the single
# number a consumer compares across two builds.
tree_listing() {
  local root="$1" rel
  ( cd -- "$root" && find . -type f -print | sed 's|^\./||' | sort ) |
    while IFS= read -r rel; do
      printf '%s\t%s\n' "$rel" "$(sha256sum <"$root/$rel" | cut -d' ' -f1)"
    done
}

fixture_digest="$(tree_listing "$out_dir/repo.git" | sha256sum | cut -d' ' -f1)" || die_infra 'digest'
source_digest="$( { printf 'history.spec\t%s\n' "$(sha256sum <"$spec_file" | cut -d' ' -f1)"
                    tree_listing "$spec_dir/$content_root" | sed "s|^|$content_root/|" ; } |
                  sha256sum | cut -d' ' -f1 )" || die_infra 'digest'

objects_json="$(sort <"$object_log" | awk -F'\t' '
  {
    printf "{\"id\":\"sha1:%s\",\"type\":\"%s\",\"raw_bytes\":%d,\"raw_sha256\":\"sha256:%s\",\"stored_sha256\":\"sha256:%s\"}\n", $1, $2, $3, $4, $5
  }')" || die_infra 'manifest'
object_count="$(wc -l <"$object_log" | tr -d ' ')" || die_infra 'manifest'

secrets_json="$(
  for key in "${!source_of[@]}"; do
    sed -n 's/^DEMO_[A-Z_]*=\(AURUM-FAKE-[A-Z0-9-]*\)$/\1/p' "${source_of[$key]}"
  done | sort -u | awk '{ printf "\"%s\"\n", $0 }'
)" || die_infra 'manifest'

# `json_array` prints one entry per line at a fixed indent, with the trailing
# comma on every line but the last. A line-oriented manifest is what lets a
# git-less bash verifier parse it under a strict regex instead of guessing.
json_array() {
  local name="$1" body="$2" tail_char="$3"
  while [[ "$body" == *$'\n' ]]; do body="${body%$'\n'}"; done
  if [[ -z "$body" ]]; then
    printf '  "%s": [],%s\n' "$name" ''
    return
  fi
  printf '  "%s": [\n' "$name"
  printf '%s\n' "$body" | awk -v total="$(printf '%s\n' "$body" | wc -l)" '
    { printf "    %s%s\n", $0, (NR == total ? "" : ",") }'
  printf '  ]%s\n' "$tail_char"
}

{
  printf '{\n'
  printf '  "schema": "aurum.git-demo-fixture",\n'
  printf '  "version": 1,\n'
  printf '  "branch": "%s",\n' "$branch"
  printf '  "author": "%s",\n' "$author"
  printf '  "committer": "%s",\n' "$committer"
  printf '  "head": "sha1:%s",\n' "$head_commit"
  printf '  "commit_count": %d,\n' "$commit_count"
  printf '  "object_count": %d,\n' "$object_count"
  json_array commits "$manifest_commits" ','
  json_array objects "$objects_json" ','
  json_array synthetic_secrets "$secrets_json" ','
  printf '  "source_digest": "sha256:%s",\n' "$source_digest"
  printf '  "fixture_digest": "sha256:%s"\n' "$fixture_digest"
  printf '}\n'
} >"$out_dir/manifest.json" || die_infra 'write'

printf 'git-demo/recreated head=sha1:%s commits=%d objects=%d fixture_digest=sha256:%s\n' \
  "$head_commit" "$commit_count" "$object_count" "$fixture_digest"
