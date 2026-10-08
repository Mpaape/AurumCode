#!/usr/bin/env bash
# Puts the Mermaid library the site serves (docs/javascripts/mermaid.min.js)
# in place before the build. It is third-party minified code, so it is not
# versioned: it is downloaded at a fixed version and accepted only when its
# sha256 matches, otherwise the build fails. The site then serves it itself,
# with no external request when a page is opened.
set -Eeuo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
version="11.4.1"
want="a43bc1afd446f9c4cc66ac5dd45d02e8d65e26fc5344ec0ef787f88d6ddb6f9e"
dest="$root/docs/javascripts/mermaid.min.js"
if [ -f "$dest" ] && [ "$(sha256sum "$dest" | cut -c1-64)" = "$want" ]; then
  exit 0
fi
mkdir -p "$(dirname "$dest")"
tmp="$(mktemp)"
trap 'rm -f -- "$tmp"' EXIT
curl -fsSL --retry 3 -o "$tmp" "https://cdn.jsdelivr.net/npm/mermaid@${version}/dist/mermaid.min.js"
got="$(sha256sum "$tmp" | cut -c1-64)"
if [ "$got" != "$want" ]; then
  echo "mermaid ${version}: sha256 ${got} difere do esperado ${want}" >&2
  exit 1
fi
mv -- "$tmp" "$dest"
