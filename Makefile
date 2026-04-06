BINARY_NAME := playwright-exporter
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -X main.version=$(VERSION)
GOFILES     := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: lint test clean fmt vet check install-deps snapshot package

# Build the binary for the current platform.
$(BINARY_NAME): $(GOFILES)
	go build -ldflags "$(LDFLAGS)" -o ./$(BINARY_NAME) .

# Alias so `make build` also works.
.PHONY: build
build: $(BINARY_NAME)

# Run the linter.
lint:
	golangci-lint run

# Run tests with the race detector.
test:
	go test -race ./...

# Remove the compiled binary.
clean:
	rm -f ./$(BINARY_NAME)

# Format source files.
fmt:
	gofmt -w .
	goimports -w .

# Run go vet.
vet:
	go vet ./...

# Run all checks before committing: fmt, vet, lint, test.
check: fmt vet lint test

# Install runtime dependencies (Node.js, npm, Playwright Chromium).
install-deps:
	sudo ./scripts/install-deps.sh

# Build a snapshot release locally (no publish).
snapshot:
	goreleaser release --snapshot --clean

# Build only the packages without publishing.
package:
	goreleaser build --snapshot --clean
