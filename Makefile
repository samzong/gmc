.PHONY: build install clean test fmt all help update-homebrew

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

build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build Command Done"

install: build
	@echo "Installing $(BINARY_NAME) $(VERSION)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Install Command Done"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean Command Done"

test:
	@echo "Running tests..."
	@go test -v ./...
	@echo "Test Command Done"

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@go mod tidy
	@echo "Format Command Done"

all: clean fmt build test

update-homebrew:
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

help:
	@echo "Available targets:"
	@echo "  build    - Build the binary"
	@echo "  install  - Build and install the binary to GOPATH"
	@echo "  clean    - Remove build artifacts"
	@echo "  test     - Run tests"
	@echo "  fmt      - Format code and tidy modules"
	@echo "  all      - Clean, format, build, and test"
	@echo "  update-homebrew - Update Homebrew formula (requires GH_PAT)"
	@echo "  help     - Show this help message"

.DEFAULT_GOAL := help 