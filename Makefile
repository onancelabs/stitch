BINARY ?= st
PLATFORMS = darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

.PHONY: build test vet install clean dist deps

# Fetch cobra/pflag and populate go.sum (needs network, runs once).
deps: go.sum
go.sum: go.mod
	go mod download

build: deps
	go build -o $(BINARY) ./cmd/st

test: deps
	go test ./...

vet:
	go vet ./...

install:
	go install ./cmd/st

clean:
	rm -f $(BINARY)
	rm -rf dist

# Cross-compile a static binary for every target platform into dist/.
dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -o dist/$(BINARY)-$$os-$$arch$$ext ./cmd/st ; \
	done
