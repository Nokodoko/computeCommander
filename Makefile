VERSION ?= 0.2.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

FOCUS_WATCHER_DIR := plugins/focus-watcher
FOCUS_WATCHER_BIN := $(FOCUS_WATCHER_DIR)/target/release/focus-watcher

.PHONY: build build-focus-watcher build-tg-viz build-bridge test lint vet clean install install-bridge generate-types

build: build-focus-watcher build-tg-viz
	go build $(LDFLAGS) -o cmdr ./cmd/cc/

build-tg-viz:
	go build $(LDFLAGS) -o bin/tg-viz ./cmd/tg-viz/

build-focus-watcher:
	@command -v cargo >/dev/null 2>&1 || { echo "ERROR: cargo (Rust toolchain) is required to build focus-watcher"; exit 1; }
	cargo build --release --manifest-path $(FOCUS_WATCHER_DIR)/Cargo.toml

install: build
	@mkdir -p $(HOME)/.local/bin
	cp cmdr $(HOME)/.local/bin/cmdr
	cp $(FOCUS_WATCHER_BIN) $(HOME)/.local/bin/focus-watcher
	cp bin/tg-viz $(HOME)/.local/bin/tg-viz

test:
	go test ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

vet:
	go vet ./...

build-bridge:
	go build $(LDFLAGS) -o bin/hook-bridge ./cmd/hook-bridge/

install-bridge: build-bridge
	@mkdir -p $(HOME)/.local/bin
	cp bin/hook-bridge $(HOME)/.local/bin/hook-bridge

generate-types:
	go run ./cmd/hook-bridge/ --generate

clean:
	rm -f cmdr
	rm -f bin/hook-bridge
	rm -f bin/tg-viz
	cargo clean --manifest-path $(FOCUS_WATCHER_DIR)/Cargo.toml 2>/dev/null || true
