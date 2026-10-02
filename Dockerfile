FROM golang:1.21-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /aurumcode ./cmd/aurumcode

FROM alpine:3.20

RUN apk add --no-cache ca-certificates git bash jq python3 py3-pip

# AUR-548: Semgrep, pinned to the exact version
# .board/bootstrap/locks/scanners.yml fixes by digest
# (docker.io/semgrep/semgrep, sast_scanner_version), installed from PyPI so
# `aurumcode review`'s own `quality_gates.sast` pass can exec it from PATH
# inside this same container -- the reusable workflow
# (.github/workflows/review.yml) runs exactly one container today, with no
# separate, docker-in-docker scanner step. pip install itself needs network
# at BUILD time, same as `go mod download` above; `semgrep scan
# --config p/<pack>` additionally needs network at RUN time to fetch a
# named registry pack -- see docs/configuration.md's own SAST section for
# the fully-offline alternative (point rule_packs at local rule files
# instead of a p/... registry name).
#
# Semgrep's official PyPI wheels target glibc (manylinux); Alpine's musl
# libc has historically been the one environment where `pip install
# semgrep` is NOT guaranteed to find a matching wheel. This install was
# authored but NOT verified against an actual `docker build` in this card's
# own time-boxed session -- see docs/specs/AUR-548.md's own "Status"
# section. If this step fails in CI, the documented fallback is switching
# this final stage's base image to a glibc distribution (e.g.
# python:3.11-slim) with the same package set installed via apt.
RUN pip3 install --no-cache-dir --break-system-packages semgrep==1.172.0

WORKDIR /github/workspace
COPY --from=builder /aurumcode /app/aurumcode
COPY scripts/action-entrypoint.sh /app/action-entrypoint.sh
RUN chmod 0755 /app/aurumcode /app/action-entrypoint.sh \
    && bash -n /app/action-entrypoint.sh

ENV AURUMCODE_CLI=/app/aurumcode
ENTRYPOINT ["/app/action-entrypoint.sh"]
