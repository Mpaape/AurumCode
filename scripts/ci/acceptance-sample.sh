#!/bin/sh
# One shard of the nightly acceptance sample (see
# .github/workflows/acceptance-sample.yml), run inside the pinned
# golang:1.27.1-alpine image. POSIX sh on purpose: that image ships without
# bash, git and python3, which the acceptances need, so they are installed
# here. The shard comes from AUR589_SHARD (`<n>/6`), set by the workflow.
set -eu
apk add --no-cache bash git python3 >/dev/null
git config --global --add safe.directory /src
go mod download
bash tests/acceptance/AUR-589.sh AC-003
bash tests/acceptance/AUR-589.sh AC-001
