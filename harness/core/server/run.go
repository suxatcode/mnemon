package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultStorePath is the ONE canonical kernel-store path the harness control plane defaults to.
// It is the single source of truth shared by `mnemon-harness server` and the lifecycle/app apply
// surface, so a write through one surface is readable by a pull through the other (no store split).
//
// It is the harness control store under the project's `.mnemon/harness` tree, so the lifecycle/app
// apply surface (coreengine, which resolves it under the project root) and `mnemon-harness server`
// (which resolves it under the CWD the operator boots from) land on the same file. Tests and dev
// may override it with an explicit path.
const DefaultStorePath = ".mnemon/harness/control/governed.db"

// RunHTTPServer boots a ControlServer over a persistent kernel store and serves the channel
// (ServerAPI: observe via Ingest, pull via PullProjection) over httpapi on addr until ctx is
// cancelled. It is the `mnemon-harness server` endpoint (the standalone mnemon-control binary
// folded into the one harness binary, D2). The kernel store + kernel are constructed INSIDE the
// server package so the CLI reaches the engine only through this factory + ServerAPI, never by
// importing kernel/reconcile directly (the P2.3 boundary).
//
// The server boots the one server-owned Runtime over the store (service mode, S11 single-writer) with
// a BARE config — an empty rule set and no preconfigured actors: a bare channel endpoint (records
// observations, serves scoped projections). Policy (rules/actors/subs) is a configuration seam a
// richer boot path supplies via RuntimeConfig / NewFromConfig.
func RunHTTPServer(ctx context.Context, addr, storePath string, out io.Writer) error {
	rt, err := OpenRuntime(storePath, RuntimeConfig{})
	if err != nil {
		return err
	}
	defer rt.Close()

	srv := &http.Server{Addr: addr, Handler: NewHTTPHandler(rt.API())}
	errc := make(chan error, 1)
	go func() {
		fmt.Fprintf(out, "mnemon-harness server: listening on %s (store %s)\n", addr, storePath)
		if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			errc <- serveErr
			return
		}
		errc <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		fmt.Fprintln(out, "mnemon-harness server: shut down")
		return nil
	case serveErr := <-errc:
		return serveErr
	}
}
