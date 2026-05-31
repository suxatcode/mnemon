package status

import "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"

// The writeback verifier. Mnemon can PUSH context perfectly (projection.applied
// carries a content digest) but cannot force a host to read, act, and report
// faithfully — so the WRITEBACK side is not engineerable, only verifiable. This
// fold over the event log yields, per host, a four-state readback (you cannot
// force the host to echo, so fewer states would lie):
//
//	observed                host echoed a digest we projected (cooperated + reported)
//	mismatch                host echoed a digest we never projected (wrong/unknown context)
//	acted-but-unattributed  host wrote events but echoed no digest at all
//	silent                  host wrote nothing back at all
//
// plus a staleness flag — the host echoed a known OLDER digest while a newer
// projection is live (acting on stale context). mismatch is distinct from
// acted-but-unattributed: the former reported a wrong/unknown value, the latter
// reported nothing, which are diagnosably different host faults.

const (
	ReadbackObserved     = "observed"
	ReadbackMismatch     = "mismatch"
	ReadbackUnattributed = "acted-but-unattributed"
	ReadbackSilent       = "silent"
)

// HostReadback is the per-host verification state.
type HostReadback struct {
	Host              string `json:"host"`
	State             string `json:"state"`
	Stale             bool   `json:"stale,omitempty"`
	LiveProjectionRef string `json:"live_projection_ref,omitempty"`
	LiveDigest        string `json:"live_digest,omitempty"`
	ObservedDigest    string `json:"observed_digest,omitempty"`
	LiveTS            string `json:"live_ts,omitempty"`
	LastWritebackTS   string `json:"last_writeback_ts,omitempty"`
}

// DeriveReadback folds projection.applied + host writeback events into a per-host
// readback. A host appears only once it has a live projection. Best-effort
// attribution: a host that wrote back without echoing is acted-but-unattributed,
// never falsely silent.
func DeriveReadback(events []schema.Event) []HostReadback {
	type hostState struct {
		liveDigest      string
		liveRef         string
		liveTS          string
		knownDigests    map[string]bool
		hadWriteback    bool
		lastWritebackTS string
		latestEcho      string
	}
	hosts := map[string]*hostState{}
	var order []string
	projDigestByID := map[string]string{}
	ensure := func(h string) *hostState {
		s, ok := hosts[h]
		if !ok {
			s = &hostState{knownDigests: map[string]bool{}}
			hosts[h] = s
			order = append(order, h)
		}
		return s
	}

	for _, ev := range events {
		host := ""
		if ev.Host != nil {
			host = *ev.Host
		}
		switch {
		case ev.Type == "projection.applied":
			digest := payloadString(ev.Payload, "context_digest")
			if ev.ID != "" && digest != "" {
				projDigestByID[ev.ID] = digest
			}
			if host != "" && digest != "" {
				s := ensure(host)
				s.liveDigest = digest
				s.liveRef = payloadString(ev.Payload, "projection_ref")
				s.liveTS = ev.TS
				s.knownDigests[digest] = true
			}
		case ev.Actor == "host-agent" && host != "":
			// A host's genuine writeback. (The projector writes as actor=projector;
			// governed apply as mnemon-manual — neither counts as host writeback.)
			s := ensure(host)
			s.hadWriteback = true
			s.lastWritebackTS = ev.TS
			// The host echoes the digest it read from PROJECTION.json — as
			// observed_projection_ref or observed_context_digest — or, failing an
			// explicit echo, via caused_by pointing at the projection.applied event.
			echo := payloadString(ev.Payload, "observed_projection_ref")
			if echo == "" {
				echo = payloadString(ev.Payload, "observed_context_digest")
			}
			if echo == "" && ev.CausedBy != nil {
				echo = projDigestByID[*ev.CausedBy] // host echoed via caused_by
			}
			if echo != "" {
				s.latestEcho = echo
			}
		}
	}

	var out []HostReadback
	for _, h := range order {
		s := hosts[h]
		if s.liveDigest == "" {
			continue // no projection for this host yet — not in readback
		}
		rb := HostReadback{
			Host:              h,
			LiveProjectionRef: s.liveRef,
			LiveDigest:        s.liveDigest,
			LiveTS:            s.liveTS,
			LastWritebackTS:   s.lastWritebackTS,
			ObservedDigest:    s.latestEcho,
		}
		switch {
		case s.latestEcho != "" && s.latestEcho == s.liveDigest:
			rb.State = ReadbackObserved
		case s.latestEcho != "" && s.knownDigests[s.latestEcho]:
			rb.State = ReadbackObserved // echoed a real, but older, projection
			rb.Stale = true
		case s.latestEcho != "":
			rb.State = ReadbackMismatch // echoed a digest we never projected
		case s.hadWriteback:
			rb.State = ReadbackUnattributed
		default:
			rb.State = ReadbackSilent
		}
		out = append(out, rb)
	}
	return out
}
