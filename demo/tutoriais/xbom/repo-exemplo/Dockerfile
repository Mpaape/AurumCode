FROM golang:1.22 AS build
WORKDIR /src
COPY . .
FROM alpine:3.20
COPY --from=docker.io/library/busybox:1.36 /bin/busybox /bin/busybox
COPY --from=build /src/app /app
