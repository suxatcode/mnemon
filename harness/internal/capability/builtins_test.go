package capability

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuiltinsLoadFromEmbeddedSpecs(t *testing.T) {
	for _, id := range []string{"memory", "skill", "note"} {
		cap, ok := Builtins[id]
		if !ok {
			t.Fatalf("builtin %q must load from assets/capabilities", id)
		}
		if cap.Decode == nil || cap.Header == nil {
			t.Fatalf("builtin %q must carry compiled decode/header", id)
		}
	}
}

// loadBuiltins 的错误路径(嵌入物走 panic;外部目录——阶段五——走这些 error):
// 坏 JSON、FromSpec 失败、跨 spec 重名/重事件类型。
func TestLoadBuiltinsErrorPaths(t *testing.T) {
	good := `{"schema_version":1,"name":"note","observed_type":"note.write_candidate.observed",
"proposed_type":"note.write.proposed","resource_kind":"note","items_field":"items",
"fields":[{"name":"text","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# Notes","field":"text"}}}}`
	dupName := strings.Replace(good, `"observed_type":"note.write_candidate.observed"`,
		`"observed_type":"other.observed"`, 1)
	dupName = strings.Replace(dupName, `"proposed_type":"note.write.proposed"`,
		`"proposed_type":"other.proposed"`, 1)

	cases := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{"malformed json", map[string]string{"bad.json": `{nope`}, "parse capability spec"},
		{"fromspec failure", map[string]string{"bad.json": `{"schema_version":1,"name":"x"}`}, "compile capability spec"},
		{"duplicate name", map[string]string{"a.json": good, "b.json": dupName}, "duplicate capability name"},
		{"duplicate observed type", map[string]string{"a.json": good, "b.json": strings.Replace(good, `"name":"note"`, `"name":"memo"`, 1)}, "already claimed"},
	}
	for _, c := range cases {
		m := fstest.MapFS{}
		for f, body := range c.files {
			m["capabilities/"+f] = &fstest.MapFile{Data: []byte(body)}
		}
		if _, err := loadBuiltins(m); err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Fatalf("%s: want error containing %q, got %v", c.name, c.wantErr, err)
		}
	}
}
