.PHONY: build test fmt clean release assert-release-version release-darwin-arm64 release-darwin-amd64 release-linux-amd64 release-linux-arm64

VERSION ?= dev
BIN := dist/ski
GO_SOURCES := $(shell find cmd internal -name '*.go')
BUILD_INPUTS := $(GO_SOURCES) go.mod go.sum Makefile
VERSION_STAMP := dist/.version-$(VERSION)
LDFLAGS := -X github.com/Z-Bra0/Ski/internal/buildinfo.Version=$(VERSION)
RELEASE_PLATFORMS := darwin-arm64 darwin-amd64 linux-amd64 linux-arm64
RELEASE_ARCHIVES := $(foreach platform,$(RELEASE_PLATFORMS),dist/ski_$(VERSION)_$(subst -,_,$(platform)).tar.gz)
CHECKSUMS_FILE := dist/ski_$(VERSION)_checksums.txt

build: $(BIN)

$(VERSION_STAMP):
	mkdir -p dist
	rm -f dist/.version-*
	touch $@

$(BIN): $(BUILD_INPUTS) $(VERSION_STAMP)
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ski

test:
	go test ./...

fmt:
	gofmt -w $(GO_SOURCES)

clean:
	rm -rf dist/*
	touch dist/.gitkeep

assert-release-version:
	@if [ "$(VERSION)" = "dev" ]; then \
		echo "VERSION is required for release, for example: make release VERSION=0.2.1"; \
		exit 1; \
	fi

release: assert-release-version $(RELEASE_PLATFORMS:%=release-%)
	@if command -v shasum >/dev/null 2>&1; then \
		cd dist && shasum -a 256 $(notdir $(RELEASE_ARCHIVES)) > $(notdir $(CHECKSUMS_FILE)); \
	elif command -v sha256sum >/dev/null 2>&1; then \
		cd dist && sha256sum $(notdir $(RELEASE_ARCHIVES)) > $(notdir $(CHECKSUMS_FILE)); \
	else \
		echo "shasum or sha256sum is required to generate release checksums"; \
		exit 1; \
	fi

release-darwin-arm64: assert-release-version $(BUILD_INPUTS)
	$(call build_release,darwin,arm64)

release-darwin-amd64: assert-release-version $(BUILD_INPUTS)
	$(call build_release,darwin,amd64)

release-linux-amd64: assert-release-version $(BUILD_INPUTS)
	$(call build_release,linux,amd64)

release-linux-arm64: assert-release-version $(BUILD_INPUTS)
	$(call build_release,linux,arm64)

define build_release
	rm -rf dist/ski_$(VERSION)_$(1)_$(2)
	mkdir -p dist/ski_$(VERSION)_$(1)_$(2)
	GOOS=$(1) GOARCH=$(2) go build -ldflags "$(LDFLAGS)" -o dist/ski_$(VERSION)_$(1)_$(2)/ski ./cmd/ski
	cp LICENSE dist/ski_$(VERSION)_$(1)_$(2)/
	tar -czf dist/ski_$(VERSION)_$(1)_$(2).tar.gz -C dist ski_$(VERSION)_$(1)_$(2)
endef
