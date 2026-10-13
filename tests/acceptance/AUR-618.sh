#!/usr/bin/env bash
# AUR-618 acceptance: the daily analysis-data copies are published as
# pre-releases and the ones the runtime no longer accepts are deleted, never
# fewer than the newest few, never anything outside analysis-data/. GitHub is
# a fake gh on PATH that logs every call; no network.
#
# Selectors:
#   all      AC-001, AC-002, AC-003, then MUT-001
#   AC-001   publish.sh creates and publishes the release as a pre-release
#            (static assertion on the script and the workflow, then a run
#            against the fake gh); the workflow deletes old copies right
#            after publishing, with the retention inputs
#   AC-002   retain.sh deletes only analysis-data/* copies published before
#            the window, keeps at least the newest N, never touches the
#            product's tags, honours --days/--keep/--dry-run, refuses bad
#            arguments without calling gh
#   AC-003   docs/configuration.md explains the Releases list
#   MUT-001  a copy of retain.sh without the prefix filter deletes a product
#            tag, so the AC-002 scenario must go RED
# Unknown selectors exit 64; infrastructure failures 79; behavior failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-618'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

repo_root="$(CDPATH='' cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)" || infra repo_root
readonly publish="$repo_root/scripts/artifacts/publish.sh"
readonly retain="$repo_root/scripts/artifacts/retain.sh"
readonly workflow="$repo_root/.github/workflows/analysis-data.yml"
readonly doc="$repo_root/docs/configuration.md"
for input in "$publish" "$retain" "$workflow" "$doc"; do
  [[ -s "$input" ]] || infra "missing-input:${input#"$repo_root"/}"
done
command -v sha256sum >/dev/null 2>&1 || infra missing_sha256sum

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a618.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

# "Now" for every retention scenario: 2026-10-12T12:00:00Z.
readonly now_epoch=1791806400

# The fake gh logs each call to gh.log and answers `release list` with the
# fixture in releases.tsv ("<tag><TAB><publishedAt>", deliberately unsorted).
readonly gh_log="$run_dir/gh.log"
readonly fixture="$run_dir/releases.tsv"
mkdir -p "$run_dir/bin"
cat >"$run_dir/bin/gh" <<FAKE
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$gh_log"
if [[ "\$1 \$2" == "release list" ]]; then cat "$fixture"; fi
FAKE
chmod +x "$run_dir/bin/gh"
export PATH="$run_dir/bin:$PATH"

# release_fixture: the product's releases and six copies around the window.
release_fixture() {
  printf '%s\t%s\n' \
    'analysis-data/20261004T100000Z' '2026-10-04T10:05:00Z' \
    'v1.1.0' '2026-09-03T15:28:02Z' \
    'analysis-data/20261012T110000Z' '2026-10-12T11:05:00Z' \
    'beta' '2025-11-04T03:22:19Z' \
    'analysis-data/20261010T100000Z' '2026-10-10T10:05:00Z' \
    'analysis-data/20261001T100000Z' '2026-10-01T10:05:00Z' \
    'v1.0.1' '2026-08-14T05:34:58Z' \
    'analysis-data/20261011T100000Z' '2026-10-11T10:05:00Z' \
    'analysis-data/20261003T100000Z' '2026-10-03T10:05:00Z' \
    >"$fixture"
}

# deleted_tags: the tags the fake gh was asked to delete, one per line.
deleted_tags() { sed -n 's/^release delete \(.*\) --yes --cleanup-tag$/\1/p' "$gh_log" 2>/dev/null || true; }

# run_retain SCRIPT ARGS...: retain.sh (or a mutated copy) against the fixture.
run_retain() {
  local script="$1"; shift
  : >"$gh_log"
  AURUM_NOW_EPOCH="$now_epoch" bash "$script" "$@" >"$run_dir/retain.out" 2>&1
}

# expect_deleted ID TAG...: exactly these tags were deleted, in this order.
expect_deleted() {
  local id="$1"; shift
  local want got
  want="$(printf '%s\n' "$@")"
  got="$(deleted_tags)"
  [[ "$got" == "$want" ]] || fail "$id/deleted-set-differs:got[$(tr '\n' ' ' <<<"$got")]want[$(tr '\n' ' ' <<<"$want")]"
  if grep -vq '^analysis-data/' <<<"$got"; then fail "$id/product-tag-deleted"; fi
}

