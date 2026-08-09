FROM golang@sha256:2414035b086e3c42b99654c8b26e6f5b1b1598080d65fd03c7f499552ff4dc94

# The board runner always invokes bash and AUR-006 executes Go tests. Install
# Bash with its Alpine runtime libraries in the same digest-pinned Go image.
RUN apk add --no-cache bash

WORKDIR /opt/aurum-a006
COPY go.mod go.sum ./
RUN GOTOOLCHAIN=local GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org go mod download

ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOSUMDB=off \
    GONOSUMDB=*
ENTRYPOINT ["/usr/local/bin/bash"]
