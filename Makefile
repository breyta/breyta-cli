BINARY_NAME=breyta
BREYTA_REPO ?= ../breyta

.PHONY: build run install tidy fmt test integration-test release-check release

VERSION ?= $(shell (git describe --tags --dirty --always --match 'v[0-9][0-9][0-9][0-9].*' 2>/dev/null || git describe --tags --dirty --always 2>/dev/null) || echo dev)
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
	gofmt -w $$(git ls-files '*.go')

test:
	go test ./...

install: test
	go install -ldflags "$(LDFLAGS)" ./cmd/breyta
	@echo "Installed: $$(go env GOPATH)/bin/$(BINARY_NAME)"

integration-test: build
	@test -x "$(BREYTA_REPO)/bases/flows-api/scripts/integration_tests.sh" || (echo "Missing $(BREYTA_REPO) checkout; set BREYTA_REPO=/path/to/breyta" >&2; exit 1)
	BREYTA_CLI_BIN="$$(pwd)/dist/$(BINARY_NAME)" "$(BREYTA_REPO)/bases/flows-api/scripts/integration_tests.sh"

release-check: fmt test integration-test

release: release-check
	@echo "release checks passed; create and push the release tag per docs/RELEASING.md"
