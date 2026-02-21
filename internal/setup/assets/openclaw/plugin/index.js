import { execSync } from "child_process";
import { existsSync, readFileSync, unlinkSync, writeFileSync } from "fs";
import { homedir } from "os";
import { join } from "path";

const COMPACT_FLAG = join(homedir(), ".mnemon", ".compact-pending");

/**
 * Extract a focused recall query from the user's prompt.
 * Strips filler, keeps keywords — mnemon recall works best with
 * keyword-rich queries rather than raw user prompts.
 */
function extractQuery(prompt) {
  if (!prompt || typeof prompt !== "string") return "";
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
  // api.pluginConfig holds plugins.entries.mnemon.config from openclaw.json
  const cfg = api.pluginConfig ?? {};
  const remind  = cfg.remind  !== false; // default on
  const nudge   = cfg.nudge   !== false; // default on
  const compact = cfg.compact === true;  // default off

  // ── before_compaction (void) ──────────────────────────────────
  // Cannot inject context directly — write a flag file instead.
  // before_prompt_build will pick it up on the next turn.
  if (compact) {
    api.on("before_compaction", async () => {
      try {
        writeFileSync(COMPACT_FLAG, String(Date.now()), "utf-8");
      } catch {
        // ignore write failure
      }
    });
  }

  // ── before_prompt_build ───────────────────────────────────────
  // Handles: remind (recall + remember reminder) + nudge reminder
  // + compact flag consumption.
  if (remind || nudge || compact) {
    api.on("before_prompt_build", async (event) => {
      const parts = [];

      // Compact flag: was compaction triggered since last turn?
      if (compact && existsSync(COMPACT_FLAG)) {
        try { unlinkSync(COMPACT_FLAG); } catch { /* ignore */ }
        parts.push(
          "[mnemon] Context compaction starting. Review this session and remember the most valuable insights (up to 5) before context is compressed. Delegate to Task sub-agents now."
        );
      }

      if (remind) {
        const query = extractQuery(event.prompt);
        const memories = recall(query);
        if (memories) {
          parts.push(`[mnemon] Relevant memories:\n${memories}`);
        }
        parts.push(
          "[mnemon] Evaluate: recall needed? After responding, evaluate: remember needed?"
        );
      }

      if (nudge) {
        parts.push(
          "[mnemon] Consider: does this exchange warrant a remember sub-agent?"
        );
      }

      if (parts.length === 0) return;
      return { prependContext: parts.join("\n\n") };
    });
  }

  // ── agent_end (void — no return value supported) ──────────────
  // Placeholder for future diagnostics; memory evaluation is handled
  // by the LLM itself via the before_prompt_build nudge above.
  api.on("agent_end", async () => {
    // no-op
  });
}
