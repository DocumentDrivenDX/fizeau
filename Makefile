.PHONY: build build-ci install-quality-tools test test-no-race test-race lint vet fmt fmt-check gosec govulncheck ci-checks ci adapter-pytest check clean coverage coverage-ratchet coverage-bump coverage-history catalog-dist rename-noise-check demos-capture demos-regen docs-cli docs-embedding docs-tools docs-adrs capture-machine-info

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BINARY_NAME := fiz
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/fiz

catalog-dist:
	go run ./cmd/catalogdist \
		--manifest internal/modelcatalog/catalog/models.yaml \
		--out website/static/catalog \
		--channel stable \
		--min-agent-version "$${MIN_AGENT_VERSION:-$$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || echo dev)}"

rename-noise-check:
	go run ./cmd/renamecheck --repo . --fail

# docs-cli regenerates the Hugo CLI reference at website/content/docs/cli/
# from the live Cobra command tree. Run after changing fiz commands or flags.
docs-cli:
	go run ./cmd/docgen-cli --out website/content/docs/cli

# docs-embedding regenerates the embedding (Go library) reference at
# website/content/docs/embedding/ from the public fizeau API source via
# go/doc. Run after changing the public surface in service.go,
# public_api.go, or public_cli_api.go.
docs-embedding:
	go run ./cmd/docgen-embedding --pkg . --out website/content/docs/embedding/_index.md

# docs-tools regenerates the tool-calling reference at
# website/content/docs/tools/_index.md by enumerating
# fizeau.BuiltinToolsForPreset for every prompt preset and emitting each
# tool's name, description, JSON Schema, and parallel-safety. Run after
# changing the registry in internal/tool/builtin.go or any tool's schema.
docs-tools:
	go run ./cmd/docgen-tools --out website/content/docs/tools/_index.md

# docs-adrs republishes Architecture Decision Records from
# docs/helix/02-design/adr/ into website/content/docs/architecture/adr/ with
# Hextra-compatible front matter. Generates a per-section index and per-ADR
# pages. Idempotent: running it twice with no source changes produces no diff.
# Run after adding, editing, or superseding an ADR.
docs-adrs:
	go run ./cmd/docgen-adrs --src docs/helix/02-design/adr --out website/content/docs/architecture/adr

build-ci:
	go build ./...

install-quality-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

test:
	CGO_ENABLED=1 go test -race ./...

test-no-race:
	go test -count=1 ./...

test-race:
	CGO_ENABLED=1 go test -race -count=1 ./...

test-integration:
	CGO_ENABLED=1 go test -race -tags=integration ./...

test-e2e:
	CGO_ENABLED=1 go test -race -tags=e2e ./...

test-fuzz:
	go test -fuzz=. -fuzztime=30s ./...

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -l . | grep -v '^\.claude/' | grep -v '^\.ddx/' | grep . && exit 1 || true

fmt-check:
	@unformatted="$$(gofmt -l . | grep -v '^\.claude/' | grep -v '^\.ddx/')"; \
	if [ -n "$$unformatted" ]; then \
		echo "Files not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

gosec:
	gosec -exclude-dir=.claude -exclude-dir=.ddx ./...

govulncheck:
	govulncheck ./...

ci-checks: build-ci vet lint gosec govulncheck fmt-check rename-noise-check test-no-race test-race
	@echo "All CI checks passed."

# adapter-pytest mirrors the .github/workflows/ci.yml adapter-pytest job.
adapter-pytest:
	python -m pytest scripts/benchmark/harness_adapters

# ci runs every gate that .github/workflows/ci.yml runs (both jobs).
ci: ci-checks adapter-pytest
	@echo "All CI jobs (test + adapter-pytest) passed."

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

# Capture fresh fiz session JSONLs against a real OpenRouter model and write
# them into demos/sessions/. Requires $OPENROUTER_API_KEY. Live LLM calls.
demos-capture:
	./demos/capture.sh

# Regenerate homepage demo asciicasts from canonical session JSONLs in
# demos/sessions/. Deterministic — no live LLM calls, no `asciinema rec`.
demos-regen:
	./demos/regen.sh

# Capture this host's hardware + serving-engine inventory and emit a YAML
# block keyed by hostname. Run ON the inference machine you want to record;
# pipe to a file and paste under the matching key in
# scripts/benchmark/machines.yaml. See the script header for prerequisites.
capture-machine-info:
	./scripts/benchmark/capture-machine-info.sh

clean:
	rm -f $(BINARY_NAME)
	go clean ./...
