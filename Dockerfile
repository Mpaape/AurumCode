FROM golang:1.21-alpine AS builder

WORKDIR /build

# Install git and build dependencies
RUN apk add --no-cache git make

# Copy go mod files first
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o aurumcode ./cmd/server
RUN go build -o aurumcode-cli ./cmd/cli
RUN go build -o test-docs-pipeline ./cmd/test-docs-pipeline

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

# Copy binaries from builder
COPY --from=builder /build/aurumcode /app/server
COPY --from=builder /build/aurumcode-cli /app/cli
COPY --from=builder /build/test-docs-pipeline /app/test-docs-pipeline

# Copy scripts for GitHub Action
COPY scripts/ /app/scripts/
RUN chmod +x /app/scripts/*.sh

# Default command for server mode
CMD ["./server"]

# When used as GitHub Action, this will be overridden by action.yml
ENTRYPOINT ["/bin/bash", "/app/scripts/action-entrypoint.sh"]

