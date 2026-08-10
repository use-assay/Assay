BINARY := assay
PKG := ./...

.PHONY: all build test cover lint fmt vet run clean

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

clean:
	rm -f $(BINARY) coverage.out coverage.html
