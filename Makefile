.PHONY: build test lint cover clean docker-build docker-run docker-shell

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

lint:
	gofmt -d .
	go vet ./...

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

clean:
	go clean ./...
	rm -f coverage.out

# Development workflow
init:
	go mod init aurumcode

tidy:
	go mod tidy

