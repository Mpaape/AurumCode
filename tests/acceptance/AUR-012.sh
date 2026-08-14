#!/usr/bin/env bash
#
# Acceptance program for card AUR-012, scenario AC-001.
#
# WHAT THIS PROVES
#
#   An empty checkout receives every demo scenario the scaffold declares, and it
#   receives no private AurumCode data.
#
#   The second half of that sentence is the falsifiable one, so it is not
#   asserted: private material is PLANTED and the bootstrap is required to
#   refuse it before any effect. Six of the sealed vectors plant something a
#   consumer repository must never receive --
#
#     * a file the allowlist does not name (`invalid-unlisted-path`);
#     * an AurumCode-private root named by the allowlist itself, sealed with a
#       correct digest (`invalid-private-path`);
#     * a path that escapes the scaffold tree (`invalid-path-escape`);
#     * a payload carrying an AurumCode-private marker, resealed so its digest
#       matches (`invalid-private-marker`);
#     * a payload carrying a value shaped like a real credential, likewise
#       resealed (`invalid-credential-shape`);
#     * an allowlisted entry replaced by a symlink into the AurumCode tree
#       (`invalid-symlink-denied`);
#
#   -- and each one must produce its own typed refusal with an effect count of
#   zero, measured by counting the files in the target before and after the run.
#   The two resealed vectors are the sharp ones: their manifest digests are
#   correct, so only a program that reads the bytes can refuse them.
#
#   The nominal vector is the control that keeps the screen honest. The demo
#   catalog deliberately contains a scenario about a leaked credential, written
#   as the synthetic placeholder `AURUM-FAKE-DEMO-TOKEN`. A bootstrap that
#   refused everything would fail it.
#
#   Nothing is taken on the bootstrap's word. This program re-derives the
#   allowlist from the sealed manifest, re-applies the path policy, recomputes
#   every digest, and greps the produced checkout for private markers and
#   credential shapes itself.
#
# WHY NO `git` IS INVOKED
#
#   The card's sealed profile is `bootstrap-readonly-v1`. Its pinned image
#   carries bash and a Go toolchain but NO `git` binary, and the worker runs
#   read-only with no network. An "empty checkout" is therefore a working tree
#   with no entries -- what a fresh clone leaves behind -- and the whole claim
#   is provable in the only environment that gates this card.
#
# EXIT CODES are disjoint on purpose:
#   0  = the promised property holds
#   1  = behavioral RED: the scaffold or the bootstrap is absent, or it lets
#        something through, or it refuses something it must accept
#   64 = unknown scenario selector
#   79 = inconclusive environment, which is never valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-012'
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

for tool in awk cat find grep head ln mkdir mktemp printf readlink rm sed sha256sum \
  sort stat timeout tr wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root

readonly scaffold_rel='demo-repo/scaffold'
readonly copier_rel="$scaffold_rel/scaffold.sh"
readonly manifest_rel="$scaffold_rel/allowlist.manifest"
readonly tree_rel="$scaffold_rel/tree"
readonly bootstrap_rel='tests/e2e/consumer/bootstrap/bootstrap.sh'
readonly cases_rel='tests/specs/AUR-012/cases.yaml'
readonly docs_rel='docs/specs/AUR-012.md'

readonly scaffold_dir="$repo_root/$scaffold_rel"
readonly copier_path="$repo_root/$copier_rel"
readonly manifest_path="$repo_root/$manifest_rel"
readonly tree_dir="$repo_root/$tree_rel"
readonly bootstrap_path="$repo_root/$bootstrap_rel"

# The shape gate, mirrored from the OCI runner. The expression is a pattern:
# none of its own bytes match it.
readonly credential_pattern='(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})'
# Re-derived here rather than sourced from the bootstrap: a screen that graded
# itself would pass by agreeing with its own definition of private.
readonly private_marker_pattern='(AURUM-PRIVATE-|AURUM_SECRET_CANARY|CandidateIdentityV1|aurum\.(evidence|acceptance-execution|second-reader-observation)|\.board/(evidence|cards)|\.taskmaster/)'
readonly -a denied_segments=(
  '.board' '.git' '.github' '.taskmaster' '.claude' '.ssh' '.aurum-bootstrap'
  'credentials' 'secrets' 'internal'
)
readonly domain_max_entries=64
readonly domain_max_file_bytes=65536

