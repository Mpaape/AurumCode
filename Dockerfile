FROM golang:1.21-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git

# Copy go mod files first
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o regenerate-docs ./cmd/regenerate-docs

# The Go extractor shells out to `gomarkdoc` (internal/documentation/extractors/
# go/extractor.go: extractPackage). Without it on PATH the generator does not
# fail - it SKIPS the language and reports `result=partial languages_skipped=go`
# with a zero exit status, so the image quietly produced no Go documentation at
# all for a Go repository. Pinned rather than @latest so the image is
# reproducible; `go install pkg@version` resolves in its own module and does not
# touch this repository's go.mod or go.sum.
FROM golang:1.21-alpine AS tools
RUN apk add --no-cache git
RUN CGO_ENABLED=0 go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0

# Final stage
FROM alpine:latest

# Install runtime dependencies for GitHub Action
RUN apk add --no-cache \
    ca-certificates \
    git \
    bash \
    curl \
    jq \
    wget

WORKDIR /app

# Copy binaries from builder.
#
# This image ships exactly one binary: regenerate-docs, built above from
# cmd/regenerate-docs. That is the only main package in the source tree.
# The entrypoint's `review` and `qa` modes need an additional `cli` binary
# that does not exist here; they therefore fail with an explicit message
# naming the missing executable rather than reporting a skipped success.
# If cmd/cli is ever added, build it above and copy it to /app/cli and those
# modes start working with no change to the entrypoint.
COPY --from=builder /build/regenerate-docs /app/regenerate-docs
COPY --from=tools /go/bin/gomarkdoc /usr/local/bin/gomarkdoc

# Copy scripts for GitHub Action
COPY scripts/ /app/scripts/
RUN chmod +x /app/scripts/*.sh

# Fail the build if the entrypoint's binary directory does not actually
# contain what the entrypoint documents, so the image and the script cannot
# drift apart silently.
RUN test -x /app/regenerate-docs \
    && test -x /usr/local/bin/gomarkdoc \
    && test -x /app/scripts/action-entrypoint.sh \
    && bash -n /app/scripts/action-entrypoint.sh \
    && bash -n /app/scripts/build-docs-site.sh \
    && bash -n /app/scripts/generate-enhanced-docs.sh

# `documentation` is the only mode this image can actually serve, so the image
# does not ship until that mode's behavioral suite passes INSIDE it, against
# this image's own busybox userland. The suite builds its own git workspaces
# and stand-in generators under /tmp and proves, among other things, that a run
# which generates nothing fails closed instead of reporting a complete
# pipeline. A syntax check alone would not have caught that.
#
# The exit status alone is NOT the gate. A suite whose cases all die in a
# subshell can still exit 0, which is the same class of defect this image is
# being gated against, so the number of cases that actually reported a verdict
# is recounted here from the suite's own output and compared with the number
# the suite is expected to contain. Trusting the status would mean trusting a
# word instead of recomputing the fact.
ARG DOCMODE_CASES=18
RUN set -eu; \
    cd /tmp; \
    rc=0; \
    bash /app/scripts/tests/documentation-mode.test.sh >/tmp/docmode.log 2>&1 || rc=$?; \
    cat /tmp/docmode.log; \
    if [ "$rc" -ne 0 ]; then \
        echo "documentation-mode suite exited $rc inside the image" >&2; exit 1; \
    fi; \
    passed="$(grep -c '^PASS ' /tmp/docmode.log || true)"; \
    if [ "$passed" -ne "$DOCMODE_CASES" ]; then \
        echo "documentation-mode suite exited 0 with $passed PASS line(s); expected $DOCMODE_CASES. A suite that reports success without running its cases is not a gate." >&2; \
        exit 1; \
    fi; \
    grep -qx "all ${DOCMODE_CASES} case(s) held" /tmp/docmode.log \
        || { echo "documentation-mode suite did not confirm it ran ${DOCMODE_CASES} case(s)" >&2; exit 1; }; \
    rm -f /tmp/docmode.log

# The suite above drives the entrypoint with stand-in generators, which proves
# the gating logic but says nothing about whether THIS image can actually
# document anything. This runs the real /app/regenerate-docs, through the real
# entrypoint, over a real Go package, and then re-derives the answer from disk:
# the markdown for the fixture's source file has to exist. Nothing here trusts
# an exit status or a printed word - the generator returns 0 while skipping a
# language whose tool is missing, which is exactly how this image came to
# produce no Go documentation at all without anyone noticing.
RUN set -eu; \
    fix=/tmp/gofix; \
    mkdir -p "$fix/pkg"; \
    printf 'module aurumcode.test/fix\n\ngo 1.21\n' >"$fix/go.mod"; \
    printf 'package pkg\n\n// Thing does a thing.\nfunc Thing() {}\n' >"$fix/pkg/thing.go"; \
    rc=0; \
    env AURUMCODE_MODE=documentation GITHUB_WORKSPACE="$fix" DOCUMENTATION_MODE=full \
        bash /app/scripts/action-entrypoint.sh >/tmp/gofix.log 2>&1 || rc=$?; \
    cat /tmp/gofix.log; \
    if [ "$rc" -ne 0 ]; then \
        echo "this image cannot document a Go package end to end (entrypoint exit $rc)" >&2; exit 1; \
    fi; \
    doc="$(find "$fix/.aurumcode/go" -type f -name '*.md' -print -quit 2>/dev/null || true)"; \
    if [ -z "$doc" ]; then \
        echo "the run exited 0 but left no Go markdown under .aurumcode/go; the Go extractor was skipped" >&2; \
        find "$fix/.aurumcode" -type f >&2; \
        exit 1; \
    fi; \
    if ! grep -q 'Thing' "$doc"; then \
        echo "the Go markdown at $doc does not mention the fixture's exported symbol, so it was not derived from the source" >&2; \
        cat "$doc" >&2; \
        exit 1; \
    fi; \
    echo "image documents Go end to end: $doc"; \
    rm -rf "$fix" /tmp/gofix.log

# The mode is required and has no default: an unset AURUMCODE_MODE is an
# error, not an implicit "all".
ENV AURUMCODE_BIN_DIR=/app

# When used as GitHub Action, this will be overridden by action.yml
ENTRYPOINT ["/bin/bash", "/app/scripts/action-entrypoint.sh"]
