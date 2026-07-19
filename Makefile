MODULE  := github.com/AgileIndustrialComplex/wt
BINARY  := wt
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X 'main.version=$(VERSION)'

.PHONY: all build install test test-race vet fmt fmt-check lint tidy clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/wt

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/wt

test:
	go test ./...

test-race:
	go test -race -cover ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
