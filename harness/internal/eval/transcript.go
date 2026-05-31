package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type TranscriptReport struct {
	Initialize          map[string]any   `json:"initialize,omitempty"`
	SkillNames          []string         `json:"skill_names,omitempty"`
	ThreadID            string           `json:"thread_id,omitempty"`
	Turns               []TranscriptTurn `json:"turns,omitempty"`
	TurnCompleted       map[string]any   `json:"turn_completed,omitempty"`
	Notifications       []map[string]any `json:"notifications,omitempty"`
	NotificationMethods []string         `json:"notification_methods,omitempty"`
	NotificationText    string           `json:"notification_text"`
	CommandText         string           `json:"command_text"`
	FinalAnswerText     string           `json:"final_answer_text"`
}

type TranscriptTurn struct {
	Index             int            `json:"index"`
	Prompt            string         `json:"prompt,omitempty"`
	TurnCompleted     map[string]any `json:"turn_completed,omitempty"`
	NotificationCount int            `json:"notification_count,omitempty"`
}

func LoadRunTranscriptReport(root, runID string) (TranscriptReport, error) {
	runReport, err := LoadRunReport(root, runID)
	if err != nil {
		return TranscriptReport{}, err
	}
	path, err := runTranscriptPath(root, runReport)
	if err != nil {
		return TranscriptReport{}, err
	}
	return LoadTranscriptReport(path)
}

func LoadTranscriptReport(path string) (TranscriptReport, error) {
	file, err := os.Open(path)
	if err != nil {
		return TranscriptReport{}, fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer file.Close()
	return ExtractTranscriptReport(file)
}

func ExtractTranscriptReport(input io.Reader) (TranscriptReport, error) {
	extractor := transcriptExtractor{
		pendingRequests: map[string]transcriptRequest{},
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record transcriptRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return TranscriptReport{}, fmt.Errorf("parse transcript line %d: %w", lineNumber, err)
		}
		payload, err := decodeJSONMap(record.Payload)
		if err != nil {
			return TranscriptReport{}, fmt.Errorf("parse transcript payload line %d: %w", lineNumber, err)
		}
		extractor.observe(record.Direction, payload)
	}
	if err := scanner.Err(); err != nil {
		return TranscriptReport{}, fmt.Errorf("read transcript: %w", err)
	}
	extractor.finish()
	return extractor.report, nil
}

func (report TranscriptReport) ReportMap() map[string]any {
	out := map[string]any{
		"skill_names":          nonNilStrings(report.SkillNames),
		"thread_id":            report.ThreadID,
		"turns":                transcriptTurnsAsMaps(report.Turns),
		"notifications":        nonNilMaps(report.Notifications),
		"notification_methods": nonNilStrings(report.NotificationMethods),
		"notification_text":    report.NotificationText,
		"command_text":         report.CommandText,
		"final_answer_text":    report.FinalAnswerText,
	}
	if report.Initialize != nil {
		out["initialize"] = report.Initialize
	}
	if report.TurnCompleted != nil {
		out["turn_completed"] = report.TurnCompleted
	}
	return out
}

type transcriptRecord struct {
	Direction string          `json:"direction"`
	Payload   json.RawMessage `json:"payload"`
}

type transcriptRequest struct {
	Method string
	Params map[string]any
}

type transcriptExtractor struct {
	report          TranscriptReport
	pendingRequests map[string]transcriptRequest
	openTurns       []int
}

func (extractor *transcriptExtractor) observe(direction string, payload map[string]any) {
	switch direction {
	case "client":
		extractor.observeClient(payload)
	case "server":
		extractor.observeServer(payload)
	}
}

func (extractor *transcriptExtractor) observeClient(payload map[string]any) {
	method := stringField(payload, "method")
	if method == "" {
		return
	}
	id := rpcIDKey(payload["id"])
	if id == "" {
		return
	}
	params := mapField(payload, "params")
	extractor.pendingRequests[id] = transcriptRequest{
		Method: method,
		Params: params,
	}
	if method == "turn/start" {
		if extractor.report.ThreadID == "" {
			extractor.report.ThreadID = stringField(params, "threadId")
		}
		turnIndex := len(extractor.report.Turns)
		extractor.report.Turns = append(extractor.report.Turns, TranscriptTurn{
			Index:             turnIndex + 1,
			Prompt:            turnStartPrompt(params),
			NotificationCount: -len(extractor.report.Notifications),
		})
		extractor.openTurns = append(extractor.openTurns, turnIndex)
	}
}

func (extractor *transcriptExtractor) observeServer(payload map[string]any) {
	id := rpcIDKey(payload["id"])
	if id == "" {
		extractor.observeNotification(payload)
		return
	}
	request, ok := extractor.pendingRequests[id]
	if !ok {
		return
	}
	defer delete(extractor.pendingRequests, id)

	result := mapField(payload, "result")
	switch request.Method {
	case "initialize":
		extractor.report.Initialize = result
	case "skills/list":
		extractor.report.SkillNames = collectSkillNames(result)
	case "thread/start":
		if threadID := nestedStringField(result, "thread", "id"); threadID != "" {
			extractor.report.ThreadID = threadID
		}
	}
}

