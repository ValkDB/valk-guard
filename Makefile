.PHONY: build test lint vet fmt clean run tidy cover install verify-go-version

BINARY := valk-guard
CMD    := ./cmd/valk-guard
REQUIRED_GO_VERSION := 1.25.8
GO_VERSION := $(shell go env GOVERSION 2>/dev/null | sed 's/^go//')
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

verify-go-version:
	@if [ -z "$(GO_VERSION)" ]; then \
		echo "Unable to determine Go version."; \
		exit 1; \
	fi
	@if [ "$$(printf '%s\n%s\n' "$(REQUIRED_GO_VERSION)" "$(GO_VERSION)" | sort -V | head -n1)" != "$(REQUIRED_GO_VERSION)" ]; then \
		echo "Go >= $(REQUIRED_GO_VERSION) is required (found $(GO_VERSION))."; \
		exit 1; \
	fi

build: verify-go-version
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

test: verify-go-version
	go test -race ./...

test-v: verify-go-version
	go test -race -v ./...

cover: verify-go-version
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view in browser: go tool cover -html=coverage.out"

lint: verify-go-version
	golangci-lint run ./...

vet: verify-go-version
	go vet ./...

fmt:
	gofmt -w .
	goimports -w .

tidy:
	go mod tidy

install: verify-go-version
	go install -ldflags "$(LDFLAGS)" $(CMD)
	@bin_dir="$$(go env GOBIN)"; \
	if [ -z "$$bin_dir" ]; then bin_dir="$$(go env GOPATH)/bin"; fi; \
	echo "Installed $(BINARY) to $$bin_dir"; \
	echo "Add $$bin_dir to PATH if '$(BINARY)' is not found."

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist/

run: build
	./$(BINARY) scan .

check: verify-go-version fmt vet lint test
	@echo "All checks passed."
