// Command o3s-cage runs inside the cage: it carries the cage's proxy and DNS ports to the host's sockets, and opens
// the tunnels ssh asks for through the proxy.
//
//	o3s-cage serve [-sockets DIR] [-proxy ADDR,...] [-dns ADDR]
//	o3s-cage connect HOST PORT [-sockets DIR]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hansehart/o3s/internal/cagebridge"
)

// Where the bridge listens and finds the host's sockets by default.
const (
	defaultSockets = "/run/o3s"
	defaultProxy   = "127.0.0.1:3128,[::1]:3128"
	defaultDNS     = "127.0.0.53:53"
)

// errUsage reports a command line outside the bridge's commands.
var errUsage = errors.New("usage: o3s-cage serve [-sockets DIR] [-proxy ADDR,...] [-dns ADDR] | o3s-cage connect HOST PORT [-sockets DIR]")

func main() {
	os.Exit(realMain())
}

// realMain runs the command line until it ends or a signal arrives, and hands back the exit status.
func realMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Report a failure with a failing status
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "o3s-cage:", err)
		return 1
	}
	return 0
}

// run carries out the command the arguments name.
func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	// Turn away an empty command line
	if len(args) == 0 {
		return errUsage
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errOut)
	sockets := fs.String("sockets", defaultSockets, "the folder that holds the host's sockets")
	proxyAddrs := fs.String("proxy", defaultProxy, "the loopback addresses the proxy port listens on")
	dnsAddr := fs.String("dns", defaultDNS, "the loopback address the DNS port listens on")
	// Hand back flags that fail to parse
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	// Carry out the command the arguments name
	switch {
	case args[0] == "serve" && fs.NArg() == 0:
		return serve(ctx, *sockets, strings.Split(*proxyAddrs, ","), *dnsAddr, errOut)
	case args[0] == "connect" && fs.NArg() == 2:
		return cagebridge.Connect(ctx, filepath.Join(*sockets, "proxy.sock"), fs.Arg(0), fs.Arg(1), in, out)
	}
	return errUsage
}

// serve waits for the host's sockets, then carries the proxy and DNS ports to them until the context ends.
func serve(ctx context.Context, sockets string, proxyAddrs []string, dnsAddr string, errOut io.Writer) error {
	proxySock, dnsSock := filepath.Join(sockets, "proxy.sock"), filepath.Join(sockets, "dns.sock")
	// Hand back a wait the context ends first
	if err := cagebridge.WaitFor(ctx, proxySock, dnsSock); err != nil {
		return err
	}
	var lc net.ListenConfig
	var wg sync.WaitGroup
	defer wg.Wait()
	// Listen on each proxy address the system offers
	for _, addr := range proxyAddrs {
		l, err := lc.Listen(ctx, "tcp", addr)
		// Carry on past an address the system lacks, such as IPv6 loopback while IPv6 is off
		if err != nil {
			fmt.Fprintln(errOut, "o3s-cage: proxy port skipped:", err)
			continue
		}
		wg.Go(func() { _ = cagebridge.Stream(ctx, l, proxySock) })
	}
	pc, err := lc.ListenPacket(ctx, "udp", dnsAddr)
	// Hand back a DNS port that fails to listen
	if err != nil {
		return err
	}
	wg.Go(func() { _ = cagebridge.Datagrams(ctx, pc, dnsSock) })
	l, err := lc.Listen(ctx, "tcp", dnsAddr)
	// Hand back a DNS port that fails to listen over TCP
	if err != nil {
		return err
	}
	wg.Go(func() { _ = cagebridge.Stream(ctx, l, dnsSock) })
	<-ctx.Done()
	return nil
}
