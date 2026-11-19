package registry

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/hansehart/o3s/internal/fixture"
)

// A registry that turns a store away reads as a plain cause, by what the registry answered, and never with the
// addresses it asked.
func TestServedNamesWhyARegistryTurnsAStoreAway(t *testing.T) {
	asked := &http.Request{URL: &url.URL{Scheme: "https", Host: fixture.Host, Path: "/token"}}
	tests := []struct {
		err  error
		want string
	}{
		{&transport.Error{StatusCode: http.StatusUnauthorized, Request: asked}, "denies access, log in with docker login " + fixture.Host},
		{&transport.Error{StatusCode: http.StatusForbidden, Request: asked}, "denies access, log in with docker login " + fixture.Host},
		{&transport.Error{StatusCode: http.StatusNotFound, Request: asked}, "holds no feature collection"},
		{&transport.Error{StatusCode: http.StatusBadGateway, Request: asked}, "is out of reach (Bad Gateway)"},
		{&url.Error{Op: "Get", URL: asked.URL.String(), Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}, "is out of reach (dial: connection refused)"},
	}
	// Check every answer a registry gives
	for _, tt := range tests {
		err := served(fixture.Host, tt.err)

		// Report a cause that reads otherwise or names an address the registry was asked at
		if err == nil || err.Error() != tt.want || strings.Contains(err.Error(), "/token") {
			t.Errorf("served(%v) = %v, want %q", tt.err, err, tt.want)
		}
	}
}
