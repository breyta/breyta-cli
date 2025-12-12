BINARY_NAME=breyta

.PHONY: build run install tidy fmt

build:
	go build -o ./dist/$(BINARY_NAME) ./cmd/breyta

run:
	go run ./cmd/breyta

tidy:
	go mod tidy

fmt:
	gofmt -w .

install:
	go install ./cmd/breyta
	@echo "Installed: $$(go env GOPATH)/bin/$(BINARY_NAME)"
