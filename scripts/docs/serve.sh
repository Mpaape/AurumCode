#!/usr/bin/env bash
# Serves the documentation locally at http://127.0.0.1:8000 (same pinned image).
set -Eeuo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
image="$(head -n1 scripts/docs/image.lock)"
exec docker run --rm -e NO_MKDOCS_2_WARNING=true --user "$(id -u):$(id -g)" -p 127.0.0.1:8000:8000 -v "$PWD:/docs" "$image" serve --dev-addr 0.0.0.0:8000 "$@"
