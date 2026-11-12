#!/usr/bin/env bash
set -euo pipefail
case "${1:-all}" in all) ;; *) exit 64 ;; esac
cd "$(dirname "$0")/../.."
exec bash .board/pipeline.sh
