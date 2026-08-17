.DEFAULT_GOAL := help

.PHONY: test
test: ## Run tests
	go test -race -count=1 ./...

.PHONY: coverage
coverage: ## Run tests with coverage report
	go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | tail -1

.PHONY: coverage-html
coverage-html: coverage ## Open HTML coverage report
	go tool cover -html=coverage.out -o coverage.html

.PHONY: bench
bench: ## Run benchmarks
	go test -run=^$$ -bench=. -benchmem ./...

.PHONY: fuzz
fuzz: ## Run fuzz targets for 30s each
	@for f in $$(go test -list='Fuzz.*' ./... | grep '^Fuzz'); do \
		echo "== $$f"; \
		go test -run=^$$ -fuzz=^$$f$$ -fuzztime=30s . || exit 1; \
	done

.PHONY: lint
lint: ## Run golangci-lint (needs a build against Go 1.27 or newer)
	@golangci-lint run ./... || { \
		echo; \
		echo "golangci-lint must itself be built with Go 1.27+ to read this module."; \
		echo "Until a release ships, 'make vet' is the gate."; \
		exit 1; \
	}

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format code
	go fmt ./...

.PHONY: ci
ci: vet test ## Run vet + tests

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'
