import { execSync } from "child_process";

/**
 * Extract a focused recall query from the user's prompt.
 * Strips filler, keeps keywords — mnemon recall works best with
 * keyword-rich queries rather than raw user prompts.
 */
function extractQuery(prompt) {
  if (!prompt || typeof prompt !== "string") return "";
  // Take the first 200 chars, collapse whitespace
  return prompt.slice(0, 200).replace(/\s+/g, " ").trim();
}

/**
 * Run mnemon recall and return results, or null on failure.
 */
function recall(query) {
  if (!query) return null;
  try {
    const result = execSync(
      `mnemon recall "${query.replace(/"/g, '\\"')}" --limit 5`,
      { timeout: 5000, encoding: "utf-8" }
    );
    if (result && !result.includes("no insights found")) {
      return result.trim();
    }
  } catch {
    // mnemon not available or recall failed — silent
  }
  return null;
}

export default function register(api) {
  const cfg = api.config?.plugins?.entries?.mnemon?.config ?? {};
  const remind  = cfg.remind  !== false; // default on
  const nudge   = cfg.nudge   !== false; // default on
  const compact = cfg.compact === true;  // default off

  // ── Remind (before_prompt_build) ──────────────────────────────
  // Per-message: inject recall results + remember reminder.
  // Equivalent to Claude Code's UserPromptSubmit hook.
  if (remind) {
    api.on("before_prompt_build", async (event) => {
      const query = extractQuery(event.prompt);
      const memories = recall(query);

      const parts = [];

      if (memories) {
        parts.push(`[mnemon] Relevant memories:\n${memories}`);
      }

      parts.push(
        "[mnemon] Evaluate: recall needed? After responding, evaluate: remember needed?"
      );

      return { prependContext: parts.join("\n\n") };
    });
  }

  // ── Nudge (agent_end) ─────────────────────────────────────────
  // After agent replies: nudge to consider a remember sub-agent.
  if (nudge) {
    api.on("agent_end", async (event) => {
      const lastMsg = event?.lastAssistantMessage ?? "";
      // Stay silent if agent already mentioned memory operations
      if (/mnemon remember|sub-agent.*remember|Stored.*imp=/i.test(lastMsg)) {
        return;
      }
      return {
        nudge: "[mnemon] Consider: does this exchange warrant a remember sub-agent?",
      };
    });
  }

  // ── Compact (before_compaction) ───────────────────────────────
  // Before context compaction: prompt to save key insights.
  if (compact) {
    api.on("before_compaction", async () => {
      return {
        prependContext:
          "[mnemon] Context compaction starting. Review this session and remember the most valuable insights (up to 5) before context is compressed. Delegate to Task sub-agents now.",
      };
    });
  }
}
