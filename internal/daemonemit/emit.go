package daemonemit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

type Options struct {
	Root          string
	Topic         string
	Payload       map[string]any
	CorrelationID string
	CausedBy      string
	Loop          string
	Host          string
	Actor         string
	Source        string
	ProjectRoot   string
	Store         string
	Now           time.Time
}

type Event struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	TS            string         `json:"ts"`
	Type          string         `json:"type"`
	Loop          *string        `json:"loop"`
	Host          *string        `json:"host"`
	Actor         string         `json:"actor"`
	Source        string         `json:"source"`
	CorrelationID string         `json:"correlation_id"`
	CausedBy      *string        `json:"caused_by"`
	Payload       map[string]any `json:"payload"`
	ProjectRoot   string         `json:"project_root,omitempty"`
	Store         string         `json:"store,omitempty"`
}

func Emit(opts Options) (Event, string, error) {
	event, err := NewEvent(opts)
	if err != nil {
		return Event{}, "", err
	}
	path := EventLogPath(opts.Root)
	if err := appendEvent(path, event); err != nil {
		return Event{}, "", err
	}
	return event, path, nil
}

func NewEvent(opts Options) (Event, error) {
	if !eventTypePattern.MatchString(opts.Topic) {
		return Event{}, fmt.Errorf("event topic must be lower-case dot-separated")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload := opts.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	actor := opts.Actor
	if actor == "" {
		actor = "mnemon-manual"
	}
	if !allowedActor(actor) {
		return Event{}, fmt.Errorf("actor %q is not allowed", actor)
	}
	source := opts.Source
	if source == "" {
		source = "mnemon.event_emit"
	}
	correlationID := opts.CorrelationID
	if correlationID == "" {
		correlationID = "event:" + uuid.NewString()
	}
	return Event{
		SchemaVersion: 1,
		ID:            "evt_" + strings.ReplaceAll(opts.Topic, ".", "_") + "_" + now.UTC().Format("20060102T150405.000000000"),
		TS:            now.UTC().Format(time.RFC3339),
		Type:          opts.Topic,
		Loop:          optionalString(opts.Loop),
		Host:          optionalString(opts.Host),
		Actor:         actor,
		Source:        source,
		CorrelationID: correlationID,
		CausedBy:      optionalString(opts.CausedBy),
		Payload:       payload,
		ProjectRoot:   opts.ProjectRoot,
		Store:         opts.Store,
	}, nil
}

func EventLogPath(root string) string {
	if override := os.Getenv("MNEMON_HARNESS_EVENTLOG"); override != "" {
		if filepath.Ext(override) == ".jsonl" {
			return filepath.Clean(override)
		}
		return filepath.Join(override, "events.jsonl")
	}
	if root == "" {
		root = "."
	}
	return filepath.Join(filepath.Clean(root), ".mnemon", "events.jsonl")
}

func PayloadFromJSON(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	if payload == nil {
		return map[string]any{}, nil
	}
	return payload, nil
}

func appendEvent(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func allowedActor(value string) bool {
	switch value {
	case "user", "host-agent", "mnemon-manual", "mnemon-daemon", "host-runner", "reconciler", "projector", "validator":
		return true
	default:
		return false
	}
}
