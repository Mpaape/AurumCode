#!/usr/bin/env bash
# Builds the documentation site into ./site with MkDocs Material, in a
# container pinned by digest (scripts/docs/image.lock). Nothing is installed
# on the host. --strict turns every warning into an error.
set -Eeuo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
image="$(head -n1 scripts/docs/image.lock)"
bash scripts/docs/mermaid.sh
exec docker run --rm -e NO_MKDOCS_2_WARNING=true --user "$(id -u):$(id -g)" -v "$PWD:/docs" "$image" build --strict "$@"