# ---------------------------------------------------------------------------
# The RED boundary. With the sealed test and vectors in place, an absent
# scaffold or an absent bootstrap must reach here and be reported as a missing
# behavior -- never as a missing tool, loader or engine.
# ---------------------------------------------------------------------------
[[ -f "$repo_root/$cases_rel" && ! -L "$repo_root/$cases_rel" ]] || infra "sealed_vectors_missing:$cases_rel"
for required in "$copier_rel" "$manifest_rel" "$bootstrap_rel" "$docs_rel"; do
  [[ -f "$repo_root/$required" && ! -L "$repo_root/$required" && -r "$repo_root/$required" ]] ||
    fail 'behavior-missing'
done
for required in "$scaffold_rel" "$tree_rel"; do
  [[ -d "$repo_root/$required" && ! -L "$repo_root/$required" ]] || fail 'behavior-missing'
done

if [[ -n "${AURUM_SECRET_CANARY:-}" ]] && grep -Rqs -F -- "$AURUM_SECRET_CANARY" "$scaffold_dir"; then
  fail 'secret-canary-in-scaffold'
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a012.XXXXXX")" || infra mktemp
cleanup() { rm -rf -- "$work" 2>/dev/null || true; }
trap cleanup EXIT INT TERM HUP
mkdir -p "$work/runs" "$work/src" || infra workspace

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
  head -n 1 "$f" | grep -qx 'schema: aurum.demo-consumer-bootstrap-cases' || fail 'cases-schema'
  sed -n '2p' "$f" | grep -qx 'version: 1' || fail 'cases-version'
  grep -qx '  max_vectors: 64' "$f" || fail 'cases-limit-vectors'
  grep -qx '  max_total_bytes: 4194304' "$f" || fail 'cases-limit-bytes'
  grep -Eqx '  deadline_seconds: ([1-9]|[12][0-9]|30)' "$f" || fail 'cases-limit-deadline'
  deadline_seconds="$(sed -n 's/^  deadline_seconds: //p' "$f")"
  [[ "$deadline_seconds" =~ ^[0-9]+$ ]] || fail 'cases-limit-deadline'
  (( deadline_seconds >= 1 && deadline_seconds <= 30 )) || fail 'cases-limit-deadline'
  grep -qx "  max_entries: $domain_max_entries" "$f" || fail 'cases-domain-entries'
  grep -qx "  max_file_bytes: $domain_max_file_bytes" "$f" || fail 'cases-domain-file-bytes'

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
sha256_of() { sha256sum <"$1" | sed 's/ .*//'; }

file_list() {
  ( cd -- "$1" && find . -mindepth 1 \( -type f -o -type l \) -print | sed 's|^\./||' | sort )
}

# A symlink is described by its target, never by the bytes it points at. That
# keeps a measurement from following the very escape a vector is planting, and
# keeps the digest independent of where the repository happens to be checked out.
tree_listing() {
  local root="$1" rel
  file_list "$root" |
    while IFS= read -r rel; do
      if [[ -L "$root/$rel" ]]; then
        printf '%s\tsymlink:%s\n' "$rel" "$(readlink -- "$root/$rel")"
      else
        printf '%s\t%s\n' "$rel" "$(sha256_of "$root/$rel")"
      fi
    done
}

tree_digest() { tree_listing "$1" | sha256sum | sed 's/ .*//'; }

