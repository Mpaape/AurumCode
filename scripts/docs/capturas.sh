#!/usr/bin/env bash
# Generates the documentation screenshots under docs/assets/capturas/ and the
# manifest capturas.json, with the Playwright image pinned by digest in
# scripts/docs/playwright.lock (the same image the CI browser check uses).
#
#   1. rewrites the "Como fica" block of every docs/tutorials/<t>.md;
#   2. renders, per tutorial case, the terminal, the PR comment and the status
#      checks from demo/tutoriais/<t>/out/<case>.log;
#   3. builds the site (scripts/docs/build.sh, --strict) and captures every
#      capability page, desktop and mobile, served locally;
#   4. writes the manifest: per image, the digest of its input (out/ log or
#      page source), the Playwright image and this command.
#
# Fixed viewports, deviceScaleFactor 1, reducedMotion, animations off and no
# external request. Running it twice over the same inputs gives the same file
# set and the same manifest; PNG bytes may differ between machines, so the
# manifest records input digests, not PNG digests.
# AURUM_CAPTURAS_SO=gate,paginas/index limits the captures (manifest stays full).
# Only docker is needed on the host; npm packages are cached in
# ${XDG_CACHE_HOME:-~/.cache}/aurumcode-docs.
set -Eeuo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
image="$(head -n1 scripts/docs/playwright.lock)"
[[ "$image" =~ @sha256:[0-9a-f]{64}$ ]] || { echo "capturas: scripts/docs/playwright.lock must pin the image by digest" >&2; exit 2; }
cache="${AURUM_DOCS_NPM_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/aurumcode-docs}"
mkdir -p "$cache"

pw() {
  docker run --rm --ipc=host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e AURUM_PLAYWRIGHT_IMAGE="$image" -e AURUM_CAPTURAS_SO="${AURUM_CAPTURAS_SO:-}" \
    -v "$root:/src" -v "$cache:/qa" -w /src "$image" \
    bash -c '[ -d /qa/node_modules/playwright ] || npm install --silent --no-audit --no-fund --prefix /qa playwright@1.58.2 >/dev/null; NODE_PATH=/qa/node_modules exec node "$@"' _ "$@"
}

pw scripts/docs/capturas.cjs secoes
# A full run starts from an empty directory, so a removed case leaves no image.
[[ -n "${AURUM_CAPTURAS_SO:-}" ]] || rm -rf docs/assets/capturas
mkdir -p docs/assets/capturas
pw scripts/docs/capturas.cjs casos
bash scripts/docs/build.sh
pw scripts/docs/capturas.cjs paginas
