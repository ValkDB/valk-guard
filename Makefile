.PHONY: build test lint vet fmt clean run tidy cover install

BINARY := valk-guard
CMD    := ./cmd/valk-guard

build:
	go build -o $(BINARY) $(CMD)

test:
	go test -race ./...

test-v:
	go test -race -v ./...

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view in browser: go tool cover -html=coverage.out"

lint:
	golangci-lint run ./...

vet:
	go vet ./...

fmt:
	gofmt -w .
	goimports -w .

tidy:
	go mod tidy

install:
	go install $(CMD)

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist/

run: build
	./$(BINARY) scan .

check: fmt vet lint test
	@echo "All checks passed."
