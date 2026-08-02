.PHONY: build test lint cover clean tidy docker-build docker-run docker-shell

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
