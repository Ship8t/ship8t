MODULE   := github.com/ship8t/ship8t
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X $(MODULE)/internal/version.stampedVersion=$(VERSION)
BINARIES := ship8t ship8td
TARGETS  := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: build test vet fmt-check vuln dist clean

# Build both binaries for the current platform into bin/.
build:
	@mkdir -p bin
	@for b in $(BINARIES); do \
		go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$$b ./cmd/$$b || exit 1; \
	done

test:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt needed for:"; echo "$$out"; exit 1; fi

# Needs network access to fetch the vulnerability database. Not part of "go test".
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Cross-compile every binary for every supported platform and write checksums.
dist:
	@rm -rf dist && mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		for b in $(BINARIES); do \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
				go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$$b-$$os-$$arch ./cmd/$$b || exit 1; \
		done; \
	done
	@cd dist && shasum -a 256 * > SHA256SUMS

clean:
	rm -rf bin dist
