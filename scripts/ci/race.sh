#!/usr/bin/env bash
# Race tests of the module, run by CI inside the pinned golang:1.27.1-bookworm
# image (see .github/workflows/ci.yml), which already has bash, git and a C
# toolchain for -race.
set -euo pipefail
export GOFLAGS=-buildvcs=false CGO_ENABLED=1
# No per-package test timeout: the job's timeout-minutes bounds the run.
go test ./... -race -count=1 -timeout 0
