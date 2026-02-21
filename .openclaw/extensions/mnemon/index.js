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
  // ── Remind (before_prompt_build) ──────────────────────────────
  // Per-message: inject recall results + remember reminder.
  // Equivalent to Claude Code's UserPromptSubmit hook.
  api.on("before_prompt_build", async (event) => {
    const query = extractQuery(event.prompt);
    const memories = recall(query);

    const parts = [];

    if (memories) {
      parts.push(`[mnemon] Relevant memories:\n${memories}`);
    }

    parts.push(
      "[mnemon] After responding, evaluate: does this exchange contain a user directive, reasoning conclusion, or durable state worth remembering? If yes, delegate to a sub-agent."
    );

    return { prependContext: parts.join("\n\n") };
  });

  // ── agent_end ─────────────────────────────────────────────────
  // Fire-and-forget: log session completion for diagnostics.
  api.on("agent_end", async () => {
    // Future: background remember evaluation or session summary.
    // Currently a no-op placeholder — remember is handled by the
    // LLM itself via the before_prompt_build reminder above.
  });
}