tree_bytes() {
  local root="$1" rel target total=0
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    if [[ -L "$root/$rel" ]]; then
      target="$(readlink -- "$root/$rel")"
      total=$(( total + ${#target} ))
    else
      total=$(( total + $(stat -c '%s' -- "$root/$rel") ))
    fi
  done < <(file_list "$root")
  printf '%s' "$total"
}

count_files() {
  local d="$1"
  if [[ ! -d "$d" ]]; then printf '0'; return 0; fi
  file_list "$d" | wc -l | tr -d ' '
}

# The vector input is the allowlist plus the tree it describes. `scaffold.sh` is
# deliberately excluded: it is the program under test, not the input, so the
# declared mutation of the copier does not silently reappear as an input-digest
# divergence instead of the behavioral detection it is meant to trigger.
scaffold_input_digest() {
  local dir="$1"
  {
    printf 'allowlist.manifest\t%s\n' "$(sha256_of "$dir/allowlist.manifest")"
    tree_listing "$dir/tree" | sed 's|^|tree/|'
  } | sha256sum | sed 's/ .*//'
}

scaffold_input_bytes() {
  local dir="$1"
  printf '%s' "$(( $(stat -c '%s' -- "$dir/allowlist.manifest") + $(tree_bytes "$dir/tree") ))"
}

# ---------------------------------------------------------------------------
# An independent reader of the sealed allowlist. The bootstrap has its own
# parser; this one exists so the two must agree about what the scaffold claims.
# ---------------------------------------------------------------------------
declare -a man_scenario=() man_bytes=() man_digest=() man_path=()
declare -a cat_scenario=() cat_count=()

read_manifest() {
  local f="$manifest_path" declared
  head -n 1 "$f" | grep -qx 'schema: aurum.demo-scaffold-allowlist' || fail 'manifest-schema'
  grep -qx 'version: 1' "$f" || fail 'manifest-version'
  grep -qx 'tree-root: tree' "$f" || fail 'manifest-tree-root'
  grep -qx "limit: max_entries $domain_max_entries" "$f" || fail 'manifest-limit-entries'
  grep -qx "limit: max_file_bytes $domain_max_file_bytes" "$f" || fail 'manifest-limit-file-bytes'

  while IFS=$'\t' read -r a b c d; do
    [[ -n "$d" ]] || continue
    man_scenario+=("$a"); man_bytes+=("$b"); man_digest+=("$c"); man_path+=("$d")
  done < <(sed -n 's/^entry: \([A-Za-z0-9-]\{1,\}\) [0-7]\{4\} \([0-9]\{1,\}\) \([0-9a-f]\{64\}\) \(.*\)$/\1\t\2\t\3\t\4/p' "$f")
  declared="$(grep -c '^entry: ' "$f" || true)"
  (( ${#man_path[@]} == declared )) || fail "manifest-entry-parse:$declared"
  (( ${#man_path[@]} >= 1 )) || fail 'manifest-empty'

  while IFS=$'\t' read -r a b; do
    [[ -n "$a" ]] || continue
    cat_scenario+=("$a"); cat_count+=("$b")
  done < <(sed -n 's/^scenario: \([a-z][a-z0-9-]\{1,40\}\) \([0-9]\{1,\}\)$/\1\t\2/p' "$f")
  declared="$(grep -c '^scenario: ' "$f" || true)"
  (( ${#cat_scenario[@]} == declared )) || fail "manifest-scenario-parse:$declared"
  (( ${#cat_scenario[@]} >= 1 )) || fail 'manifest-no-scenarios'
}
read_manifest

entry_total="${#man_path[@]}"
scenario_total="${#cat_scenario[@]}"

path_policy_violation() {
  # Echoes the reason a path must never be copied, or nothing at all.
  local path="$1" segment denied
  if (( ${#path} > 128 )); then printf 'path-too-long'; return 0; fi
  if [[ ! "$path" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]]; then printf 'path-shape'; return 0; fi
  case "$path" in
    */|*//*|*/../*|../*|*/..) printf 'path-shape'; return 0 ;;
  esac
  while IFS= read -r segment; do
    [[ -n "$segment" ]] || continue
    for denied in "${denied_segments[@]}"; do
      if [[ "$segment" == "$denied" ]]; then printf 'private-root'; return 0; fi
    done
    case "$segment" in
      .env|.env.*) printf 'private-root'; return 0 ;;
    esac
  done < <(printf '%s\n' "$path" | tr '/' '\n')
  return 0
}

# The sealed scaffold must describe itself exactly, and must already satisfy the
# policy the bootstrap enforces. A scaffold that only passes because the checker
# is lenient is caught here, before any vector runs.
declare -A allow_index=()
for (( i = 0; i < entry_total; i++ )); do
  p="${man_path[i]}"
  allow_index[$p]=1
  reason="$(path_policy_violation "$p")"
  [[ -z "$reason" ]] || finding "scaffold-path-policy:$reason:$p"
  if [[ -L "$tree_dir/$p" ]]; then
    finding "scaffold-symlink:$p"
  elif [[ ! -f "$tree_dir/$p" ]]; then
    finding "scaffold-entry-missing:$p"
  else
    [[ "$(stat -c '%s' -- "$tree_dir/$p")" == "${man_bytes[i]}" ]] ||
      finding "scaffold-manifest-divergence:bytes:$p"
    [[ "$(sha256_of "$tree_dir/$p")" == "${man_digest[i]}" ]] ||
      finding "scaffold-manifest-divergence:digest:$p"
    (( $(stat -c '%s' -- "$tree_dir/$p") <= domain_max_file_bytes )) ||
      finding "scaffold-file-too-large:$p"
  fi
done
(( entry_total <= domain_max_entries )) || finding "scaffold-too-many-entries:$entry_total"

while IFS= read -r present; do
  [[ -n "$present" ]] || continue
  [[ -n "${allow_index[$present]:-}" ]] || finding "scaffold-unlisted-path:$present"
done < <(file_list "$tree_dir")

# Every declared scenario owns the entries it claims and is published in the
# catalog the consumer reads, so "all scenarios" is a checked number, not a word.
for (( s = 0; s < scenario_total; s++ )); do
  id="${cat_scenario[s]}"
  observed=0
  for (( i = 0; i < entry_total; i++ )); do
    [[ "${man_scenario[i]}" == "$id" ]] || continue
    observed=$(( observed + 1 ))
    [[ "${man_path[i]}" == "scenarios/$id/"* ]] || finding "scaffold-scenario-path:${man_path[i]}"
  done
  (( observed == cat_count[s] )) || finding "scaffold-scenario-count:$id:$observed"
  marker="$(printf '`%s`' "$id")"
  grep -Fq -- "$marker" "$tree_dir/scenarios/INDEX.md" || finding "scaffold-scenario-uncatalogued:$id"
  [[ -f "$tree_dir/scenarios/$id/scenario.yaml" ]] || finding "scaffold-scenario-descriptor:$id"
done

if grep -REqs -- "$credential_pattern" "$tree_dir"; then
  finding 'credential-shape-in-scaffold'
fi
if grep -REqs -- "$private_marker_pattern" "$tree_dir"; then
  finding 'private-marker-in-scaffold'
fi

observed_input_digest="$(scaffold_input_digest "$scaffold_dir")"

# ---------------------------------------------------------------------------
# Vector construction. Every helper below changes the INPUT; none of them
# changes the program under test.
# ---------------------------------------------------------------------------
copy_scaffold() {
  local dest="$1" rel
  mkdir -p "$dest/tree" || infra copy
  cat -- "$copier_path" >"$dest/scaffold.sh"
  cat -- "$manifest_path" >"$dest/allowlist.manifest"
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    case "$rel" in */*) mkdir -p "$dest/tree/${rel%/*}" || infra copy ;; esac
    cat -- "$tree_dir/$rel" >"$dest/tree/$rel"
  done < <(file_list "$tree_dir")
}

sort_entries() {
  local m="$1" tmp="$1.sorted"
  { grep -v '^entry: ' "$m"; grep '^entry: ' "$m" | sort -k6,6; } >"$tmp"
  cat -- "$tmp" >"$m"
  rm -f -- "$tmp"
}

reseal_entry() {
  local dir="$1" path="$2" bytes digest tmp="$1/allowlist.manifest.resealed"
  bytes="$(stat -c '%s' -- "$dir/tree/$path")"
  digest="$(sha256_of "$dir/tree/$path")"
  awk -v p="$path" -v b="$bytes" -v d="$digest" '
    $1 == "entry:" && $6 == p { printf "entry: %s %s %s %s %s\n", $2, $3, b, d, $6; next }
    { print }
  ' "$dir/allowlist.manifest" >"$tmp"
  cat -- "$tmp" >"$dir/allowlist.manifest"
  rm -f -- "$tmp"
}

append_entry() {
  local dir="$1" sc="$2" path="$3" bytes="$4" digest="$5"
  printf 'entry: %s 0644 %s %s %s\n' "$sc" "$bytes" "$digest" "$path" >>"$dir/allowlist.manifest"
  sort_entries "$dir/allowlist.manifest"
}

drop_entry() {
  local dir="$1" path="$2" tmp="$1/allowlist.manifest.dropped"
  awk -v p="$path" '!($1 == "entry:" && $6 == p)' "$dir/allowlist.manifest" >"$tmp"
  cat -- "$tmp" >"$dir/allowlist.manifest"
  rm -f -- "$tmp"
}

# A synthetic scaffold, used by the boundary vectors so the domain limits are
# exercised without disturbing the sealed demo catalog.
synth_scaffold() {
  local dir="$1" entries="$2" first_bytes="$3" i name p
  mkdir -p "$dir/tree/scenarios/synthetic" || infra synth
  cat -- "$copier_path" >"$dir/scaffold.sh"
  for (( i = 1; i <= entries; i++ )); do
    name="$(printf 'f%04d.txt' "$i")"
    if (( i == 1 && first_bytes > 0 )); then
      head -c "$first_bytes" /dev/zero | tr '\0' 'A' >"$dir/tree/scenarios/synthetic/$name"
    else
      printf 'synthetic bounded payload %04d\n' "$i" >"$dir/tree/scenarios/synthetic/$name"
    fi
  done
  {
    printf 'schema: aurum.demo-scaffold-allowlist\n'
    printf 'version: 1\n'
    printf 'tree-root: tree\n'
    printf 'limit: max_entries %d\n' "$domain_max_entries"
    printf 'limit: max_file_bytes %d\n' "$domain_max_file_bytes"
    printf 'limit: max_total_bytes 1048576\n'
    printf 'limit: max_path_bytes 128\n'
    printf 'scenario: synthetic %d\n' "$entries"
    for (( i = 1; i <= entries; i++ )); do
      name="$(printf 'f%04d.txt' "$i")"
      p="scenarios/synthetic/$name"
      printf 'entry: synthetic 0644 %s %s %s\n' \
        "$(stat -c '%s' -- "$dir/tree/$p")" "$(sha256_of "$dir/tree/$p")" "$p"
    done
  } >"$dir/allowlist.manifest"
}

# ---------------------------------------------------------------------------
# The program under test.
# ---------------------------------------------------------------------------
bootstrap_rc=0; bootstrap_code=''; bootstrap_effects=0
bootstrap_plan_digest=''; bootstrap_reported_digest=''

run_bootstrap() {
  local scaffold="$1" target="$2" tmp="${3:-${TMPDIR:-/tmp}}" dir before after rc
  dir="${target%/*}"
  mkdir -p "$dir" || infra run
  before="$(count_files "$target")"
  set +e
  TMPDIR="$tmp" timeout "${deadline_seconds}s" \
    bash "$bootstrap_path" "$scaffold" "$target" >"$dir/stdout" 2>"$dir/stderr"
  rc=$?
  set -e
  bootstrap_rc="$rc"
  (( rc != 79 )) || infra "bootstrap-inconclusive:$(head -n 1 "$dir/stderr")"
  bootstrap_code="$(head -n 1 "$dir/stdout" | sed -n 's|^consumer-bootstrap/\([a-z-]\{1,\}\).*|\1|p')"
  [[ -n "$bootstrap_code" ]] || bootstrap_code='none'
  bootstrap_reported_digest="$(head -n 1 "$dir/stdout" | sed -n 's|^.*tree_digest=sha256:\([0-9a-f]\{64\}\).*|\1|p')"
  bootstrap_plan_digest="$(awk -F'\t' '$1 == "copied" { print $2 }' "$dir/stdout" | sha256sum | sed 's/ .*//')"
  after="$(count_files "$target")"
  bootstrap_effects=$(( after - before ))
}

# An independent audit of a checkout the bootstrap says it produced. None of it
# reads the bootstrap's own report.
audit_consumer() {
  local target="$1" tag="$2" produced rel reason found=0
  produced="$(file_list "$target")"
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    found=$(( found + 1 ))
    if [[ -z "${allow_index[$rel]:-}" ]]; then
      finding "$tag:unlisted-in-consumer:$rel"
      continue
    fi
    reason="$(path_policy_violation "$rel")"
    [[ -z "$reason" ]] || finding "$tag:policy-in-consumer:$reason:$rel"
    [[ ! -L "$target/$rel" ]] || finding "$tag:symlink-in-consumer:$rel"
  done <<<"$produced"
  (( found == entry_total )) || finding "$tag:consumer-entry-count:$found"

  for (( i = 0; i < entry_total; i++ )); do
    rel="${man_path[i]}"
    if [[ ! -f "$target/$rel" ]]; then
      finding "$tag:scenario-not-delivered:$rel"
      continue
    fi
    [[ "$(sha256_of "$target/$rel")" == "${man_digest[i]}" ]] ||
      finding "$tag:consumer-content-divergence:$rel"
  done

  for (( s = 0; s < scenario_total; s++ )); do
    [[ -d "$target/scenarios/${cat_scenario[s]}" ]] ||
      finding "$tag:scenario-missing:${cat_scenario[s]}"
  done

  ! grep -REqs -- "$credential_pattern" "$target" || finding "$tag:credential-shape-in-consumer"
  ! grep -REqs -- "$private_marker_pattern" "$target" || finding "$tag:private-marker-in-consumer"
  if [[ -n "${AURUM_SECRET_CANARY:-}" ]] && grep -Rqs -F -- "$AURUM_SECRET_CANARY" "$target"; then
    finding "$tag:secret-canary-in-consumer"
  fi
}

# Assembled at run time from two halves so no tracked file carries a
# credential-shaped literal. The value is never printed.
synthetic_shape="AKIA$(printf 'ABCDEFGH'; printf 'IJKLMNOP')"

observations="$work/observations.tsv"
: >"$observations"
run1_digest=''
run2_digest=''
run1_plan=''
run2_plan=''
consumer_target=''

run_vector() {
  local name="$1" kind="$2" dir src target out_exit out_code out_effects out_artifact
  local out_input out_bytes
  dir="$work/runs/$name"
  src="$dir/scaffold"
  target="$dir/target"
  out_artifact='none'
  mkdir -p "$dir" || infra run

  case "$name" in
    nominal-bootstrap)
      out_input="sha256:$(scaffold_input_digest "$scaffold_dir")"
      out_bytes="$(scaffold_input_bytes "$scaffold_dir")"
      run_bootstrap "$scaffold_dir" "$target"
      out_exit="$bootstrap_rc"; out_code="$bootstrap_code"; out_effects="$bootstrap_effects"
      run1_plan="$bootstrap_plan_digest"
      if (( out_exit == 0 )); then
        run1_digest="$(tree_digest "$target")"
        out_artifact="sha256:$run1_digest"
        consumer_target="$target"
        audit_consumer "$target" 'nominal'
        [[ "$bootstrap_reported_digest" == "$run1_digest" ]] || finding 'reported-digest-divergence'
      elif [[ "$out_code" == 'unlisted-entry-copied' ]]; then
        # The declared mutation: the copier put a file in the consumer that the
        # allowlist never named.
        finding 'MUT-001'
      fi
      ;;
    nominal-replay)
      out_input="sha256:$(scaffold_input_digest "$scaffold_dir")"
      out_bytes="$(scaffold_input_bytes "$scaffold_dir")"
      mkdir -p "$dir/tmp" "$target" || infra replay
      run_bootstrap "$scaffold_dir" "$target" "$dir/tmp"
      out_exit="$bootstrap_rc"; out_code="$bootstrap_code"; out_effects="$bootstrap_effects"
      run2_plan="$bootstrap_plan_digest"
      if (( out_exit == 0 )); then
        run2_digest="$(tree_digest "$target")"
        out_artifact="sha256:$run2_digest"
        [[ "$run2_digest" == "$run1_digest" ]] || finding 'replay-digest-divergence'
        [[ "$run2_plan" == "$run1_plan" ]] || finding 'replay-order-divergence'
      elif [[ "$out_code" == 'unlisted-entry-copied' ]]; then
        finding 'MUT-001'
      fi
      ;;
    invalid-*)
      copy_scaffold "$src"
      case "$name" in
        invalid-unlisted-path)
          printf 'unlisted demo note\n' >"$src/tree/scenarios/clean-change/EXTRA.md"
          ;;
        invalid-private-path)
          mkdir -p "$src/tree/internal/security" || infra vector
          printf 'package security // AurumCode internal source\n' \
            >"$src/tree/internal/security/redaction.go"
          append_entry "$src" '-' 'internal/security/redaction.go' \
            "$(stat -c '%s' -- "$src/tree/internal/security/redaction.go")" \
            "$(sha256_of "$src/tree/internal/security/redaction.go")"
          ;;
        invalid-path-escape)
          append_entry "$src" '-' '../../../etc/passwd' \
            "$(stat -c '%s' -- "$src/tree/README.md")" "$(sha256_of "$src/tree/README.md")"
          ;;
        invalid-private-marker)
          printf 'note = "AURUM-PRIVATE-BOARD-NOTE copied out of the board"\n' \
            >"$src/tree/scenarios/secret-leak/src/config.py"
          reseal_entry "$src" 'scenarios/secret-leak/src/config.py'
          ;;
        invalid-credential-shape)
          printf 'key = "%s"\n' "$synthetic_shape" \
            >"$src/tree/scenarios/secret-leak/src/config.py"
          reseal_entry "$src" 'scenarios/secret-leak/src/config.py'
          ;;
        invalid-digest-mismatch)
          # Same length, different bytes: only a digest can tell these apart.
          sed 's/^# AurumCode consumer demo repository$/# AurumCode consumer demo repositorX/' \
            "$src/tree/README.md" >"$src/README.patched"
          cat -- "$src/README.patched" >"$src/tree/README.md"
          rm -f -- "$src/README.patched"
          ;;
        invalid-symlink-denied)
          # A relative escape out of the scaffold and into the board. The target
          # string is what is measured, so this vector's input digest does not
          # depend on where the repository is checked out or on what the file it
          # points at happens to contain.
          rm -f -- "$src/tree/scenarios/docs-drift/docs/api.md"
          ln -s '../../../../../.board/evidence/AUR-012/validated.json' \
            "$src/tree/scenarios/docs-drift/docs/api.md"
          ;;
        invalid-scenario-incomplete)
          rm -f -- "$src/tree/scenarios/docs-drift/docs/api.md"
          drop_entry "$src" 'scenarios/docs-drift/docs/api.md'
          ;;
        invalid-target-not-empty)
          mkdir -p "$target" || infra vector
          printf 'a checkout that is not empty\n' >"$target/PREEXISTING.md"
          ;;
        *)
          finding "unknown-vector:$name"
          return 0
          ;;
      esac
      out_input="sha256:$(scaffold_input_digest "$src")"
      out_bytes="$(scaffold_input_bytes "$src")"
      run_bootstrap "$src" "$target"
      out_exit="$bootstrap_rc"; out_code="$bootstrap_code"; out_effects="$bootstrap_effects"
      if (( out_exit == 0 )); then
        finding "detector-missed:$name"
        out_artifact="sha256:$(tree_digest "$target")"
      fi
      ;;
    boundary-max-entries|boundary-too-many-entries|boundary-max-file-bytes|boundary-file-too-large)
      case "$name" in
        boundary-max-entries) synth_scaffold "$src" "$domain_max_entries" 0 ;;
        boundary-too-many-entries) synth_scaffold "$src" $(( domain_max_entries + 1 )) 0 ;;
        boundary-max-file-bytes) synth_scaffold "$src" 4 "$domain_max_file_bytes" ;;
        boundary-file-too-large) synth_scaffold "$src" 4 $(( domain_max_file_bytes + 1 )) ;;
      esac
      out_input="sha256:$(scaffold_input_digest "$src")"
      out_bytes="$(scaffold_input_bytes "$src")"
      run_bootstrap "$src" "$target"
      out_exit="$bootstrap_rc"; out_code="$bootstrap_code"; out_effects="$bootstrap_effects"
      if (( out_exit == 0 )); then
        out_artifact="sha256:$(tree_digest "$target")"
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

