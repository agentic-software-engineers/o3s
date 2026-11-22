package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// askTimeout bounds one question to the control socket.
const askTimeout = 3 * time.Second

// client reaches the control socket at the path.
func client(path string) *http.Client {
	return &http.Client{
		Timeout: askTimeout,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}},
	}
}

// GetInfo asks the process on the control socket about itself.
func GetInfo(ctx context.Context, path string) (Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://o3s/v1/info", nil)
	// Hand back a request that fails to build
	if err != nil {
		return Info{}, err
	}
	resp, err := client(path).Do(req)
	// Hand back a process that stays silent
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	// Hand back an answer other than the info
	if resp.StatusCode != http.StatusOK {
		return Info{}, fmt.Errorf("control answered %s", resp.Status)
	}
	var info Info
	err = json.NewDecoder(resp.Body).Decode(&info)
	return info, err
}

// Shutdown asks the process on the control socket to stop.
func Shutdown(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://o3s/v1/shutdown", nil)
	// Hand back a request that fails to build
	if err != nil {
		return err
	}
	resp, err := client(path).Do(req)
	// Hand back a process that stays silent
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	// Hand back an answer other than the acceptance
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("control answered %s", resp.Status)
	}
	return nil
}
