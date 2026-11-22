package cagebridge

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Sizes a UDP answer keeps to.
const (
	// classicUDP is the largest UDP answer a client takes while it leaves its size unnamed.
	classicUDP = 512
	// ednsSize is the size the bridge's own EDNS record names.
	ednsSize = 1232
)

// exchangeTimeout bounds one query's trip to the socket and back.
const exchangeTimeout = 5 * time.Second

// errTooLong reports a message longer than a stream's two-byte length can carry.
var errTooLong = errors.New("DNS message longer than 65535 bytes")

// Datagrams answers every UDP query on the packet connection through the socket, which speaks DNS over a stream, until
// the context ends. An answer larger than the client takes over UDP comes back cut short with truncation set, so the
// client asks again over TCP.
func Datagrams(ctx context.Context, pc net.PacketConn, sock string) error {
	stop := context.AfterFunc(ctx, func() { _ = pc.Close() })
	defer stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	buf := make([]byte, math.MaxUint16)
	// Answer queries until the connection closes
	for {
		n, addr, err := pc.ReadFrom(buf)
		// Report a clean stop once the context ends, and any other failure as it is
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		q := append([]byte(nil), buf[:n]...)
		wg.Go(func() {
			a, err := exchange(ctx, sock, q)
			// Leave a query unanswered whose socket stays silent, so the client asks again
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(fit(q, a), addr)
		})
	}
}

// exchange sends one query over the socket and reads its answer.
func exchange(ctx context.Context, sock string, q []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, exchangeTimeout)
	defer cancel()
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", sock)
	// Hand back a socket out of reach
	if err != nil {
		return nil, err
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	size := len(q)
	// Turn away a query longer than its two-byte length carries
	if size > math.MaxUint16 {
		return nil, errTooLong
	}
	// Hand back a query that fails to go out
	if _, err := c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(size)), q...)); err != nil {
		return nil, err
	}
	var n [2]byte
	// Hand back an answer that fails to arrive
	if _, err := io.ReadFull(c, n[:]); err != nil {
		return nil, err
	}
	a := make([]byte, binary.BigEndian.Uint16(n[:]))
	_, err = io.ReadFull(c, a)
	return a, err
}

// fit hands back the answer as it is where it fits the size the client takes over UDP, and otherwise an answer cut
// short to its header and question with truncation set.
func fit(q, a []byte) []byte {
	limit, edns := udpLimit(q)
	// Send an answer that fits as it is
	if len(a) <= limit {
		return a
	}
	var p dnsmessage.Parser
	h, err := p.Start(a)
	// Send an answer whose header fails to parse as it is, for the client to judge
	if err != nil {
		return a
	}
	questions, _ := p.AllQuestions()
	h.Truncated = true
	b := dnsmessage.NewBuilder(nil, h)
	_ = b.StartQuestions()
	// Echo each question the answer is for
	for _, question := range questions {
		_ = b.Question(question)
	}
	// Answer an EDNS query with an EDNS record of the bridge's own
	if edns {
		_ = b.StartAdditionals()
		var rh dnsmessage.ResourceHeader
		_ = rh.SetEDNS0(ednsSize, dnsmessage.RCodeSuccess, false)
		_ = b.OPTResource(rh, dnsmessage.OPTResource{})
	}
	cut, err := b.Finish()
	// Send the answer as it is where the short form fails to build
	if err != nil {
		return a
	}
	return cut
}

// udpLimit reads the largest UDP answer the client takes, and whether it asked with EDNS.
func udpLimit(q []byte) (int, bool) {
	var p dnsmessage.Parser
	// Take the classic size for a query whose header fails to parse
	if _, err := p.Start(q); err != nil {
		return classicUDP, false
	}
	_ = p.SkipAllQuestions()
	_ = p.SkipAllAnswers()
	_ = p.SkipAllAuthorities()
	// Look for the EDNS record among the additionals
	for {
		h, err := p.AdditionalHeader()
		// Take the classic size once the section ends ahead of an EDNS record
		if err != nil {
			return classicUDP, false
		}
		// Take the size the EDNS record names, at the classic size or above
		if h.Type == dnsmessage.TypeOPT {
			return max(int(h.Class), classicUDP), true
		}
		_ = p.SkipAdditional()
	}
}
