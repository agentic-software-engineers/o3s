// Command o3s sets up the hosts, secrets and features of an o3s home.
package main

import (
	"context"
	"os"

	"github.com/hansehart/o3s/internal/cli"
)

// version is the version a release build sets.
var version string

// cageDigest is the digest of the cage image a release build was built with, which the release sets.
var cageDigest string

func main() {
	cli.Version = version
	cli.Digests = map[string]string{"ghcr.io/hansehart/o3s-cage": cageDigest}
	os.Exit(cli.Run(context.Background(), os.Args[1:], cli.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}
