# decide playground frontend.
#
# The playground is a single-page app that the Go binary embeds. Building it is
# a separate step from `go build` because it needs Node, which the Go toolchain
# knows nothing about. Contributors without Node can still build and test the Go
# side: web/dist/index.html is committed as a placeholder and decide-playground
# says so at startup.

VERSION ?= dev

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: web ## Build the CLI and the playground with the frontend embedded

.PHONY: build-cli
build-cli: ## Build the decide CLI only
	go build ./...

.PHONY: playground
playground: web ## Build and run the playground on 127.0.0.1:842
	go run ./cmd/decide-playground

.PHONY: web
web: web-install ## Build the frontend into web/dist
	cd web && npm run build

.PHONY: web-install
web-install: ## Install the frontend dependencies
	cd web && npm ci

.PHONY: web-dev
web-dev: ## Run the Vite dev server against a running decide-playground
	cd web && npm run dev

.PHONY: web-check
web-check: web-install ## Typecheck and lint the frontend
	cd web && npm run lint && npx tsc -b

.PHONY: test
test: ## Run the Go tests
	go test ./...

.PHONY: test-all
test-all: test web-check ## Run the Go tests and the frontend checks

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format the Go sources
	gofmt -w .

.PHONY: lint
lint: vet ## Vet the Go sources and fail on unformatted files
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: run
run: ## Build and run the CLI against the example ticket
	go run ./cmd/decide examples/ticket.json

.PHONY: clean
clean: ## Remove build output
	rm -rf dist web/dist/assets
