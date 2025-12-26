BUILD_DIR=./build
BINARY_NAME=gmc
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILDTIME=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS=-ldflags "-X github.com/samzong/gmc/cmd.Version=$(VERSION) -X 'github.com/samzong/gmc/cmd.BuildTime=$(BUILDTIME)'"

# Homebrew related variables
CLEAN_VERSION=$(shell echo $(VERSION) | sed 's/^v//')
HOMEBREW_TAP_REPO=homebrew-tap
FORMULA_FILE=Formula/gmc.rb
BRANCH_NAME=update-gmc-$(CLEAN_VERSION)

# Adjust architecture definitions to match goreleaser output
SUPPORTED_ARCHS = Darwin_x86_64 Darwin_arm64 Linux_x86_64 Linux_arm64

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build
.PHONY: build
build: ## Build the binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build Command Done"

.PHONY: install
install: build ## Install the binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME) $(VERSION)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Install Command Done"

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean Command Done"

##@ Development
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
fmt: ## Format code and tidy modules
	@echo "Formatting code..."
	@go fmt ./...
	@go mod tidy
	@echo "Format Command Done"

.PHONY: lint
lint: ## Run code analysis
	@echo "$(BLUE)Running code analysis...$(NC)"
	$(call ensure-external-tool,golangci-lint,$(GOLANGCI_LINT_INSTALL))
	golangci-lint run
	@echo "$(GREEN)Code analysis completed$(NC)"

.PHONY: lint-fix
lint-fix: ## Run code analysis with auto-fix
	@echo "$(BLUE)Running code analysis with auto-fix...$(NC)"
	$(call ensure-external-tool,golangci-lint,$(GOLANGCI_LINT_INSTALL))
	golangci-lint run --fix
	@echo "$(GREEN)Code analysis and fixes completed$(NC)"

.PHONY: man
man: ## Generate man pages
	@echo "Generating man pages..."
	@mkdir -p docs/man
	@go run cmd/gendoc/main.go
	@echo "Man pages generated in docs/man/"

