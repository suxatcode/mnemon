package app

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/profile"
)

type ProfileEntryInput struct {
	ProfileID         string
	EntryID           string
	Type              string
	Summary           string
	Content           string
	Evidence          []string
	ProjectionTargets []string
}

func (h *Harness) ProfileEntryAdd(out io.Writer, in ProfileEntryInput) error {
	store, err := profile.New(h.root)
	if err != nil {
		return err
	}
	evidence, err := parseProfileEvidence(in.Evidence)
	if err != nil {
		return err
	}
	targets, err := parseProfileProjectionTargets(in.ProjectionTargets)
	if err != nil {
		return err
	}
	// Govern the direct CLI write through the kernel BEFORE the host AddEntry, so `profile
	// entry add` is NOT a second canonical writer that bypasses the rule pre-gate (P2
	// adversarial fix — D1 single writer). Resolve the canonical entry id ONCE and feed it to
	// BOTH the kernel write and AddEntry; a fresh applyID (not deduped) lets the kernel rule
	// pre-gate deny a duplicate entry id rather than silently dedup it.
	now := time.Now().UTC()
	entryID := profile.ResolveEntryID(in.EntryID, in.Type, in.Summary, now)
	profileID := profile.NormalizeProfileID(in.ProfileID)
	engine, err := h.coreEngine()
	if err != nil {
		return err
	}
	res, err := engine.AdmitCreate(uuid.NewString(), "memory", profileID+"/"+entryID, map[string]any{
		"content":    in.Content,
		"summary":    in.Summary,
		"entry_type": in.Type,
		"profile_id": profileID,
		"entry_id":   entryID,
	})
	if err != nil {
		return fmt.Errorf("lower profile entry to kernel: %w", err)
	}
	if !res.Accepted {
		return fmt.Errorf("kernel denied profile entry %q: %s", entryID, res.Reason)
	}
	prof, entry, err := store.AddEntry(profile.AddEntryOptions{
		ProfileID:         in.ProfileID,
		EntryID:           entryID,
		Type:              in.Type,
		Summary:           in.Summary,
		Content:           in.Content,
		Evidence:          evidence,
		ProjectionTargets: targets,
		Now:               now,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "recorded profile entry %s in %s\n", entry.ID, profile.ProfileRef(prof.ID))
	return nil
}

func (h *Harness) ProfileShow(out io.Writer, profileID, host, loop, format string) error {
	store, err := profile.New(h.root)
	if err != nil {
		return err
	}
	prof, err := store.Load(profileID)
	if err != nil {
		return err
	}
	prof = store.FilterEntries(prof, host, loop)
	if format == "json" {
		return writeJSON(out, prof)
	}
	if format != "" && format != "text" {
		return fmt.Errorf("unsupported --format %q", format)
	}
	writeProfileText(out, prof, host, loop)
	return nil
}

func parseProfileEvidence(values []string) ([]profile.EvidenceRef, error) {
	result := make([]profile.EvidenceRef, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 3)
		if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("evidence %q must be type=ref or type=ref=summary", value)
		}
		ref := profile.EvidenceRef{
			Type: strings.TrimSpace(parts[0]),
			Ref:  strings.TrimSpace(parts[1]),
		}
		if len(parts) == 3 {
			ref.Summary = strings.TrimSpace(parts[2])
		}
		result = append(result, ref)
	}
	return result, nil
}

func parseProfileProjectionTargets(values []string) ([]profile.ProjectionTarget, error) {
	result := make([]profile.ProjectionTarget, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "/", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("project-to %q must be host/loop", value)
		}
		result = append(result, profile.ProjectionTarget{
			Host: strings.TrimSpace(parts[0]),
			Loop: strings.TrimSpace(parts[1]),
		})
	}
	return result, nil
}

func writeProfileText(out io.Writer, prof profile.Profile, host, loop string) {
	fmt.Fprintf(out, "profile %s: %s\n", prof.ID, prof.ScopeType)
	if strings.TrimSpace(host) != "" || strings.TrimSpace(loop) != "" {
		fmt.Fprintf(out, "filter: host=%s loop=%s\n", strings.TrimSpace(host), strings.TrimSpace(loop))
	}
	fmt.Fprintf(out, "entries: %d\n", len(prof.Entries))
	for _, entry := range prof.Entries {
		fmt.Fprintf(out, "- %s [%s] %s\n", entry.ID, entry.Type, entry.Summary)
		fmt.Fprintf(out, "  content: %s\n", entry.Content)
		fmt.Fprintf(out, "  evidence: %d\n", len(entry.Evidence))
		if len(entry.ProjectionTargets) > 0 {
			targets := make([]string, 0, len(entry.ProjectionTargets))
			for _, target := range entry.ProjectionTargets {
				targets = append(targets, target.Host+"/"+target.Loop)
			}
			fmt.Fprintf(out, "  project_to: %s\n", strings.Join(targets, ", "))
		}
	}
}
