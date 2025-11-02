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

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY --from=builder /build/aurumcode /app/aurumcode
COPY --from=builder /build/aurumcode-cli /app/aurumcode-cli
COPY --from=builder /build/test-docs-pipeline /app/test-docs-pipeline

CMD ["./aurumcode"]

