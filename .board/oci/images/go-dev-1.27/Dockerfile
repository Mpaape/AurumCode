# Shared development container for builders and reviewers (.board/bin/go-shared,
# .board/bin/go-live). Same pinned Go toolchain as the sealed acceptance image
# (go-bash-1.27), plus git: package tests drive real repositories. Build:
#   docker build -t aurum-dev-go:1.27.1-2026-10-02 .board/oci/images/go-dev-1.27
FROM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414
RUN apk add --no-cache bash git
ENV GOTOOLCHAIN=local
