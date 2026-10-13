#!/usr/bin/env bash
# AUR-618: keep only the analysis-data copies the runtime can still use.
# Deletes the GitHub Releases, and their tags, named analysis-data/* that
# were published more than --days ago, always keeping the --keep newest
# copies. Only tags under the analysis-data/ prefix are ever candidates: the
# product's releases (v*) are never listed, so never deleted. Drafts are
# left alone, because a draft is a copy publish.sh has not finished.
#
#   retain.sh [--days N] [--keep N] [--dry-run]
#
# Defaults: --days 7 (the runtime's analysis_data.max_age_days default) and
# --keep 3. --dry-run only lists what would be deleted. AURUM_GH overrides
# the gh binary and AURUM_NOW_EPOCH fixes "now" (both for tests).
set -Eeuo pipefail
export LC_ALL=C

readonly prefix='analysis-data/'
days=7
keep=3
dry_run=0
gh_bin="${AURUM_GH:-gh}"

usage() { echo 'usage: retain.sh [--days N] [--keep N] [--dry-run]' >&2; exit 64; }
is_count() { [[ "$1" =~ ^[0-9]+$ ]]; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --days) [[ $# -ge 2 ]] && is_count "$2" || usage; days="$2"; shift 2 ;;
    --keep) [[ $# -ge 2 ]] && is_count "$2" || usage; keep="$2"; shift 2 ;;
    --dry-run) dry_run=1; shift ;;
    *) usage ;;
  esac
done

now="${AURUM_NOW_EPOCH:-$(date -u +%s)}"
is_count "$now" || { echo 'retain.sh: AURUM_NOW_EPOCH must be a UNIX timestamp' >&2; exit 64; }
cutoff="$(date -u -d "@$((now - days * 86400))" +%Y-%m-%dT%H:%M:%SZ)"

# Published (non-draft) releases as "<tag><TAB><publishedAt>", newest first.
# publishedAt is UTC ISO 8601 of fixed width, so string order is time order.
list_published() {
  "$gh_bin" release list --limit 1000 --json tagName,publishedAt,isDraft \
    --jq '.[] | select(.isDraft | not) | [.tagName, .publishedAt] | @tsv' |
    sort -t "$(printf '\t')" -k2,2r
}

seen=0
deleted=0
while IFS=$'\t' read -r tag published; do
  [[ "$tag" == "$prefix"* ]] || continue  # AUR-618 AC-002: only analysis-data copies are candidates
  seen=$((seen + 1))
  (( seen > keep )) || continue
  if [[ -z "$published" ]]; then
    echo "retain.sh: $tag has no publication date, left alone" >&2
    continue
  fi
  [[ "$published" < "$cutoff" ]] || continue
  if (( dry_run )); then
    echo "would delete $tag (published $published, before $cutoff)"
    continue
  fi
  "$gh_bin" release delete "$tag" --yes --cleanup-tag
  echo "deleted $tag (published $published, before $cutoff)"
  deleted=$((deleted + 1))
done < <(list_published)
echo "retention: $((seen - deleted)) of $seen ${prefix}* copies kept (window ${days}d, at least $keep kept)"