##@ Release
.PHONY: update-homebrew
update-homebrew: ## Update Homebrew formula
	@echo "==> Starting Homebrew formula update process..."
	@if [ -z "$(GH_PAT)" ]; then \
		echo "❌ Error: GH_PAT environment variable is required"; \
		exit 1; \
	fi

	@echo "==> Current version information:"
	@echo "    - VERSION: $(VERSION)"
	@echo "    - CLEAN_VERSION: $(CLEAN_VERSION)"

	@echo "==> Preparing working directory..."
	@rm -rf tmp && mkdir -p tmp
	
	@echo "==> Cloning Homebrew tap repository..."
	@cd tmp && git clone https://$(GH_PAT)@github.com/samzong/$(HOMEBREW_TAP_REPO).git
	@cd tmp/$(HOMEBREW_TAP_REPO) && echo "    - Creating new branch: $(BRANCH_NAME)" && git checkout -b $(BRANCH_NAME)

	@echo "==> Processing architectures and calculating checksums..."
	@cd tmp/$(HOMEBREW_TAP_REPO) && \
	for arch in $(SUPPORTED_ARCHS); do \
		echo "    - Processing $$arch..."; \
		if [ "$(DRY_RUN)" = "1" ]; then \
			echo "      [DRY_RUN] Would download: https://github.com/samzong/gmc/releases/download/v$(CLEAN_VERSION)/gmc_$${arch}.tar.gz"; \
			case "$$arch" in \
				Darwin_x86_64) DARWIN_AMD64_SHA="fake_sha_amd64" ;; \
				Darwin_arm64) DARWIN_ARM64_SHA="fake_sha_arm64" ;; \
				Linux_x86_64) LINUX_AMD64_SHA="fake_sha_linux_amd64" ;; \
				Linux_arm64) LINUX_ARM64_SHA="fake_sha_linux_arm64" ;; \
			esac; \
		else \
			echo "      - Downloading release archive..."; \
			curl -L -sSfO "https://github.com/samzong/gmc/releases/download/v$(CLEAN_VERSION)/gmc_$${arch}.tar.gz" || { echo "❌ Failed to download $$arch archive"; exit 1; }; \
			echo "      - Calculating SHA256..."; \
			sha=$$(shasum -a 256 "gmc_$${arch}.tar.gz" | cut -d' ' -f1); \
			case "$$arch" in \
				Darwin_x86_64) DARWIN_AMD64_SHA="$$sha"; echo "      ✓ Darwin AMD64 SHA: $$sha" ;; \
				Darwin_arm64) DARWIN_ARM64_SHA="$$sha"; echo "      ✓ Darwin ARM64 SHA: $$sha" ;; \
				Linux_x86_64) LINUX_AMD64_SHA="$$sha"; echo "      ✓ Linux AMD64 SHA: $$sha" ;; \
				Linux_arm64) LINUX_ARM64_SHA="$$sha"; echo "      ✓ Linux ARM64 SHA: $$sha" ;; \
			esac; \
		fi; \
	done; \
	\
	if [ "$(DRY_RUN)" = "1" ]; then \
		echo "==> [DRY_RUN] Would update formula with:"; \
		echo "    - Darwin AMD64 SHA: $$DARWIN_AMD64_SHA"; \
		echo "    - Darwin ARM64 SHA: $$DARWIN_ARM64_SHA"; \
		echo "    - Linux AMD64 SHA: $$LINUX_AMD64_SHA"; \
		echo "    - Linux ARM64 SHA: $$LINUX_ARM64_SHA"; \
		echo "    - Would commit and push changes"; \
		echo "    - Would create PR"; \
	else \
		echo "==> Updating formula file..."; \
		echo "    - Updating version to $(CLEAN_VERSION)"; \
		sed -i '' -e 's|version ".*"|version "$(CLEAN_VERSION)"|' $(FORMULA_FILE); \
		\
		echo "    - Updating URLs and checksums"; \
		sed -i '' \
			-e '/on_macos/,/end/ { \
				/if Hardware::CPU.arm?/,/else/ { \
					s|url ".*"|url "https://github.com/samzong/gmc/releases/download/v#{version}/gmc_Darwin_arm64.tar.gz"|; \
					s|sha256 ".*"|sha256 "'"$$DARWIN_ARM64_SHA"'"|; \
				}; \
				/else/,/end/ { \
					s|url ".*"|url "https://github.com/samzong/gmc/releases/download/v#{version}/gmc_Darwin_x86_64.tar.gz"|; \
					s|sha256 ".*"|sha256 "'"$$DARWIN_AMD64_SHA"'"|; \
				}; \
			}' \
			-e '/on_linux/,/end/ { \
				/if Hardware::CPU.arm?/,/else/ { \
					s|url ".*"|url "https://github.com/samzong/gmc/releases/download/v#{version}/gmc_Linux_arm64.tar.gz"|; \
					s|sha256 ".*"|sha256 "'"$$LINUX_ARM64_SHA"'"|; \
				}; \
				/else/,/end/ { \
					s|url ".*"|url "https://github.com/samzong/gmc/releases/download/v#{version}/gmc_Linux_x86_64.tar.gz"|; \
					s|sha256 ".*"|sha256 "'"$$LINUX_AMD64_SHA"'"|; \
				}; \
			}' $(FORMULA_FILE); \
		\
		echo "    - Checking for changes..."; \
		if ! git diff --quiet $(FORMULA_FILE); then \
			echo "==> Changes detected, creating pull request..."; \
			echo "    - Adding changes to git"; \
			git add $(FORMULA_FILE); \
			echo "    - Committing changes"; \
			git commit -m "chore: bump to $(VERSION)"; \
			echo "    - Pushing to remote"; \
			git push -u origin $(BRANCH_NAME); \
			echo "    - Preparing pull request data"; \
			pr_data=$$(jq -n \
				--arg title "chore: update gmc to $(VERSION)" \
				--arg body "Auto-generated PR\nSHAs:\n- Darwin(amd64): $$DARWIN_AMD64_SHA\n- Darwin(arm64): $$DARWIN_ARM64_SHA" \
				--arg head "$(BRANCH_NAME)" \
				--arg base "main" \
				'{title: $$title, body: $$body, head: $$head, base: $$base}'); \
			echo "    - Creating pull request"; \
			curl -X POST \
				-H "Authorization: token $(GH_PAT)" \
				-H "Content-Type: application/json" \
				https://api.github.com/repos/samzong/$(HOMEBREW_TAP_REPO)/pulls \
				-d "$$pr_data"; \
			echo "✅ Pull request created successfully"; \
		else \
			echo "❌ No changes detected in formula file"; \
			exit 1; \
		fi; \
	fi

	@echo "==> Cleaning up temporary files..."
	@rm -rf tmp
	@echo "✅ Homebrew formula update process completed"

##@ Quality
.PHONY: check
check: fmt lint test ## Run all quality checks (fmt, lint, test)
	@echo "$(GREEN)All quality checks passed!$(NC)"

.DEFAULT_GOAL := help 
