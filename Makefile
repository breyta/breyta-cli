BINARY_NAME=breyta

.PHONY: build run install tidy fmt test

VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS ?= \
	-X github.com/breyta/breyta-cli/internal/buildinfo.Version=$(VERSION) \
	-X github.com/breyta/breyta-cli/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/breyta/breyta-cli/internal/buildinfo.Date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o ./dist/$(BINARY_NAME) ./cmd/breyta

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/breyta

tidy:
	go mod tidy

fmt:
	gofmt -w .

test:
	go test ./...

install: test
	go install -ldflags "$(LDFLAGS)" ./cmd/breyta
	@echo "Installed: $$(go env GOPATH)/bin/$(BINARY_NAME)"