ac001() {
  # Static: both gh calls of publish.sh mark the release as a pre-release.
  grep -Eq '^"\$gh_bin" release create .* --draft --prerelease ' "$publish" || fail 'AC-001/create-not-prerelease'
  grep -Eq '^"\$gh_bin" release edit .* --draft=false --prerelease ' "$publish" || fail 'AC-001/edit-not-prerelease'
  # Static: the publish job runs publish.sh, then retain.sh with the inputs.
  local job pub ret
  job="$(grep -n '^  publish:' "$workflow" | cut -d: -f1)"
  pub="$(grep -n 'bash scripts/artifacts/publish.sh' "$workflow" | cut -d: -f1)"
  ret="$(grep -n 'bash scripts/artifacts/retain.sh --days "\$RETENTION_DAYS" --keep "\$KEEP_AT_LEAST"' "$workflow" | cut -d: -f1)"
  [[ -n "$job" && -n "$pub" && -n "$ret" ]] || fail 'AC-001/workflow-steps-missing'
  (( job < pub && pub < ret )) || fail 'AC-001/retention-not-after-publish'
  grep -q "RETENTION_DAYS: \${{ inputs.retention_days || '7' }}" "$workflow" || fail 'AC-001/retention-days-input'
  grep -q "KEEP_AT_LEAST: \${{ inputs.keep_at_least || '3' }}" "$workflow" || fail 'AC-001/keep-at-least-input'
  grep -q "^      retention_days:" "$workflow" || fail 'AC-001/dispatch-input-days'
  grep -q "^      keep_at_least:" "$workflow" || fail 'AC-001/dispatch-input-keep'

  # Dynamic: a tested dist published through the fake gh.
  local dist="$run_dir/dist" file_digest set_digest tag='analysis-data/20261012T120000Z'
  mkdir -p "$dist"
  printf 'osv payload\n' >"$dist/osv-Alpha.zip"
  file_digest="sha256:$(sha256sum "$dist/osv-Alpha.zip" | cut -d' ' -f1)"
  set_digest="sha256:$(printf 'osv-Alpha.zip\0%s\n' "$file_digest" | sha256sum | cut -d' ' -f1)"
  cat >"$dist/manifest.json" <<MANIFEST
{
  "schema": "aurum.analysis-data/1",
  "generated_at": "2026-10-12T12:00:00Z",
  "sources": [],
  "files": [
    {
      "path": "osv-Alpha.zip",
      "sha256": "$file_digest",
      "size": 12,
      "kind": "osv"
    }
  ],
  "scanners": {},
  "set_digest": "$set_digest"
}
MANIFEST
  printf '%s\n%s\n' "$set_digest" "$tag" >"$dist.tests-passed"
  : >"$gh_log"
  (cd "$run_dir" && bash "$publish" dist) >"$run_dir/publish.out" 2>&1 || fail "AC-001/publish-failed:$(tail -1 "$run_dir/publish.out")"
  grep -q "^release create $tag .* --draft --prerelease --latest=false " "$gh_log" || fail 'AC-001/run-create-not-prerelease'
  grep -q "^release edit $tag --draft=false --prerelease --latest=false$" "$gh_log" || fail 'AC-001/run-edit-not-prerelease'
}

