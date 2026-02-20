package assets

import "embed"

//go:embed claude/user_prompt.sh
var ClaudeUserPromptHook []byte

//go:embed claude/stop.sh
var ClaudeStopHook []byte

//go:embed claude/SKILL.md
var ClaudeSkill []byte

//go:embed claude/claude_memory.md
var ClaudeMemoryMD []byte

//go:embed openclaw/index.ts
var OpenClawPlugin []byte

//go:embed openclaw/openclaw.plugin.json
var OpenClawManifest []byte

//go:embed openclaw/package.json
var OpenClawPackageJSON []byte

//go:embed openclaw/openclaw_memory.md
var OpenClawMemoryMD []byte

// All returns the embedded filesystem for inspection.
//
//go:embed claude openclaw
var All embed.FS
