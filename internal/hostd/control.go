package hostd

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// versionHeader carries the process's version on every control answer.
const versionHeader = "O3s-Version"

// headerTimeout bounds how long the control socket waits for a request's header.
const headerTimeout = 10 * time.Second

// shutdownGrace bounds how long the control socket lets the answers under way finish once the process stops.
const shutdownGrace = 5 * time.Second

// Info is what a running host process says about itself.
type Info struct {
	// Version is the o3s version the process runs.
	Version string `json:"version"`
	// PID is the process's own ID.
	PID int `json:"pid"`
	// Home is the folder of the home it serves.
	Home string `json:"home"`
}

// ListenControl listens on the control socket, taking the place of one a process left behind. The caller holds the
// home's lock, so a socket at the path belongs to a process that is gone.
func ListenControl(ctx context.Context, path string) (net.Listener, error) {
	// Clear the socket a gone process left behind
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "unix", path)
	// Hand back a socket that fails to listen
	if err != nil {
		return nil, err
	}
	// Open the socket to its owner alone, and keep it through the runtime folder's clean-ups
	if err := os.Chmod(path, 0o600|os.ModeSticky); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

// ServeControl answers on the listener until the context ends: the info on GET /v1/info, and a stop of the process on
// POST /v1/shutdown.
func ServeControl(ctx context.Context, l net.Listener, info Info, stop context.CancelFunc, log *slog.Logger) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(versionHeader, info.Version)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})
	mux.HandleFunc("POST /v1/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(versionHeader, info.Version)
		w.WriteHeader(http.StatusAccepted)
		log.Info("shutdown asked")
		stop()
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: headerTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelDebug),
	}
	finished := make(chan struct{})
	ending := context.AfterFunc(ctx, func() {
		defer close(finished)
		grace, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		// Deliver every answer under way, the shutdown's own included, then close what outlasts the grace
		if err := srv.Shutdown(grace); err != nil {
			_ = srv.Close()
		}
	})
	err := srv.Serve(l)
	// Wait for the answers under way where the stop has begun
	if !ending() {
		<-finished
	}
	// Report a server that stopped once the context ended as a clean stop
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
