FROM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414

# AUR-534: toolchain image for every offline Go profile (go-unit-offline-v1,
# bootstrap-readonly-v1, and the other profiles that only need bash from the
# same pinned base). The board runner always invokes bash and several cards
# execute Go tests from it, so both are baked into the same digest-pinned
# image, mirroring the pattern in bootstrap-readonly-v1/Dockerfile.
RUN apk add --no-cache bash

WORKDIR /opt/aurum-a006
COPY go.mod go.sum ./
RUN GOTOOLCHAIN=local GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org go mod download all
# `go mod download all` appends hashes go mod tidy leaves out (modules of the
# full graph, e.g. kr/pty, x/sync); the baked go.sum must stay byte-identical
# to the repository's for AC-004, so the repository's copy is restored. The
# module cache keeps every downloaded module, so `go list -m all` stays
# offline-green (AUR-590).
COPY go.sum ./

# Self-reference for AC-004: the acceptance test proves the running image was
# built from this exact versioned Dockerfile and the repository's real
# go.mod/go.sum by comparing the baked copies against the worktree's.
COPY .board/oci/images/go-bash-1.27/Dockerfile /opt/aurum-a006/toolchain.Dockerfile

ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOSUMDB=off \
    GONOSUMDB=*
