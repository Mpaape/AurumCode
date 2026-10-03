FROM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /aurumcode ./cmd/aurumcode

# The secrets engine: gitleaks copied from the image the scanners lock pins
# by digest (.board/bootstrap/locks/scanners.yml, secrets_scanner_image);
# pulling by digest fails if the registry serves other bytes. The binary is
# static, and the build fails unless it reports the locked version.
FROM docker.io/zricethezav/gitleaks@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f AS gitleaks

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

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
# VERIFIED: `docker build -t aurumcode-aur548-check:tmp -f Dockerfile .`
# builds this stage successfully on Alpine 3.20 -- semgrep 1.172.0 ships a
# musllinux_1_2_x86_64 wheel, so no glibc/musl incompatibility applies for
# this exact pinned version -- and `docker run --rm --entrypoint semgrep
# aurumcode-aur548-check:tmp --version` reports `1.172.0` from inside the
# built image. Should a future version bump ever drop musllinux wheel
# support, the documented fallback is switching this final stage's base
# image to a glibc distribution (e.g. python:3.11-slim) with the same
# package set installed via apt.
#
# WARNING for whoever bumps this version: `--x-ignore-semgrepignore-files`
# (internal/analysis/semgrep.go's policyOrigin flags) is an UNDOCUMENTED,
# internal Semgrep flag ("THIS OPTION IS NOT PART OF THE SEMGREP API AND
# MAY CHANGE OR DISAPPEAR WITHOUT NOTICE" per `semgrep scan --help`). A
# version bump must re-run `semgrep scan --help` and confirm the flag is
# still there before shipping -- if it silently disappears, the
# invocation errors on the unrecognized flag, which this project's own
# B1 handling already turns into a fail-closed inconclusive result
# (never a silent, unprotected scan), so a missed recheck fails safe,
# but a human should still close the gap deliberately rather than leave
# the SAST pass permanently inconclusive under policy.
RUN pip3 install --no-cache-dir --break-system-packages semgrep==1.172.0

COPY --from=gitleaks /usr/bin/gitleaks /usr/local/bin/gitleaks
RUN test "$(gitleaks version)" = "v8.30.1"

WORKDIR /github/workspace
COPY --from=builder /aurumcode /app/aurumcode
COPY scripts/action-entrypoint.sh /app/action-entrypoint.sh
RUN chmod 0755 /app/aurumcode /app/action-entrypoint.sh \
    && bash -n /app/action-entrypoint.sh

ENV AURUMCODE_CLI=/app/aurumcode
ENTRYPOINT ["/app/action-entrypoint.sh"]
