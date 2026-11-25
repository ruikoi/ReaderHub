.PHONY: check build build-all test-e2e

export GOCACHE := $(CURDIR)/.cache/go-build
export GOMODCACHE := $(CURDIR)/.cache/go-mod
export GOTOOLCHAIN := auto

check:
	npm --prefix web run build
	go test ./...
	go vet ./...

build:
	bash scripts/build.sh

build-all:
	bash scripts/build.sh --all

test-e2e: build
	bash scripts/test-e2e.sh
