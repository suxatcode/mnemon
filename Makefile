# ──────────────────────────────────────────────────────────────────────
# Mnemon Makefile
# ──────────────────────────────────────────────────────────────────────

BINARY      := mnemon
GOBIN       := $(shell go env GOBIN)
ifeq ($(GOBIN),)
  GOBIN     := $(shell go env GOPATH)/bin
endif

SKILL_SRC   := skills/mnemon
SKILL_DST   := $(HOME)/.claude/skills/mnemon

HOOKS_SRC   := scripts/hooks
HOOKS_DST   := $(HOME)/.claude/hooks/mnemon
CLAUDE_SETTINGS := $(HOME)/.claude/settings.json
CLAUDE_MEMORY   := scripts/claude_memory.md
CLAUDE_MD       := CLAUDE.md

.PHONY: build install uninstall inject eject inject-hooks eject-hooks claude-inject claude-eject setup sync-assets check-assets test clean help

.DEFAULT_GOAL := help

# ── Build ────────────────────────────────────────────────────────────

build: ## Build the mnemon binary
	go build -o $(BINARY) .

# ── Install / Uninstall ─────────────────────────────────────────────

install: build ## Build and install mnemon to $GOBIN
	@mkdir -p $(GOBIN)
	cp $(BINARY) $(GOBIN)/$(BINARY)
	@echo "Installed: $(GOBIN)/$(BINARY)"

uninstall: eject eject-hooks ## Remove binary, skill, and hooks
	rm -f $(GOBIN)/$(BINARY)
	@echo "Removed: $(GOBIN)/$(BINARY)"

# ── Skill ────────────────────────────────────────────────────────────

inject: ## Install mnemon skill to ~/.claude/skills/mnemon/
	@mkdir -p $(SKILL_DST)
	cp $(SKILL_SRC)/SKILL.md $(SKILL_DST)/SKILL.md
	@echo "  Skill → $(SKILL_DST)/SKILL.md"

eject: ## Remove mnemon skill
	@if [ -d "$(SKILL_DST)" ]; then \
		rm -rf "$(SKILL_DST)"; \
		echo "Removed: $(SKILL_DST)"; \
	else \
		echo "No mnemon skill found at $(SKILL_DST)"; \
	fi

# ── Hooks (Claude Code only) ────────────────────────────────────────

define JQ_REMOVE_MNEMON
def has_mnemon: ((.command? // "") | test("mnemon")) or ((.prompt? // "") | test("mnemon"));
def no_mnemon: (has_mnemon | not) and ((.hooks? // []) | all(has_mnemon | not));
.hooks //= {} |
.hooks.UserPromptSubmit = [(.hooks.UserPromptSubmit // [])[] | select(no_mnemon)] |
.hooks.Stop = [(.hooks.Stop // [])[] | select(no_mnemon)]
endef
export JQ_REMOVE_MNEMON

inject-hooks: ## Install Claude Code hooks for auto-recall/remember
	@mkdir -p $(HOOKS_DST)
	@cp $(HOOKS_SRC)/user_prompt.sh $(HOOKS_DST)/user_prompt.sh
	@cp $(HOOKS_SRC)/stop.sh $(HOOKS_DST)/stop.sh
	@chmod +x $(HOOKS_DST)/*.sh
	@if [ ! -f "$(CLAUDE_SETTINGS)" ]; then echo '{}' > "$(CLAUDE_SETTINGS)"; fi
	@jq "$$JQ_REMOVE_MNEMON" "$(CLAUDE_SETTINGS)" | \
	jq '.hooks.UserPromptSubmit += [{"hooks": [{"type": "command", "command": "$(HOOKS_DST)/user_prompt.sh"}]}]' | \
	jq '.hooks.Stop += [{"hooks": [{"type": "command", "command": "$(HOOKS_DST)/stop.sh"}]}]' \
		> "$(CLAUDE_SETTINGS).tmp" && mv "$(CLAUDE_SETTINGS).tmp" "$(CLAUDE_SETTINGS)"
	@echo "  Hooks → $(HOOKS_DST)/"
	@echo "  Config → $(CLAUDE_SETTINGS)"

eject-hooks: ## Remove Claude Code hooks
	@if [ -d "$(HOOKS_DST)" ]; then rm -rf "$(HOOKS_DST)"; echo "Removed: $(HOOKS_DST)"; fi
	@if [ -f "$(CLAUDE_SETTINGS)" ]; then \
		jq "$$JQ_REMOVE_MNEMON" "$(CLAUDE_SETTINGS)" > "$(CLAUDE_SETTINGS).tmp" && \
		mv "$(CLAUDE_SETTINGS).tmp" "$(CLAUDE_SETTINGS)"; \
		echo "Cleaned: $(CLAUDE_SETTINGS)"; \
	fi

# ── CLAUDE.md memory injection ─────────────────────────────────────

claude-inject: ## Inject memory guidance into ./CLAUDE.md
	@if grep -q 'mnemon:start' "$(CLAUDE_MD)" 2>/dev/null; then \
		echo "Already injected in $(CLAUDE_MD)"; \
	else \
		if [ -f "$(CLAUDE_MD)" ]; then \
			printf '\n' >> "$(CLAUDE_MD)"; \
		fi; \
		cat "$(CLAUDE_MEMORY)" >> "$(CLAUDE_MD)"; \
		echo "  Memory → $(CLAUDE_MD)"; \
	fi

claude-eject: ## Remove memory guidance from ./CLAUDE.md
	@if [ -f "$(CLAUDE_MD)" ] && grep -q 'mnemon:start' "$(CLAUDE_MD)"; then \
		sed '/<!-- mnemon:start -->/,/<!-- mnemon:end -->/d' "$(CLAUDE_MD)" > "$(CLAUDE_MD).tmp" && \
		mv "$(CLAUDE_MD).tmp" "$(CLAUDE_MD)"; \
		echo "Cleaned: $(CLAUDE_MD)"; \
	else \
		echo "No mnemon section in $(CLAUDE_MD)"; \
	fi

# ── Setup (one-command) ─────────────────────────────────────────────

setup: install inject inject-hooks ## Full setup: binary + skill + hooks (deprecated: use 'mnemon setup')
	@echo ""
	@echo "Setup complete:"
	@echo "  Binary → $(GOBIN)/$(BINARY)"
	@echo "  Skill  → $(SKILL_DST)/SKILL.md"
	@echo "  Hooks  → $(HOOKS_DST)/"
	@echo ""
	@echo "NOTE: 'make setup' is deprecated. Use 'mnemon setup' instead."
	@echo "Start a new Claude Code session to verify."

# ── Asset Sync ──────────────────────────────────────────────────────

ASSETS_DIR := internal/setup/assets/claude

sync-assets: ## Sync source-of-truth files into embedded assets
	@cp scripts/hooks/user_prompt.sh $(ASSETS_DIR)/user_prompt.sh
	@cp scripts/hooks/stop.sh $(ASSETS_DIR)/stop.sh
	@cp skills/mnemon/SKILL.md $(ASSETS_DIR)/SKILL.md
	@cp scripts/claude_memory.md $(ASSETS_DIR)/claude_memory.md
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
