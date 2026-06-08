// Package driver is the co-hosted Background Driver: it runs INSIDE the Local Runtime process (holding
// the same single store-writer lock — never a second opener) and periodically drives the governed
// Tick, drains projection invalidations, and re-projects the host's managed definition files. It is
// the only place re-projection lives, so the runtime never imports hostsurface (the locked boundary).
package driver

import (
	"context"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// Driver drives one runtime's background duties. reproject is invoked only when a Tick actually
// drained an invalidation (it is nil for a runtime with no host projection).
type Driver struct {
	rt        *runtime.Runtime
	reproject func() error
	interval  time.Duration
}

// New builds a Driver over rt with an injected re-projection callback (the host-free seam used by
// tests). interval <= 0 defaults to one second.
func New(rt *runtime.Runtime, reproject func() error, interval time.Duration) *Driver {
	return &Driver{rt: rt, reproject: reproject, interval: interval}
}

// ForHost builds a Driver whose re-projection refreshes the host's managed definition files via
// hostsurface.ReProject (the no-clobber path). Re-projection lives here, in the driver, so the runtime
// never imports hostsurface.
func ForHost(rt *runtime.Runtime, pc hostsurface.ProjectContext, interval time.Duration) *Driver {
	return New(rt, func() error { _, err := hostsurface.ReProject(pc, nil); return err }, interval)
}

// Tick runs one background cycle: advance the governed Tick, drain any projection invalidations, and —
// only if something was invalidated — re-project. It uses the runtime's own store (no second opener).
func (d *Driver) Tick(ctx context.Context) error {
	if _, err := d.rt.Tick(); err != nil {
		return err
	}
	n, err := d.rt.DrainOutbox()
	if err != nil {
		return err
	}
	if n > 0 && d.reproject != nil {
		return d.reproject()
	}
	return nil
}

// Run loops Tick on the interval until ctx is cancelled (clean shutdown). It returns ctx.Err() on
// cancellation, or the first Tick error.
func (d *Driver) Run(ctx context.Context) error {
	interval := d.interval
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := d.Tick(ctx); err != nil {
				return err
			}
		}
	}
}
