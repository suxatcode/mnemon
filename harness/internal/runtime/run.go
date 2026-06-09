package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
)

// DiscoverProjectStore resolves the canonical control-store path for the project that contains the
// current working directory. It walks up from the CWD for an existing `.mnemon` directory (the
// project marker, like git's `.git`) and resolves DefaultStorePath under that project root — so the
// channel server lands on the SAME store the lifecycle/app apply surface uses (which resolves
// DefaultStorePath under the project root) regardless of WHICH subdirectory the server is booted
// from. With no `.mnemon` ancestor it falls back to DefaultStorePath under the CWD; an operator
// running the server detached from the project tree must pass an explicit --store. OpenRuntime
// absolutizes the result, so the boot log + status report the canonical path.
func DiscoverProjectStore() string {
	return filepath.Join(DiscoverProjectRoot(), DefaultStorePath)
}

// DiscoverProjectRoot walks up from the current working directory for an existing `.mnemon` directory
// and returns the project root that contains it (the dir, not `.mnemon` itself), or the CWD when no
// `.mnemon` ancestor exists. It is the base for resolving DefaultStorePath and project-relative
// binding/credential refs, so every harness surface resolves them against the same root.
func DiscoverProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir := cwd; ; {
		if fi, err := os.Stat(filepath.Join(dir, ".mnemon")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

// DefaultStorePath is the canonical Local Mnemon kernel-store path under the
// project's `.mnemon/harness` tree. Tests and dev may override it with an
// explicit path.
const DefaultStorePath = ".mnemon/harness/local/governed.db"

// RunHTTPServer boots a ControlServer over a persistent kernel store and serves the channel
// (channel.ServerAPI: observe via Ingest, pull via PullProjection) over httpapi on addr until ctx is
// cancelled. The kernel store + kernel are constructed inside the server
// package so command surfaces use this factory + channel.ServerAPI rather than importing
// kernel/reconcile directly.
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
	return ServeRuntime(ctx, addr, rt, channel.HeaderAuthenticator{}, out)
}

// RunHTTPServerWithBindings boots the server from a loaded channel-binding manifest (P3.2): the
// runtime enforces the bindings (channel.BindingSet authorizer) and serves only the subscription scopes the
// bindings declare, and — when the bindings carry credential refs — a channel.TokenAuthenticator resolves the
// principal from the bearer token (trusted-header auth remains the local/dev/httptest default when no
// tokens are configured). The store path is still the canonical project store.
func RunHTTPServerWithBindings(ctx context.Context, addr, storePath string, loaded channel.LoadedBindings, out io.Writer) error {
	rt, err := OpenRuntime(storePath, RuntimeConfig{
		Bindings: loaded.Bindings,
		Subs:     channel.SubsFromBindings(loaded.Bindings),
	})
	if err != nil {
		return err
	}
	defer rt.Close()
	return ServeRuntime(ctx, addr, rt, channel.NewBindingAuthenticator(loaded), out)
}

// ServeRuntime serves the runtime's channel over httpapi until ctx is cancelled. It is the shared
// boot loop for the bare and binding-configured server front doors.
func ServeRuntime(ctx context.Context, addr string, rt *Runtime, auth channel.Authenticator, out io.Writer) error {
	srv := &http.Server{Addr: addr, Handler: NewRuntimeHandler(rt, auth)}
	errc := make(chan error, 1)
	go func() {
		fmt.Fprintf(out, "Local Mnemon: listening on %s (store %s)\n", addr, rt.StorePath())
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
		fmt.Fprintln(out, "Local Mnemon: shut down")
		return nil
	case serveErr := <-errc:
		return serveErr
	}
}
