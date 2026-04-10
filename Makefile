.PHONY: build build-ci install-quality-tools test test-no-race test-race lint vet fmt fmt-check gosec govulncheck ci-checks check clean coverage coverage-ratchet coverage-bump coverage-history

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

build:
	go build $(LDFLAGS) ./cmd/ddx-agent

build-ci:
	go build ./...

install-quality-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

test:
	go test -race ./...

test-no-race:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

test-integration:
	go test -race -tags=integration ./...

test-e2e:
	go test -race -tags=e2e ./...

test-fuzz:
	go test -fuzz=. -fuzztime=30s ./...

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -l . | grep . && exit 1 || true

fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Files not formatted:"; \
		gofmt -l .; \
		exit 1; \
	fi

gosec:
	gosec ./...

govulncheck:
	govulncheck ./...

ci-checks: build-ci vet lint gosec govulncheck fmt-check test-no-race test-race
	@echo "All CI checks passed."

check: fmt vet lint test coverage-ratchet
	@echo "All checks passed."

# Coverage targets
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "Full coverage report: coverage.html"
	go tool cover -html=coverage.out -o coverage.html

coverage-ratchet:
	@echo "Running coverage ratchet check..."
	@go run scripts/coverage-ratchet.go

coverage-bump: coverage-ratchet
	@echo "Auto-bumping coverage floors where coverage exceeds floor by >10%..."
	@go run scripts/coverage-ratchet.go --bump

coverage-history:
	@echo "Coverage history (from .helix-ratchets/coverage-floor.json):"
	@cat .helix-ratchets/coverage-floor.json | jq '.history'

coverage-trend: coverage-ratchet
	@echo "Coverage trend from history:"
	@go run scripts/coverage-ratchet.go --trend

clean:
	rm -f ddx-agent
	go clean ./...