for (( v = 0; v < vector_count; v++ )); do
  run_vector "${vec_name[v]}" "${vec_kind[v]}"
done

observed_rows="$(wc -l <"$observations" | tr -d ' ')"
(( observed_rows == vector_count )) || finding "observation-count:$observed_rows"

# The six vectors that each plant something a consumer repository must never
# receive. Every one of them has to produce its own typed refusal with zero
# effects, or the "no private data" half of the outcome is unproved.
readonly -a planted_names=(
  invalid-unlisted-path invalid-private-path invalid-path-escape
  invalid-private-marker invalid-credential-shape invalid-symlink-denied
)

measured_total=0
planted_vectors=0
planted_detected=0
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
  for planted in "${planted_names[@]}"; do
    [[ "$o_name" == "$planted" ]] || continue
    planted_vectors=$(( planted_vectors + 1 ))
    if [[ "$o_exit" == '1' && "$o_effects" == '0' && "$o_code" == "${vec_code[v]}" ]]; then
      planted_detected=$(( planted_detected + 1 ))
    fi
  done
done
(( measured_total <= 4194304 )) || finding "measured-total-bytes:$measured_total"

# Six plants, six typed refusals, zero effects. Anything less is not a proof
# that private data stays out of the consumer repository.
(( planted_vectors == 6 )) || finding "planted-vector-count:$planted_vectors"
(( planted_detected == planted_vectors )) || finding "planted-undetected:$(( planted_vectors - planted_detected ))"

