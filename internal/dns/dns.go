// Package dns answers the cage's name lookups: it serves DNS on each stream the relay hands over, answers the names
// the allowlist holds from the host's nameservers, and answers every other name as missing.
package dns

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"
)

// errTooLong reports a message longer than a stream's two-byte length can carry.
var errTooLong = errors.New("DNS message longer than 65535 bytes")

// Timings and limits a server takes where its options leave them at zero.
const (
	// defaultIdle is how long a stream may wait between full queries before the server ends it.
	defaultIdle = 10 * time.Second
	// defaultAttempt is how long the server waits on one nameserver.
	defaultAttempt = 2 * time.Second
	// defaultBudget is how long the server spends on one query across every nameserver, inside the five seconds a
	// stub resolver waits.
	defaultBudget = 4 * time.Second
	// defaultInFlight is how many queries of one stream the server works on at once.
	defaultInFlight = 32
)

// Policy decides which names the cage may resolve.
type Policy interface {
	// Lists reports whether the allowlist holds the name, in lower case and with its closing dot dropped.
	Lists(name string) bool
}

// Upstream hands over the nameservers the host uses right now, as host and port, in the order to ask them.
type Upstream interface {
	// Nameservers lists the nameservers to ask.
	Nameservers() ([]string, error)
}

// Options are what a server works with.
type Options struct {
	// Policy decides which names resolve, and every name is answered as missing while it is empty.
	Policy Policy
	// Upstream lists the nameservers to forward to, and every allowed query fails while it is empty.
	Upstream Upstream
	// Log takes a line for every denied name.
	Log *slog.Logger
	// Dial reaches a nameserver, and a plain dialer does where it is empty.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// Idle is how long a stream may wait between full queries.
	Idle time.Duration
	// Attempt is how long the server waits on one nameserver.
	Attempt time.Duration
	// Budget is how long the server spends on one query across every nameserver.
	Budget time.Duration
	// InFlight is how many queries of one stream the server works on at once.
	InFlight int
}

// Server answers the cage's queries, one stream at a time as the relay hands them over.
type Server struct {
	o Options
}

// New makes a server with the options, filling the timings and limits left at zero.
func New(o Options) *Server {
	// Discard the log lines while the options leave the log empty
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	// Reach nameservers directly while the options leave the dial empty
	if o.Dial == nil {
		var d net.Dialer
		o.Dial = d.DialContext
	}
	o.Idle = orDefault(o.Idle, defaultIdle)
	o.Attempt = orDefault(o.Attempt, defaultAttempt)
	o.Budget = orDefault(o.Budget, defaultBudget)
	o.InFlight = orDefault(o.InFlight, defaultInFlight)
	return &Server{o: o}
}

// orDefault hands back the value, or the default where the value is zero.
func orDefault[T time.Duration | int](v, d T) T {
	// Take the default for a value left at zero
	if v <= 0 {
		return d
	}
	return v
}

// Handle answers the queries of one stream, each as soon as it is ready, until the stream ends, stays idle past its
// idle period, or the context ends.
func (s *Server) Handle(ctx context.Context, c net.Conn) {
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	var (
		wg    sync.WaitGroup
		write sync.Mutex
	)
	defer wg.Wait()
	slots := make(chan struct{}, s.o.InFlight)
	// Read one full query after another until the stream ends
	for {
		_ = c.SetReadDeadline(time.Now().Add(s.o.Idle))
		msg, err := readFrame(c)
		// End the stream once it closes, idles out or breaks off a query
		if err != nil {
			return
		}
		// Wait for a free slot, or stop once the context ends
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		wg.Go(func() {
			defer func() { <-slots }()
			reply := s.answer(ctx, msg)
			// Stay silent on a message the server drops
			if reply == nil {
				return
			}
			write.Lock()
			defer write.Unlock()
			_ = c.SetWriteDeadline(time.Now().Add(s.o.Idle))
			_ = writeFrame(c, reply)
		})
	}
}

// readFrame reads one DNS message behind its two-byte length.
func readFrame(r io.Reader) ([]byte, error) {
	var size [2]byte
	// Hand back a stream that ends before the length
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}
	msg := make([]byte, binary.BigEndian.Uint16(size[:]))
	_, err := io.ReadFull(r, msg)
	return msg, err
}

// writeFrame sends one DNS message with its two-byte length in a single write.
func writeFrame(w io.Writer, msg []byte) error {
	size := len(msg)
	// Turn away a message longer than its two-byte length can carry
	if size > math.MaxUint16 {
		return errTooLong
	}
	_, err := w.Write(append(binary.BigEndian.AppendUint16(nil, uint16(size)), msg...))
	return err
}
