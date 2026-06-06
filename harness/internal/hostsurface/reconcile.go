package hostsurface

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

type ReconcileResult struct {
	Host     string      `json:"host"`
	Status   string      `json:"status"`
	Items    []DriftItem `json:"items,omitempty"`
	Repaired []DriftItem `json:"repaired,omitempty"`
	EventID  string      `json:"event_id,omitempty"`
}

func RunCodexReconcile(ctx context.Context, opts CodexOptions) (ReconcileResult, error) {
	projector, loops, err := newCodexProjector("diff", opts)
	if err != nil {
		return ReconcileResult{}, err
	}
	items, err := collectCodexDrift(projector, loops)
	if err != nil {
		return ReconcileResult{}, err
	}
	result := ReconcileResult{
		Host:   "codex",
		Status: "noop",
		Items:  items,
	}
	eventType := "reconcile.noop"
	if len(items) > 0 {
		if err := RunCodexProjector(ctx, "install", opts); err != nil {
			return ReconcileResult{}, err
		}
		result.Status = "repaired"
		result.Repaired = append([]DriftItem(nil), items...)
		eventType = "projection.repaired"
	}
	eventID, err := appendReconcileEvent(projector.projectRoot, eventType, result, loops)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.EventID = eventID
	return result, nil
}

func collectCodexDrift(projector codexProjector, loops []string) ([]DriftItem, error) {
	var items []DriftItem
	for _, loopName := range loops {
		loop, err := declaration.LoadLoop(projector.declarationRoot, loopName)
		if err != nil {
			return nil, err
		}
		binding, err := declaration.LoadBinding(projector.declarationRoot, "codex", loopName)
		if err != nil {
			return nil, err
		}
		loopItems, err := projector.driftItems(loop, binding, false)
		if err != nil {
			return nil, fmt.Errorf("diff codex/%s: %w", loopName, err)
		}
		items = append(items, loopItems...)
	}
	return items, nil
}

func appendReconcileEvent(root, eventType string, result ReconcileResult, loops []string) (string, error) {
	store, err := eventlog.New(root)
	if err != nil {
		return "", err
	}
	nowTime := time.Now().UTC()
	now := nowTime.Truncate(time.Second).Format(time.RFC3339)
	eventID := reconcileEventID(eventType, nowTime)
	host := result.Host
	var loopPtr *string
	if len(loops) == 1 {
		loop := loops[0]
		loopPtr = &loop
	}
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            eventID,
		TS:            now,
		Type:          eventType,
		Loop:          loopPtr,
		Host:          &host,
		Actor:         "reconciler",
		Source:        "mnemon-harness.loop.reconcile",
		CorrelationID: eventID,
		Payload: map[string]any{
			"host":           result.Host,
			"status":         result.Status,
			"drift_count":    len(result.Items),
			"repaired_count": len(result.Repaired),
			"drift_items":    driftItemsRaw(result.Items),
		},
	}
	if err := store.Append(event); err != nil {
		return "", err
	}
	return eventID, nil
}

func driftItemsRaw(items []DriftItem) []map[string]any {
	raw := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw = append(raw, map[string]any{
			"host":    item.Host,
			"loop":    item.Loop,
			"action":  item.Action,
			"target":  item.Target,
			"detail":  item.Detail,
			"dry_run": item.DryRun,
		})
	}
	return raw
}

func reconcileEventID(eventType string, ts time.Time) string {
	return fmt.Sprintf("evt_%s_%d", strings.ReplaceAll(eventType, ".", "_"), ts.UnixNano())
}
