.PHONY: build test test-race lint cover clean tidy docker-build docker-run docker-shell

IMAGE ?= aurumcode-dev
GO_IMAGE ?= golang:1.21-alpine

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm -it -v "$(PWD):/workspace" -w /workspace $(IMAGE) review --help

docker-shell:
	docker run --rm -it --entrypoint sh -v "$(PWD):/workspace" -w /workspace $(IMAGE)

# Go is never required on the host. These targets use a disposable toolchain
# container and the repository checkout as a volume.
build:
	docker run --rm -v "$(PWD):/src" -w /src $(GO_IMAGE) go build ./...

test:
	docker run --rm -v "$(PWD):/src" -w /src $(GO_IMAGE) go test ./... -count=1

test-race:
	docker run --rm -v "$(PWD):/src" -w /src golang:1.21-bookworm sh -c 'CGO_ENABLED=1 go test ./... -race -count=1'

lint:
	docker run --rm -v "$(PWD):/src" -w /src $(GO_IMAGE) sh -c 'test -z "$$(gofmt -l .)" && go vet ./...'

cover:
	docker run --rm -v "$(PWD):/src" -w /src $(GO_IMAGE) sh -c 'go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out'

clean:
	docker run --rm -v "$(PWD):/src" -w /src $(GO_IMAGE) go clean ./...

tidy:
	docker run --rm -v "$(PWD):/src" -w /src $(GO_IMAGE) go mod tidy
