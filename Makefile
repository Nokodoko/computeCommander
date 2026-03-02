VERSION ?= 0.2.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: build test lint vet clean

build:
	go build $(LDFLAGS) -o cmdr ./cmd/cc/

test:
	go test ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

vet:
	go vet ./...

clean:
	rm -f cmdr
