#!/usr/bin/env bash
# AUR-533: publish a tested analysis-data artifact as an immutable GitHub
# Release tagged analysis-data/<UTC timestamp>. Refuses unless build.sh left
# its "<dist>.tests-passed" marker AND the files on disk still match the
# manifest (nothing changed between the test and the upload). The release is
# created as a draft, filled, then published, so a half-uploaded release is
# never visible to the runtime. An existing tag is never touched.
set -Eeuo pipefail
export LC_ALL=C

dist="${1:-dist}"
marker="$dist.tests-passed"
gh_bin="${AURUM_GH:-gh}"

[[ -f "$marker" ]] || { echo "publish.sh: $marker missing: artifact was not tested, refusing to publish" >&2; exit 1; }
{ read -r digest; read -r tag; } <"$marker"
[[ "$tag" == analysis-data/* ]] || { echo "publish.sh: bad tag in marker" >&2; exit 1; }
manifest_digest="$(sed -n 's/^  "set_digest": "\(sha256:[0-9a-f]*\)".*$/\1/p' "$dist/manifest.json")"
[[ "$manifest_digest" == "$digest" ]] || { echo "publish.sh: manifest changed after the tests" >&2; exit 1; }

assets=("$dist/manifest.json")
while IFS=$'\t' read -r name want; do
  got="sha256:$(sha256sum "$dist/$name" | cut -d' ' -f1)"
  [[ "$got" == "$want" ]] || { echo "publish.sh: $name changed after the tests" >&2; exit 1; }
  assets+=("$dist/$name")
done < <(awk '
  /^  "files": \[/ { in_files = 1; next }
  in_files && /^  \]/ { in_files = 0 }
  in_files && /^      "path": / { gsub(/^      "path": "|",$/, ""); name = $0 }
  in_files && /^      "sha256": / { gsub(/^      "sha256": "|",$/, ""); printf "%s\t%s\n", name, $0 }
' "$dist/manifest.json")
[[ "${#assets[@]}" -gt 1 ]] || { echo "publish.sh: manifest lists no files" >&2; exit 1; }

"$gh_bin" release create "$tag" "${assets[@]}" --draft --latest=false \
  --title "Analysis data ${tag#analysis-data/}" \
  --notes "Automated analysis-data artifact. set_digest: $digest"
"$gh_bin" release edit "$tag" --draft=false --latest=false
echo "published $tag"
