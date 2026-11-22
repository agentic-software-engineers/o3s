package hostd

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/host"
	"github.com/hansehart/o3s/internal/paths"
	"github.com/hansehart/o3s/internal/secrets"
	"github.com/hansehart/o3s/internal/service"
)

// ServeOptions are what the host process runs with.
type ServeOptions struct {
	// Version is the o3s version the process reports.
	Version string
	// Log takes the process's log lines.
	Log *slog.Logger
}

// Serve runs the home's host process until the context ends or a shutdown arrives: it holds the home, answers on the
// control socket, and serves the cage's connections from the home's files.
func Serve(ctx context.Context, p Place, o ServeOptions) error {
	lock, err := TryLock(p.Lock())
	// Turn away a second process for the home
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	t, err := transport(p.Sockets)
	// Hand back a system the cage's transport awaits
	if err != nil {
		return err
	}
	secretsFile, err := secrets.Path()
	// Hand back a secrets file whose place stays unknown
	if err != nil {
		return err
	}
	l, err := ListenControl(ctx, p.Control())
	// Hand back a control socket that fails to listen
	if err != nil {
		return err
	}
	defer os.Remove(p.Control())
	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	info := Info{Version: o.Version, PID: os.Getpid(), Home: p.Home}
	wg.Go(func() { _ = ServeControl(ctx, l, info, stop, o.Log) })
	o.Log.Info("serving", "home", p.Home, "sockets", p.Sockets, "version", o.Version)
	return service.Serve(ctx, service.Options{
		Files:     service.Files{Config: filepath.Join(p.Home, config.File), Secrets: secretsFile},
		CA:        host.CAFile(paths.Proxy()),
		Marker:    host.Marker(p.Home),
		Transport: t,
		Log:       o.Log,
	})
}