# The documented sealed values are the ones this run measured, so the document
# cannot drift from the behavior without failing here.
if [[ -n "$run1_digest" ]]; then
  grep -Fq "sha256:$run1_digest" "$repo_root/$docs_rel" || finding 'docs-consumer-digest-missing'
fi
grep -Fq "sha256:$observed_input_digest" "$repo_root/$docs_rel" || finding 'docs-input-digest-missing'
for (( s = 0; s < scenario_total; s++ )); do
  marker="$(printf '`%s`' "${cat_scenario[s]}")"
  grep -Fq -- "$marker" "$repo_root/$docs_rel" || finding "docs-scenario-missing:${cat_scenario[s]}"
done

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

[[ -n "$consumer_target" ]] || fail 'no-consumer-checkout'

sweep_digest="$(sha256sum <"$observations" | sed 's/ .*//')"
vectors_json="$(awk -F'\t' '
  BEGIN { printf "[" }
  {
    if (n++) printf ","
    printf "{\"name\":\"%s\",\"kind\":\"%s\",\"exit_code\":%d,\"code\":\"%s\",\"effects\":%d,\"artifact_digest\":\"%s\",\"input_bytes\":%d}", $1, $2, $3, $4, $5, $6, $8
  }
  END { printf "]" }' "$observations")"