func (extractor *transcriptExtractor) observeNotification(payload map[string]any) {
	extractor.report.Notifications = append(extractor.report.Notifications, payload)
	if stringField(payload, "method") != "turn/completed" {
		return
	}
	extractor.report.TurnCompleted = payload
	if len(extractor.openTurns) == 0 {
		return
	}
	turnIndex := extractor.openTurns[0]
	extractor.openTurns = extractor.openTurns[1:]
	turn := &extractor.report.Turns[turnIndex]
	turn.TurnCompleted = payload
	turn.NotificationCount += len(extractor.report.Notifications)
}

func (extractor *transcriptExtractor) finish() {
	for _, turnIndex := range extractor.openTurns {
		turn := &extractor.report.Turns[turnIndex]
		if turn.NotificationCount < 0 {
			turn.NotificationCount += len(extractor.report.Notifications)
		}
	}
	extractor.report.NotificationMethods = notificationMethods(extractor.report.Notifications)
	extractor.report.NotificationText = combinedText(extractor.report.Notifications)
	extractor.report.CommandText = combinedText(commandNotifications(extractor.report.Notifications))
	extractor.report.FinalAnswerText = finalAnswerText(extractor.report.Notifications)
}

func runTranscriptPath(root string, report RunReport) (string, error) {
	for _, ref := range report.ArtifactRefs {
		if ref.Kind == "transcript" || strings.Contains(ref.URI, "jsonrpc-transcript") {
			return artifactPath(root, ref.URI), nil
		}
	}
	return "", fmt.Errorf("run report %s has no transcript artifact", report.RunID)
}

func artifactPath(root, uri string) string {
	if filepath.IsAbs(uri) {
		return filepath.Clean(uri)
	}
	return filepath.Join(cleanRoot(root), filepath.FromSlash(uri))
}

func decodeJSONMap(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func rpcIDKey(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case json.Number:
		return typed.String()
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func mapField(value map[string]any, key string) map[string]any {
	child, ok := value[key].(map[string]any)
	if !ok {
		return nil
	}
	return child
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func nestedStringField(value map[string]any, parent, key string) string {
	parentValue := mapField(value, parent)
	if parentValue == nil {
		return ""
	}
	return stringField(parentValue, key)
}

func turnStartPrompt(params map[string]any) string {
	input, ok := params["input"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if text := stringField(item, "text"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func collectSkillNames(value any) []string {
	seen := map[string]bool{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if name := stringField(typed, "name"); name != "" {
				seen[name] = true
			}
			for _, key := range sortedMapKeys(typed) {
				walk(typed[key])
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case []map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func notificationMethods(notifications []map[string]any) []string {
	seen := map[string]bool{}
	for _, item := range notifications {
		if method := stringField(item, "method"); method != "" {
			seen[method] = true
		}
	}
	methods := make([]string, 0, len(seen))
	for method := range seen {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func commandNotifications(notifications []map[string]any) []map[string]any {
	var matches []map[string]any
	for _, item := range notifications {
		if strings.Contains(combinedText(item), "commandExecution") {
			matches = append(matches, item)
		}
	}
	return matches
}

func finalAnswerText(notifications []map[string]any) string {
	matches := collectMatchingObjects(notifications, func(item map[string]any) bool {
		return stringField(item, "type") == "agentMessage" &&
			stringField(item, "phase") == "final_answer" &&
			stringField(item, "text") != ""
	})
	texts := make([]string, 0, len(matches))
	for _, item := range matches {
		texts = append(texts, stringField(item, "text"))
	}
	return strings.Join(texts, "\n")
}

func combinedText(value any) string {
	return strings.Join(allStrings(value), "\n")
}

func allStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case map[string]any:
		var out []string
		for _, key := range sortedMapKeys(typed) {
			out = append(out, allStrings(typed[key])...)
		}
		return out
	case []any:
		var out []string
		for _, item := range typed {
			out = append(out, allStrings(item)...)
		}
		return out
	case []map[string]any:
		var out []string
		for _, item := range typed {
			out = append(out, allStrings(item)...)
		}
		return out
	default:
		return nil
	}
}

func collectMatchingObjects(value any, predicate func(map[string]any) bool) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		var matches []map[string]any
		if predicate(typed) {
			matches = append(matches, typed)
		}
		for _, key := range sortedMapKeys(typed) {
			matches = append(matches, collectMatchingObjects(typed[key], predicate)...)
		}
		return matches
	case []any:
		var matches []map[string]any
		for _, item := range typed {
			matches = append(matches, collectMatchingObjects(item, predicate)...)
		}
		return matches
	case []map[string]any:
		var matches []map[string]any
		for _, item := range typed {
			matches = append(matches, collectMatchingObjects(item, predicate)...)
		}
		return matches
	default:
		return nil
	}
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func transcriptTurnsAsMaps(turns []TranscriptTurn) []map[string]any {
	out := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		item := map[string]any{
			"index":              turn.Index,
			"prompt":             turn.Prompt,
			"notification_count": turn.NotificationCount,
		}
		if turn.TurnCompleted != nil {
			item["turn_completed"] = turn.TurnCompleted
		}
		out = append(out, item)
	}
	return out
}

func nonNilMaps(value []map[string]any) []map[string]any {
	if value == nil {
		return []map[string]any{}
	}
	return value
}

func nonNilStrings(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}
