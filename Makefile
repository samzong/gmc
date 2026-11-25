BOLD  := \033[1m
CYAN  := \033[36m
GREEN := \033[32m
RESET := \033[0m

.DEFAULT_GOAL := help

BUILD_DIR=./build
BINARY_NAME=gmc
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILDTIME=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS=-ldflags "-X github.com/samzong/gmc/cmd.Version=$(VERSION) -X 'github.com/samzong/gmc/cmd.BuildTime=$(BUILDTIME)'"

# ── Build ────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build the binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build Command Done"

.PHONY: install
install: build ## Install the binary to GOPATH/bin (or ~/go/bin if GOPATH unset)
	@echo "Installing $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(or $(GOPATH),$(HOME)/go)/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(or $(GOPATH),$(HOME)/go)/bin/
	@if [ -d "$(HOME)/.zsh/completions" ]; then \
		$(BUILD_DIR)/$(BINARY_NAME) completion zsh > "$(HOME)/.zsh/completions/_gmc"; \
		printf '$(GREEN)  ✓ Updated ~/.zsh/completions/_gmc$(RESET)\n'; \
	fi
	@echo "Install Command Done"

.PHONY: completions
completions: build ## Regenerate zsh completion script (~/.zsh/completions/_gmc)
	@mkdir -p "$(HOME)/.zsh/completions"
	@$(BUILD_DIR)/$(BINARY_NAME) completion zsh > "$(HOME)/.zsh/completions/_gmc"
	@printf '$(GREEN)  ✓ Wrote ~/.zsh/completions/_gmc$(RESET)\n'

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean Command Done"

# ── Development ──────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...
	@echo "Test Command Done"

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -coverprofile=$(BUILD_DIR)/coverage.out ./...
	@go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@go tool cover -func=$(BUILD_DIR)/coverage.out
	@echo "Coverage report generated: $(BUILD_DIR)/coverage.html"

.PHONY: test-coverage-ci
test-coverage-ci: ## Run tests with coverage for CI (no HTML)
	@echo "Running tests with coverage (CI mode)..."
	@go test -coverprofile=$(BUILD_DIR)/coverage.out ./...
	@go tool cover -func=$(BUILD_DIR)/coverage.out | tail -1

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format Command Done"

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	@echo "Tidying modules..."
	@go mod tidy
	@echo "Tidy Command Done"

.PHONY: lint
lint: ## Run code analysis
	@echo "Running code analysis..."
	$(call ensure-external-tool,golangci-lint,$(GOLANGCI_LINT_INSTALL))
	golangci-lint run
	@echo "$(GREEN)Code analysis completed$(RESET)"

.PHONY: lint-fix
lint-fix: ## Run code analysis with auto-fix
	@echo "Running code analysis with auto-fix..."
	$(call ensure-external-tool,golangci-lint,$(GOLANGCI_LINT_INSTALL))
	golangci-lint run --fix
	@echo "$(GREEN)Code analysis and fixes completed$(RESET)"

.PHONY: man
man: ## Generate man pages
	@echo "Generating man pages..."
	@mkdir -p docs/man
	@go run cmd/gendoc/main.go
	@echo "Man pages generated in docs/man/"

# ── Quality ──────────────────────────────────────────────────────────────────

.PHONY: check
check: fmt lint test ## Run all quality checks (fmt, lint, test)
	@echo "$(GREEN)All quality checks passed!$(RESET)"

# ── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "\n$(BOLD)gmc$(RESET) — parallel worktrees for AI agents, plus AI-generated commits\n"} \
		/^# ── / {n = $$0; gsub(/(^# ── | (─)+$$)/, "", n); printf "\n$(BOLD)%s$(RESET)\n", n} \
		/^[a-zA-Z0-9_-]+:.*## / {printf "  $(CYAN)make %-17s$(RESET) %s\n", $$1, $$2} \
		END {printf "\n"}' $(MAKEFILE_LIST)
