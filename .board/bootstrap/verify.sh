#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

root="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)}"
index="$root/locks.yml"
locks_dir="$root/locks"
children=(AUR-359 AUR-360 AUR-361 AUR-362 AUR-363 AUR-364)
paths=(locks/trust-root.yml locks/go.yml locks/actions.yml locks/scanners.yml locks/parsers.yml locks/docs.yml)

fail() {
  printf 'lockset error: %s\n' "$1" >&2
  exit 1
}

[[ -f "$index" && ! -L "$index" ]] || fail "missing regular index: $index"
[[ -d "$locks_dir" && ! -L "$locks_dir" ]] || fail "missing regular lock directory: $locks_dir"

while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "${line//[[:space:]]/}" || "${line:0:1}" == '#' ]] && continue
  [[ "$line" != *$'\t'* ]] || fail 'tab is not allowed in the index'
  if [[ "$line" =~ (^|[^A-Za-z])(pass|passed|fail|failed|verdict|proved|authenticated|approved)([^A-Za-z]|$) ]]; then
    fail 'verdict claims are not allowed in the index'
  fi
done < "$index"

schema_count=0
identity=''
identity_count=0
declare -a ids=() declared=() bound=()
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "${line//[[:space:]]/}" || "${line:0:1}" == '#' ]] && continue
  case "$line" in
    'schema: '*)
      [[ "$line" == 'schema: bootstrap-lockset-v1' ]] || fail 'invalid schema'
      schema_count=$((schema_count + 1))
      ;;
    'identity: '*)
      identity="${line#identity: }"
      [[ "$identity" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'invalid identity'
      identity_count=$((identity_count + 1))
      ;;
    'child: '*)
      read -r id rel digest extra <<< "${line#child: }"
      [[ -n "${id:-}" && -n "${rel:-}" && -n "${digest:-}" && -z "${extra:-}" ]] || fail 'malformed child row'
      expected=''
      for i in "${!children[@]}"; do
        [[ "${children[i]}" == "$id" ]] && expected="${paths[i]}"
      done
      [[ -n "$expected" && "$rel" == "$expected" ]] || fail "unexpected binding for $id"
      [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "invalid digest for $id"
      for old in "${ids[@]}"; do [[ "$old" != "$id" ]] || fail "duplicate child $id"; done
      ids+=("$id")
      bound+=("$rel")
      declared+=("$digest")
      ;;
    *)
      fail "unrecognized index line: $line"
      ;;
  esac
done < "$index"

(( schema_count == 1 )) || fail 'schema must occur exactly once'
(( identity_count == 1 )) || fail 'identity must occur exactly once'
(( ${#ids[@]} == ${#children[@]} )) || fail 'child count does not match the locked set'
for child in "${children[@]}"; do
  found=0
  for id in "${ids[@]}"; do [[ "$id" == "$child" ]] && found=1; done
  (( found == 1 )) || fail "missing child $child"
done

tmp="$(mktemp)"
trap 'rm -f "$tmp" "$tmp.sorted"' EXIT
for i in "${!ids[@]}"; do
  artifact="$root/${bound[i]}"
  [[ -f "$artifact" && ! -L "$artifact" ]] || fail "missing artifact for ${ids[i]}"
  actual="sha256:$(sha256sum -- "$artifact" | cut -d' ' -f1)"
  [[ "$actual" == "${declared[i]}" ]] || fail "digest mismatch for ${ids[i]}"
  printf '%s %s\n' "${ids[i]}" "$actual" >> "$tmp"
done
sort "$tmp" > "$tmp.sorted"
computed="sha256:$(sha256sum -- "$tmp.sorted" | cut -d' ' -f1)"
[[ "$computed" == "$identity" ]] || fail "identity mismatch: declared $identity computed $computed"
printf 'ok %s\n' "$computed"
