// mnemond is the LOCAL governance daemon: the standalone-daemon packaging of the exact
// `mnemon-harness local run` boot path (P1 D13 — the mnemond name now belongs to the local
// trust domain; the remote hub binary builds as mnemon-hub). It is the LOCAL trust domain
// main: it imports internal/app and shares the boot face in app/localboot.go with `local run`,
// so flags, banner, T1 loopback floor, and serve behavior stay alias-identical. One daemon per
// project store (the store's single-writer flock enforces it).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mnemond: %v\n", err)
		os.Exit(1)
	}
}

// run is the whole daemon behind a testable seam: parse the `local run`-equivalent flag face,
// resolve the SAME boot chain (app.ResolveLocalBoot: setup config discovery, endpoint-derived
// listen address, T1 loopback validation), print the same banner, and serve Local Mnemon until
// ctx cancels.
func run(ctx context.Context, args []string, out, errw io.Writer) error {
	fs := flag.NewFlagSet("mnemond", flag.ContinueOnError)
	fs.SetOutput(errw)
	root := fs.String("root", ".", "project root")
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	syncInterval := fs.Duration("sync-interval", 0, "sync worker cadence (0 = default 30s)")
	allowNonLoopback := fs.Bool("allow-nonloopback", false, "explicitly allow listening on a non-loopback address (T1: loopback-only by default)")
	ignoreExternal := fs.Bool("ignore-external", false, "boot the embedded-only capability catalog, ignoring external packages under .mnemon/loops (each ignored package is named on stderr)")
	allowInsecureRemote := fs.Bool("allow-insecure-remote", false, "let the background sync worker use a plaintext http:// Remote Workspace endpoint with a non-loopback host (T2: fail-closed by default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	projectRoot := "."
	if *root != "" {
		projectRoot = filepath.Clean(*root)
	}
	boot, err := app.ResolveLocalBoot(projectRoot, "", "")
	if err != nil {
		return err
	}
	listenAddr := *addr
	addrChanged := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrChanged = true
		}
	})
	if !addrChanged {
		listenAddr = app.ListenAddrFromEndpoint(boot.Config.Endpoint, *addr)
	}
	if err := app.ValidateListenAddr(listenAddr, *allowNonLoopback); err != nil {
		return err
	}
	fmt.Fprintln(out, "Local Mnemon: ready")
	fmt.Fprintln(out, "Remote Workspace: "+app.RemoteWorkspaceStatus(projectRoot))
	return app.RunLocalHTTPServerWithBindings(ctx, listenAddr, boot.StorePath, boot.Loaded, app.ServeOptions{
		Loops:               boot.Config.Loops,
		Hosts:               boot.Config.Hosts,
		ProjectRoot:         projectRoot,
		MirrorMode:          boot.Config.MirrorMode,
		IgnoreExternal:      *ignoreExternal,
		AllowInsecureRemote: *allowInsecureRemote,
		SyncInterval:        *syncInterval,
	}, io.Discard)
}
