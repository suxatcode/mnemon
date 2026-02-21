# ──────────────────────────────────────────────────────────────────────
# Mnemon Makefile
# ──────────────────────────────────────────────────────────────────────

BINARY      := mnemon
GOBIN       := $(shell go env GOBIN)
ifeq ($(GOBIN),)
  GOBIN     := $(shell go env GOPATH)/bin
endif

ASSETS_DIR := internal/setup/assets/claude

.PHONY: build install uninstall sync-assets check-assets test clean help

.DEFAULT_GOAL := help

# ── Build ────────────────────────────────────────────────────────────

build: ## Build the mnemon binary
	go build -o $(BINARY) .

# ── Install / Uninstall ─────────────────────────────────────────────

install: build ## Build and install mnemon to $GOBIN
	@mkdir -p $(GOBIN)
	cp $(BINARY) $(GOBIN)/$(BINARY)
	@echo "Installed: $(GOBIN)/$(BINARY)"

uninstall: ## Remove mnemon binary from $GOBIN
	rm -f $(GOBIN)/$(BINARY)
	@echo "Removed: $(GOBIN)/$(BINARY)"
	@echo "Run 'mnemon setup --eject' first to remove integrations."

# ── Asset Sync ──────────────────────────────────────────────────────

sync-assets: ## Sync source-of-truth files into embedded assets
	@cp scripts/hooks/user_prompt.sh $(ASSETS_DIR)/user_prompt.sh
	@cp scripts/hooks/stop.sh $(ASSETS_DIR)/stop.sh
	@cp scripts/hooks/prime.sh $(ASSETS_DIR)/prime.sh
	@cp scripts/hooks/compact.sh $(ASSETS_DIR)/compact.sh
	@cp skills/mnemon/SKILL.md $(ASSETS_DIR)/SKILL.md
	@echo "Assets synced to $(ASSETS_DIR)/"

check-assets: sync-assets ## Verify embedded assets match source (CI)
	@git diff --exit-code $(ASSETS_DIR)/ || (echo "ERROR: Embedded assets out of sync. Run 'make sync-assets'." && exit 1)

# ── Test ─────────────────────────────────────────────────────────────

test: build ## Run E2E test suite
	bash scripts/e2e_test.sh

# ── Clean ────────────────────────────────────────────────────────────

clean: ## Remove build artifacts and test data
	rm -f $(BINARY)
	rm -rf .testdata

# ── Help ─────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
