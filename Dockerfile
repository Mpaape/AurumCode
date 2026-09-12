FROM golang:1.21-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /aurumcode ./cmd/aurumcode

FROM alpine:3.20

RUN apk add --no-cache ca-certificates git bash jq

WORKDIR /github/workspace
COPY --from=builder /aurumcode /app/aurumcode
COPY scripts/action-entrypoint.sh /app/action-entrypoint.sh
RUN chmod 0755 /app/aurumcode /app/action-entrypoint.sh \
    && bash -n /app/action-entrypoint.sh

ENV AURUMCODE_CLI=/app/aurumcode
ENTRYPOINT ["/app/action-entrypoint.sh"]
