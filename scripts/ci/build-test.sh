#!/bin/sh
# Build and unit tests of the module, run by CI inside the pinned
# golang:1.27.1-alpine image (see .github/workflows/ci.yml). POSIX sh on
# purpose: that image ships without bash and git. Both are installed here,
# because tests run bash scripts and the scanner engines map added lines
# with `git diff` (without git a scan is inconclusive, never clean).
set -eu
if ! command -v bash >/dev/null 2>&1 || ! command -v git >/dev/null 2>&1; then
  apk add --no-cache bash git >/dev/null
fi
export GOFLAGS=-buildvcs=false
go build ./...
go test ./... -count=1
