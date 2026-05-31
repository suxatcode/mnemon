package ui

import (
	"encoding/json"
	"strings"
)

// prettyJSON re-indents a raw JSON string and clips each line to width w. Falls
// back to the raw string (clipped) when it does not parse.
func prettyJSON(raw string, w int) string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return clipLines(raw, w)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return clipLines(raw, w)
	}
	return clipLines(string(b), w)
}

// prettyMap renders a map as indented JSON, clipped to width w.
func prettyMap(m map[string]any, w int) string {
	if len(m) == 0 {
		return "—"
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "—"
	}
	return clipLines(string(b), w)
}

func clipLines(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = truncPlain(l, w)
	}
	return strings.Join(lines, "\n")
}
