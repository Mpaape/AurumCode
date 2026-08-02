.PHONY: build test test-race lint cover clean tidy docker-build docker-run docker-shell

# Docker commands
docker-build:
	docker-compose build

docker-run:
	docker-compose run --rm aurumcode

docker-shell:
	docker-compose run --rm aurumcode sh

# Go commands (run inside Docker)
build:
	go build ./...

test:
	go test ./... -v

# Requires cgo (a C compiler): the project's own dev image (Dockerfile:
# golang:1.21-alpine) has no C toolchain, so running this target inside
# `docker-compose run aurumcode` fails on the missing compiler regardless of
# whether a race exists - that failure is not a race finding. Run it on a
# host with gcc, or in a Debian-based Go image, e.g.:
#   docker run --rm -v "$$PWD":/src -w /src golang:1.21 make test-race
test-race:
	CGO_ENABLED=1 go test ./... -race -count=1

# `gofmt -l` is used instead of `gofmt -d` because gofmt exits 0 even when it
# reports formatting differences, which would make this target incapable of
# failing.
lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		printf 'gofmt needed:\n%s\n' "$$unformatted" >&2; \
		exit 1; \
	fi
	go vet ./...

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

clean:
	go clean ./...
	rm -f coverage.out

tidy:
	go mod tidy