# ac002 SCRIPT: the retention scenarios against SCRIPT (retain.sh or a copy).
ac002() {
  local script="$1"
  release_fixture

  # Defaults (7 days, keep 3): three copies are older than the window and
  # beyond the newest three; the product's tags are never candidates.
  run_retain "$script" || fail "AC-002/defaults-exit:$(tail -1 "$run_dir/retain.out")"
  expect_deleted AC-002/defaults 'analysis-data/20261004T100000Z' 'analysis-data/20261003T100000Z' 'analysis-data/20261001T100000Z'
  grep -q '^retention: 3 of 6 analysis-data/\* copies kept (window 7d, at least 3 kept)$' "$run_dir/retain.out" || fail 'AC-002/defaults-summary'
  grep -q '^release list --limit 1000 --json tagName,publishedAt,isDraft --jq ' "$gh_log" || fail 'AC-002/list-call'

  # Window of 1 day, keep 1: only the copy from two days ago goes; the one
  # published exactly one day ago is not older than the window.
  printf '%s\t%s\n' \
    'analysis-data/20261010T120000Z' '2026-10-10T12:00:00Z' \
    'analysis-data/20261012T120000Z' '2026-10-12T12:00:00Z' \
    'analysis-data/20261011T120000Z' '2026-10-11T12:00:00Z' \
    'v1.1.0' '2026-09-03T15:28:02Z' \
    >"$fixture"
  run_retain "$script" --days 1 --keep 1 || fail 'AC-002/parameters-exit'
  expect_deleted AC-002/parameters 'analysis-data/20261010T120000Z'

  # All copies old: the newest three stay anyway.
  printf '%s\t%s\n' \
    'analysis-data/20260901T100000Z' '2026-09-01T10:05:00Z' \
    'analysis-data/20260904T100000Z' '2026-09-04T10:05:00Z' \
    'analysis-data/20260902T100000Z' '2026-09-02T10:05:00Z' \
    'analysis-data/20260903T100000Z' '2026-09-03T10:05:00Z' \
    >"$fixture"
  run_retain "$script" || fail 'AC-002/floor-exit'
  expect_deleted AC-002/floor 'analysis-data/20260901T100000Z'

  # Fewer copies than the floor, all old: nothing is deleted.
  printf '%s\t%s\n' \
    'analysis-data/20260901T100000Z' '2026-09-01T10:05:00Z' \
    'analysis-data/20260902T100000Z' '2026-09-02T10:05:00Z' \
    'v1.0.1' '2026-08-14T05:34:58Z' \
    >"$fixture"
  run_retain "$script" || fail 'AC-002/below-floor-exit'
  [[ -z "$(deleted_tags)" ]] || fail 'AC-002/below-floor-deleted'

  # Dry run lists and deletes nothing.
  release_fixture
  run_retain "$script" --dry-run || fail 'AC-002/dry-run-exit'
  [[ -z "$(deleted_tags)" ]] || fail 'AC-002/dry-run-deleted'
  [[ "$(grep -c '^would delete analysis-data/' "$run_dir/retain.out")" == 3 ]] || fail 'AC-002/dry-run-listing'

  # Bad arguments: usage error, gh never called.
  : >"$gh_log"
  if AURUM_NOW_EPOCH="$now_epoch" bash "$script" --days x >/dev/null 2>&1; then fail 'AC-002/bad-days-accepted'; fi
  if AURUM_NOW_EPOCH="$now_epoch" bash "$script" --keep >/dev/null 2>&1; then fail 'AC-002/missing-keep-accepted'; fi
  [[ ! -s "$gh_log" ]] || fail 'AC-002/gh-called-on-bad-arguments'
}

ac003() {
  local text
  text="$(tr '\n' ' ' <"$doc" | sed -e 's/`//g' -e 's/[[:space:]][[:space:]]*/ /g')"
  local phrase
  for phrase in \
    '### O que a lista de Releases mostra' \
    'as versões do produto (vX.Y.Z, a mais nova marcada como Latest)' \
    'Cada cópia é publicada como pre-release' \
    'apaga as cópias publicadas há mais de 7 dias (release e tag), mantendo sempre as 3 mais novas' \
    'retention_days' 'keep_at_least' 'scripts/artifacts/retain.sh' '--dry-run' \
    'Nada fora do prefixo analysis-data/ é candidato'; do
    grep -qF -- "$phrase" <<<"$text" || fail "AC-003/ausente:$phrase"
  done
}

# MUT-001: the single prefix filter of retain.sh (anchored by its comment)
# is removed from a copy; the AC-002 scenario must then delete a product
# tag, which ac002 reports as RED.
mut001() {
  local copy="$run_dir/retain-mut.sh" anchor='# AUR-618 AC-002: only analysis-data copies are candidates'
  cp "$retain" "$copy"
  [[ "$(grep -Fc "$anchor" "$copy")" == 1 ]] || infra mutation-anchor-not-unique
  sed -i "/$anchor/s/.*/  : # MUT-001: prefix filter removed/" "$copy"
  grep -Fq 'MUT-001: prefix filter removed' "$copy" || infra mutation-not-applied
  bash -n "$copy" || infra mutation-does-not-parse
  if (ac002 "$copy") >"$run_dir/mut.out" 2>&1; then fail 'MUT-001/mutation-survived'; fi
  grep -q 'AC-002/defaults/product-tag-deleted\|AC-002/defaults/deleted-set-differs' "$run_dir/mut.out" || fail "MUT-001/red-for-another-reason:$(tail -1 "$run_dir/mut.out")"
  deleted_tags | grep -q '^v1\.0\.1$' || fail 'MUT-001/product-tag-not-deleted-by-mutant'
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 "$retain" ;;
  AC-003) ac003 ;;
  MUT-001) mut001 ;;
  all) ac001; ac002 "$retain"; ac003; mut001 ;;
esac
printf '%s/%s/ok\n' "$card" "$selector"
