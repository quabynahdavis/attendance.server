GO := go
GOFLAGS := -trimpath
MAIN_PKG := ./cmd/attendance
BIN_DIR := bin
BINARY := $(BIN_DIR)/attendance

.DEFAULT_GOAL := help

.PHONY: all build env run seed dev test test-coverage bench lint fmt vet deps install clean check docker up help

all: build

$(BINARY): $(shell find . -name '*.go' -not -path './vendor/*' -not -path './bin/*')
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY) $(MAIN_PKG)

build: $(BINARY)

env:
	@test -f .env || cp .env.example .env

run: build
	set -a; [ -f .env ] && . ./.env; set +a; exec $(BINARY)

seed: env
	set -a; [ -f .env ] && . ./.env; set +a; $(GO) run ./cmd/seed

dev:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	if ! command -v air >/dev/null 2>&1; then \
		echo "air is not installed; install github.com/air-verse/air"; \
		exit 1; \
	fi; \
	exec air -c air.toml

test:
	$(GO) test ./... -count=1

test-coverage:
	$(GO) test ./... -count=1 -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html

bench:
	$(GO) test ./... -bench=. -benchmem -count=3

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint is not installed; install github.com/golangci/golangci-lint/cmd/golangci-lint"; \
		exit 1; \
	fi
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

deps:
	$(GO) mod download
	$(GO) mod tidy

install: $(BINARY)
	cp $(BINARY) $(shell $(GO) env GOPATH)/bin/attendance

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html

check: fmt vet test build

docker:
	docker build -t attendance .

up:
	docker compose up --build

help:
	@echo "Attendance server"
	@echo ""
	@echo "Targets:"
	@echo "  all             Build the attendance server"
	@echo "  build           Build $(BINARY)"
	@echo "  env             Copy .env.example to .env when .env is missing"
	@echo "  run             Build and run the server (loads .env when present)"
	@echo "  seed            Load .env and run go run ./cmd/seed"
	@echo "  dev             Run air -c air.toml (loads .env when present)"
	@echo "  test            Run tests once"
	@echo "  test-coverage   Write coverage.out and coverage.html"
	@echo "  bench           Run benchmarks with memory stats"
	@echo "  lint            Run golangci-lint"
	@echo "  fmt             Format Go source"
	@echo "  vet             Run go vet"
	@echo "  deps            Download modules and tidy go.mod"
	@echo "  install         Copy $(BINARY) into GOPATH/bin"
	@echo "  clean           Remove the binary directory and coverage files"
	@echo "  check           Format, vet, test, and build"
	@echo "  docker          Build the attendance image"
	@echo "  up              Build and start containers with Docker Compose"
	@echo "  help            Show this help"
