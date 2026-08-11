BINARY := assay
PKG := ./...

CONTRACTS := assay-contracts

.PHONY: all build test cover lint fmt vet run clean \
	contract-test contract-lint contract-build

all: build

build:
	go build -o $(BINARY) ./cmd/assay

test:
	go test -race $(PKG)

cover:
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out | tail -1

fmt:
	gofmt -w .

vet:
	go vet $(PKG)

lint:
	golangci-lint run

run: build
	./$(BINARY) serve

contract-test:
	cd $(CONTRACTS) && cargo test

contract-lint:
	cd $(CONTRACTS) && cargo fmt --all -- --check
	cd $(CONTRACTS) && cargo clippy --all-targets -- -D warnings

contract-build:
	cd $(CONTRACTS) && cargo build --release --target wasm32-unknown-unknown

clean:
	rm -f $(BINARY) coverage.out coverage.html
	cd $(CONTRACTS) && cargo clean
