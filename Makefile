.PHONY: build test fmt clean

BIN := dist/ski
GO_SOURCES := $(shell find cmd internal -name '*.go')
BUILD_INPUTS := $(GO_SOURCES) go.mod go.sum Makefile

build: $(BIN)

$(BIN): $(BUILD_INPUTS)
	go build -o $(BIN) ./cmd/ski

test:
	go test ./...

fmt:
	gofmt -w $(GO_SOURCES)

clean:
	rm -f $(BIN)
