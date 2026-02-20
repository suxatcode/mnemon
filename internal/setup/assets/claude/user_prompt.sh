#!/bin/bash
# mnemon UserPromptSubmit hook
# Auto-recall relevant memories for each user message.

INPUT=$(cat)
PROMPT=$(echo "$INPUT" | jq -r '.prompt // empty' 2>/dev/null)

# Skip empty or short prompts
if [ -z "$PROMPT" ] || [ ${#PROMPT} -lt 5 ]; then
  exit 0
fi

RESULT=$(mnemon recall "$PROMPT" --limit 5 2>/dev/null)
if [ -n "$RESULT" ] && ! echo "$RESULT" | grep -qi "no insights found"; then
  echo "[Past memory] $RESULT"
fi
