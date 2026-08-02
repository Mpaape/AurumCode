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

# Copy scripts for GitHub Action
COPY scripts/ /app/scripts/
RUN chmod +x /app/scripts/*.sh

# Fail the build if the entrypoint's binary directory does not actually
# contain what the entrypoint documents, so the image and the script cannot
# drift apart silently.
RUN test -x /app/regenerate-docs \
    && test -x /app/scripts/action-entrypoint.sh \
    && bash -n /app/scripts/action-entrypoint.sh

# The mode is required and has no default: an unset AURUMCODE_MODE is an
# error, not an implicit "all".
ENV AURUMCODE_BIN_DIR=/app

# When used as GitHub Action, this will be overridden by action.yml
ENTRYPOINT ["/bin/bash", "/app/scripts/action-entrypoint.sh"]
