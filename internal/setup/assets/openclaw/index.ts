import { execSync } from "child_process";

const TIMEOUT = 5000; // 5s

interface PluginAPI {
  inject(text: string): void;
}

interface PluginHooks {
  onMessageReceived?(api: PluginAPI, message: string): void;
  onAgentEnd?(api: PluginAPI, response: string): void;
  onSessionStart?(api: PluginAPI): void;
}

function run(cmd: string): string {
  try {
    return execSync(cmd, { timeout: TIMEOUT, encoding: "utf-8" }).trim();
  } catch {
    return "";
  }
}

function escapeShell(s: string): string {
  return s.replace(/'/g, "'\\''");
}

const mnemonPlugin: PluginHooks = {
  onMessageReceived(api: PluginAPI, message: string) {
    if (!message || message.length < 5) return;
    const result = run(`mnemon recall '${escapeShell(message)}' --limit 5`);
    if (result && !/no insights found/i.test(result)) {
      api.inject(`[Past memory] ${result}`);
    }
  },

  onAgentEnd(api: PluginAPI, response: string) {
    if (/mnemon remember|sub-agent.*remember|Stored.*imp=/i.test(response)) return;
    api.inject(
      "[mnemon] Consider: does this exchange warrant storing a memory?",
    );
  },

  onSessionStart(api: PluginAPI) {
    const result = run("mnemon status");
    if (result) {
      api.inject(`[mnemon] Memory active. ${result}`);
    }
  },
};

export default mnemonPlugin;
