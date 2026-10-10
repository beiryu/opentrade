VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: help build web run test vet lint fmt seed replay clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: web ## Build the single binary with the web app embedded
	go build -ldflags "$(LDFLAGS)" -o opentrade ./cmd/opentrade

web: ## Build the web app into internal/api/ui/dist (needs Node 20 + pnpm)
	cd web && pnpm install --frozen-lockfile && pnpm build

run: ## Serve on 127.0.0.1:8080 (go run; UI from the last `make web`)
	go run ./cmd/opentrade serve

test: ## Run Go tests
	go test ./...

vet: ## go vet + gofmt check
	go vet ./...
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal && echo "gofmt: files need formatting" && exit 1)

lint: vet ## Lint Go and web
	cd web && pnpm lint

fmt: ## Format Go sources
	gofmt -w cmd internal

seed: ## Generate the synthetic sample data in data/sample
	go run ./cmd/opentrade seed

replay: ## Replay 2026 so far with the momentum baseline (no API key needed)
	go run ./cmd/opentrade replay --from 2026-01-02 --to 2026-10-09 --decider momentum

clean: ## Remove build outputs and the local database
	rm -rf opentrade internal/api/ui/dist/assets internal/api/ui/dist/index.html web/dist data/local