scenarios_json="$(printf '%s\n' "${cat_scenario[@]}" | awk '
  BEGIN { printf "[" }
  { if (n++) printf ","; printf "\"%s\"", $0 }
  END { printf "]" }')"
if [[ -n "$run1_digest" && "$run1_digest" == "$run2_digest" && "$run1_plan" == "$run2_plan" ]]; then
  replay_identical='true'
else
  replay_identical='false'
fi

printf '{"schema":"aurum.demo-consumer-bootstrap-acceptance","version":1,"card":"%s","scenario":"%s","candidate_identity_v1":{"scaffold_input_sha256":"sha256:%s","copier_sha256":"sha256:%s","bootstrap_sha256":"sha256:%s","vectors_tree_sha256":"sha256:%s"},"consumer_tree_sha256":"sha256:%s","entry_count":%d,"scenario_count":%d,"scenarios":%s,"planted_private_vectors":%d,"planted_private_detected":%d,"sweep_sha256":"sha256:%s","vector_count":%d,"vectors":%s,"replay":{"first_run":"sha256:%s","second_run":"sha256:%s","plan_order":"sha256:%s","identical":%s},"result":"pass"}\n' \
  "$card" "$scenario" "$observed_input_digest" "$(sha256_of "$copier_path")" \
  "$(sha256_of "$bootstrap_path")" "$(tree_digest "$repo_root/tests/specs/AUR-012")" \
  "$run1_digest" "$entry_total" "$scenario_total" "$scenarios_json" \
  "$planted_vectors" "$planted_detected" "$sweep_digest" "$vector_count" "$vectors_json" \
  "$run1_digest" "$run2_digest" "$run1_plan" "$replay_identical"
