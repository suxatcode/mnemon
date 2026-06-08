// Package assets embeds the harness's built-in loop/host/binding manifests and their projected asset
// files (GUIDE, hooks, skills, subagents). Embedding makes the mnemon-harness binary self-contained:
// setup/refresh/validate read from FS, never from an on-disk source tree. Embedded keys carry NO
// "harness/" prefix and use forward slashes ("loops/<loop>/loop.json").
package assets

import "embed"

//go:embed loops hosts bindings
var FS embed.FS
