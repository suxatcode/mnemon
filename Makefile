# ──────────────────────────────────────────────────────────────────────
# Mnemon Makefile
# ──────────────────────────────────────────────────────────────────────

BINARY      := mnemon
GOBIN       := $(shell go env GOBIN)
ifeq ($(GOBIN),)
  GOBIN     := $(shell go env GOPATH)/bin
endif

CLAUDE_DIR   := $(HOME)/.claude
CLAUDE_MD    := $(CLAUDE_DIR)/CLAUDE.md
CLAUDE_SETTINGS := $(CLAUDE_DIR)/settings.json
SKILL_FILE   := memory.md
HOOK_SCRIPT  := $(GOBIN)/mnemon-hook

# Sentinel markers for injected skill block
SENTINEL_BEGIN := \#\# BEGIN mnemon-skill
SENTINEL_END   := \#\# END mnemon-skill

.PHONY: build install uninstall inject eject inject-hooks eject-hooks test clean help

# ── Build ────────────────────────────────────────────────────────────

build: ## Build the mnemon binary
	go build -o $(BINARY) .

# ── Install / Uninstall ─────────────────────────────────────────────

install: build ## Build and install mnemon to $GOBIN
	@mkdir -p $(GOBIN)
	cp $(BINARY) $(GOBIN)/$(BINARY)
	cp scripts/mnemon_hook.sh $(GOBIN)/mnemon-hook
	@echo "Installed: $(GOBIN)/$(BINARY)"
	@echo "Installed: $(GOBIN)/mnemon-hook"

uninstall: eject ## Remove mnemon binary, hook script, and eject all
	rm -f $(GOBIN)/$(BINARY)
	rm -f $(GOBIN)/mnemon-hook
	@echo "Removed: $(GOBIN)/$(BINARY)"
	@echo "Removed: $(GOBIN)/mnemon-hook"

# ── Skill injection (CLAUDE.md) ─────────────────────────────────────

inject-skill: ## Inject mnemon skill into global ~/.claude/CLAUDE.md
	@mkdir -p $(CLAUDE_DIR)
	@# Remove old block first (idempotent)
	@if [ -f "$(CLAUDE_MD)" ] && grep -q "$(SENTINEL_BEGIN)" "$(CLAUDE_MD)"; then \
		sed '/$(SENTINEL_BEGIN)/,/$(SENTINEL_END)/d' "$(CLAUDE_MD)" > "$(CLAUDE_MD).tmp" && \
		mv "$(CLAUDE_MD).tmp" "$(CLAUDE_MD)"; \
	fi
	@# Append new block
	@echo "" >> "$(CLAUDE_MD)"
	@echo "## BEGIN mnemon-skill" >> "$(CLAUDE_MD)"
	@cat $(SKILL_FILE) >> "$(CLAUDE_MD)"
	@echo "" >> "$(CLAUDE_MD)"
	@echo "## END mnemon-skill" >> "$(CLAUDE_MD)"
	@echo "  Skill  → $(CLAUDE_MD)"

# ── Hook injection (settings.json) ──────────────────────────────────

inject-hooks: ## Inject mnemon hooks into ~/.claude/settings.json
	@mkdir -p $(CLAUDE_DIR)
	@if [ ! -f "$(CLAUDE_SETTINGS)" ]; then echo '{}' > "$(CLAUDE_SETTINGS)"; fi
	@# Merge hook config via jq (idempotent: replaces existing mnemon hooks)
	@jq --arg hook "$(HOOK_SCRIPT)" \
		'.hooks.UserPromptSubmit = (.hooks.UserPromptSubmit // [] | map(select(.matcher != "mnemon"))) + [{"matcher": "mnemon", "hooks": [{"type": "command", "command": $$hook}]}]' \
		"$(CLAUDE_SETTINGS)" > "$(CLAUDE_SETTINGS).tmp" && \
		mv "$(CLAUDE_SETTINGS).tmp" "$(CLAUDE_SETTINGS)"
	@echo "  Hooks  → $(CLAUDE_SETTINGS)"

eject-hooks: ## Remove mnemon hooks from ~/.claude/settings.json
	@if [ -f "$(CLAUDE_SETTINGS)" ] && jq -e '.hooks.UserPromptSubmit' "$(CLAUDE_SETTINGS)" >/dev/null 2>&1; then \
		jq '.hooks.UserPromptSubmit = [.hooks.UserPromptSubmit[] | select(.matcher != "mnemon")] | if .hooks.UserPromptSubmit == [] then del(.hooks.UserPromptSubmit) else . end | if .hooks == {} then del(.hooks) else . end' \
			"$(CLAUDE_SETTINGS)" > "$(CLAUDE_SETTINGS).tmp" && \
		mv "$(CLAUDE_SETTINGS).tmp" "$(CLAUDE_SETTINGS)"; \
		echo "Ejected mnemon hooks from $(CLAUDE_SETTINGS)"; \
	else \
		echo "No mnemon hooks found in $(CLAUDE_SETTINGS)"; \
	fi

# ── Combined inject / eject ─────────────────────────────────────────

inject: inject-skill inject-hooks ## Inject skill + hooks
	@echo "Inject complete."

eject: eject-hooks ## Eject skill + hooks
	@if [ -f "$(CLAUDE_MD)" ] && grep -q "$(SENTINEL_BEGIN)" "$(CLAUDE_MD)"; then \
		sed '/$(SENTINEL_BEGIN)/,/$(SENTINEL_END)/d' "$(CLAUDE_MD)" > "$(CLAUDE_MD).tmp" && \
		mv "$(CLAUDE_MD).tmp" "$(CLAUDE_MD)"; \
		echo "Ejected mnemon skill from $(CLAUDE_MD)"; \
	else \
		echo "No mnemon skill block found in $(CLAUDE_MD)"; \
	fi

# ── Setup (one-command) ─────────────────────────────────────────────

setup: install inject ## Install binary + inject skill + hooks (full setup)
	@echo ""
	@echo "Setup complete:"
	@echo "  Binary:  $(GOBIN)/$(BINARY)"
	@echo "  Hook:    $(GOBIN)/mnemon-hook"
	@echo "  Skill:   $(CLAUDE_MD)"
	@echo "  Hooks:   $(CLAUDE_SETTINGS)"
	@echo ""
	@echo "Start a new Claude Code session to verify."

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
