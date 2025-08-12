SHELL := /bin/bash

GO      ?= go
PKGS    := $(shell $(GO) list ./... | grep -v /vendor/)
CACHEPKG:= ./cache

# ---- Lint config ----
GOLANGCI_LINT ?= $(shell $(GO) env GOPATH)/bin/golangci-lint
LINT_VERSION  ?= v1.61.0

.PHONY: help test race cover coverhtml bench fuzz lint lint-install tidy vet fmt ci \
        bench-cmd bench-cmd-2q tools

help:
	@echo "Targets:"
	@echo "  make test          - run all tests with -race"
	@echo "  make cover         - run tests with coverage (atomic) -> coverage.out"
	@echo "  make coverhtml     - openable HTML coverage report -> coverage.html"
	@echo "  make bench         - run microbenchmarks in $(CACHEPKG)"
	@echo "  make fuzz          - run fuzz test (short default)"
	@echo "  make lint          - run golangci-lint (auto-installs if missing)"
	@echo "  make tidy vet fmt  - module tidy, vet, fmt"
	@echo "  make ci            - tidy + vet + fmt + test + lint"
	@echo "  make bench-cmd     - run cmd/bench (use ARGS='...')"
	@echo "  make bench-cmd-2q  - run cmd/bench with 2Q defaults"

# ---- Tests ----
test:
	$(GO) test -race -count=1 ./...

race: test

cover:
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./...

coverhtml: cover
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

# ---- Benchmarks ----
bench:
	$(GO) test $(CACHEPKG) -bench . -benchmem -run ^$

# ---- Fuzz (requires Go 1.18+) ----
fuzz:
	$(GO) test $(CACHEPKG) -run=Fuzz -fuzz=FuzzCache_SetGetRemove -fuzztime=10s

# ---- Lint ----
lint: lint-install
	$(GOLANGCI_LINT) run

lint-install:
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { \
		echo "Installing golangci-lint $(LINT_VERSION)..." ; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
			sh -s -- -b $$($(GO) env GOPATH)/bin $(LINT_VERSION) ; \
	}

# ---- Hygiene ----
tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

ci: tidy vet fmt test lint

# ---- Run the load generator (cmd/bench) ----
# Pass custom flags via ARGS, e.g.:
#   make bench-cmd ARGS="-cap=100000 -reads=85 -duration=20s -http=:8080 -pprof=:6060"
ARGS ?=
bench-cmd:
	$(GO) run ./cmd/bench $(ARGS)

bench-cmd-2q:
	$(GO) run ./cmd/bench -policy=2q -cap=100000 -shards=0 -reads=85 -duration=20s -http=:8080 -pprof=:6060
