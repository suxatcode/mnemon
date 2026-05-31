package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTranscriptReportBuildsPythonCompatibleFields(t *testing.T) {
	transcript := strings.NewReader(`{"direction":"client","payload":{"id":1,"method":"initialize","params":{"clientInfo":{"name":"mnemon"}}}}
{"direction":"server","payload":{"id":1,"result":{"protocolVersion":"2026-05-27"}}}
{"direction":"client","payload":{"method":"initialized","params":{}}}
{"direction":"client","payload":{"id":2,"method":"skills/list","params":{"cwds":["/tmp/workspace"],"forceReload":true}}}
{"direction":"server","payload":{"id":2,"result":{"skills":[{"name":"memory-set"},{"name":"memory-get"},{"name":"memory-set"}]}}}
{"direction":"client","payload":{"id":3,"method":"thread/start","params":{"cwd":"/tmp/workspace"}}}
{"direction":"server","payload":{"id":3,"result":{"thread":{"id":"thread-abc"}}}}
{"direction":"server","payload":{"method":"session/configured","params":{"message":"ready"}}}
{"direction":"client","payload":{"id":4,"method":"turn/start","params":{"threadId":"thread-abc","input":[{"type":"text","text":"Recall the app-server decision."}],"cwd":"/tmp/workspace"}}}
{"direction":"server","payload":{"id":4,"result":{}}}
{"direction":"server","payload":{"method":"codex/event","params":{"event":{"type":"commandExecution","command":"mnemon recall app-server"}}}}
{"direction":"server","payload":{"method":"codex/event","params":{"event":{"type":"agentMessage","phase":"final_answer","text":"Use the Codex app-server decision."}}}}
{"direction":"server","payload":{"method":"turn/completed","params":{"turnId":"turn-1"}}}
`)

	report, err := ExtractTranscriptReport(transcript)
	if err != nil {
		t.Fatalf("ExtractTranscriptReport returned error: %v", err)
	}
	if report.Initialize["protocolVersion"] != "2026-05-27" {
		t.Fatalf("unexpected initialize result: %#v", report.Initialize)
	}
	if strings.Join(report.SkillNames, ",") != "memory-get,memory-set" {
		t.Fatalf("unexpected skill names: %#v", report.SkillNames)
	}
	if report.ThreadID != "thread-abc" {
		t.Fatalf("unexpected thread id: %s", report.ThreadID)
	}
	if len(report.Turns) != 1 {
		t.Fatalf("expected one turn: %#v", report.Turns)
	}
	if report.Turns[0].Prompt != "Recall the app-server decision." {
		t.Fatalf("unexpected prompt: %#v", report.Turns[0])
	}
	if report.Turns[0].NotificationCount != 3 {
		t.Fatalf("unexpected notification count: %#v", report.Turns[0])
	}
	if report.TurnCompleted == nil || report.Turns[0].TurnCompleted == nil {
		t.Fatalf("expected turn completion notification: %#v", report.Turns[0])
	}
	if len(report.Notifications) != 4 {
		t.Fatalf("unexpected notifications: %#v", report.Notifications)
	}
	if strings.Join(report.NotificationMethods, ",") != "codex/event,session/configured,turn/completed" {
		t.Fatalf("unexpected notification methods: %#v", report.NotificationMethods)
	}
	if !strings.Contains(report.NotificationText, "mnemon recall app-server") || !strings.Contains(report.NotificationText, "Use the Codex app-server decision.") {
		t.Fatalf("unexpected notification text: %s", report.NotificationText)
	}
	if !strings.Contains(report.CommandText, "mnemon recall app-server") || strings.Contains(report.CommandText, "final_answer") {
		t.Fatalf("unexpected command text: %s", report.CommandText)
	}
	if report.FinalAnswerText != "Use the Codex app-server decision." {
		t.Fatalf("unexpected final answer text: %s", report.FinalAnswerText)
	}

	reportMap := report.ReportMap()
	if reportMap["command_text"] != report.CommandText || reportMap["final_answer_text"] != report.FinalAnswerText {
		t.Fatalf("report map does not expose assertion text fields: %#v", reportMap)
	}
}

func TestLoadRunTranscriptReportFindsTranscriptArtifact(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".mnemon/harness/reports/runner/run-001-codex-app-server-semantic-run.json", `{
  "schema_version": 1,
  "kind": "CodexAppServerSemanticRunReport",
  "run_id": "run-001",
  "runner_id": "codex-app-server",
  "job_id": "eval_default_memory",
  "job_spec": "eval.memory",
  "loop": "eval",
  "status": "ready",
  "message": "ok",
  "artifact_refs": [
    {"id": "artifact:jsonrpc-transcript", "kind": "transcript", "uri": ".mnemon/harness/runs/codex-app-server/run-001/artifacts/jsonrpc-transcript.jsonl", "media_type": "application/jsonl", "privacy": "project"}
  ]
}`)
	writeFile(t, root, ".mnemon/harness/runs/codex-app-server/run-001/artifacts/jsonrpc-transcript.jsonl", `{"direction":"client","payload":{"id":1,"method":"thread/start","params":{}}}
{"direction":"server","payload":{"id":1,"result":{"thread":{"id":"thread-from-artifact"}}}}
`)

	report, err := LoadRunTranscriptReport(root, "run-001")
	if err != nil {
		t.Fatalf("LoadRunTranscriptReport returned error: %v", err)
	}
	if report.ThreadID != "thread-from-artifact" {
		t.Fatalf("unexpected transcript report: %#v", report)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
