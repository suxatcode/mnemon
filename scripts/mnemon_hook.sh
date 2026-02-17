#!/usr/bin/env bash
# mnemon UserPromptSubmit hook
# Injects a reminder into Claude's context on each user message.
# Claude sees this text and decides whether to act on it.

cat <<'REMINDER'
[mnemon] Before responding, check: did the previous exchange produce any of these?
  - User preference or workflow choice → mnemon remember --cat preference
  - Architectural/design decision → mnemon remember --cat decision
  - Bug fix or lesson learned → mnemon remember --cat insight
  - Key fact (technical, research, market data, any tracked topic) → mnemon remember --cat fact
  - Task completed → mnemon remember --cat context
  - User showed ongoing interest in a topic → mnemon remember --cat preference, then save related findings
When in doubt, save — a redundant diff costs nothing, lost context costs a re-search.
If yes, run `mnemon diff "<fact>"` then `mnemon remember` before continuing.
If nothing worth saving, proceed normally.
REMINDER
