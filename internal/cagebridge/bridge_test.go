//go:build unix

package cagebridge_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hansehart/o3s/internal/cagebridge"
	"github.com/hansehart/o3s/internal/fixture"
)

// socketDir hands a test a short folder, since a socket's path has to fit the kernel's small limit.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when the folder fails to appear
	if err != nil {
		t.Fatalf("Setup: MkdirTemp = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// unixServer serves each connection on a socket in the folder with the handler until the test ends.
func unixServer(t *testing.T, path string, handle func(c net.Conn)) {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "unix", path)
	// Stop when the socket fails to listen
	if err != nil {
		t.Fatalf("Setup: Listen(%s) = %v", path, err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		// Serve every connection until the listener closes
		for {
			c, err := l.Accept()
			// Stop once the listener closes
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				handle(c)
			}()
		}
	}()
}

// loopback listens on a free port of the loopback address given.
func loopback(t *testing.T, addr string) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", net.JoinHostPort(addr, "0"))
	// Stop when the loopback address fails to listen
	if err != nil {
		t.Fatalf("Setup: Listen(%s) = %v", addr, err)
	}
	return l
}

// bridging carries the listener's connections to the socket until the test ends.
func bridging(t *testing.T, l net.Listener, sock string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cagebridge.Stream(ctx, l, sock)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// dial reaches the listener.
func dial(t *testing.T, l net.Listener) net.Conn {
	t.Helper()
	var d net.Dialer
	c, err := d.DialContext(t.Context(), "tcp", l.Addr().String())
	// Stop when the bridge stays out of reach
	if err != nil {
		t.Fatalf("Setup: Dial = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	return c
}

// A connection to the loopback reaches the socket, and a half-close lets the far side answer after reading to its end.
func TestStreamCarriesBothWaysAndPassesAHalfClose(t *testing.T) {
	sock := filepath.Join(socketDir(t), "proxy.sock")
	unixServer(t, sock, func(c net.Conn) {
		_, _ = io.ReadAll(c)
		_, _ = c.Write([]byte(fixture.Reply))
	})
	l := loopback(t, "127.0.0.1")
	bridging(t, l, sock)
	c := dial(t, l)

	_, _ = c.Write([]byte(fixture.Message))
	_ = c.(*net.TCPConn).CloseWrite()
	got, err := io.ReadAll(c)

	// Require the answer sent after the half-close
	if err != nil || string(got) != fixture.Reply {
		t.Errorf("read %q, %v, want %q", got, err, fixture.Reply)
	}
}

// A body far larger than any buffer arrives whole.
func TestStreamCarriesALargeBodyWhole(t *testing.T) {
	body := bytes.Repeat([]byte(fixture.Message), 50<<20/len(fixture.Message))
	want := sha256.Sum256(body)
	sock := filepath.Join(socketDir(t), "proxy.sock")
	unixServer(t, sock, func(c net.Conn) { _, _ = c.Write(body) })
	l := loopback(t, "127.0.0.1")
	bridging(t, l, sock)
	c := dial(t, l)

	got, err := io.ReadAll(c)

	// Require every byte in order
	if err != nil || sha256.Sum256(got) != want {
		t.Errorf("read %d bytes, %v, want %d bytes matching", len(got), err, len(body))
	}
}

// The IPv6 loopback reaches the socket as the IPv4 one does.
func TestStreamAnswersOnTheIPv6Loopback(t *testing.T) {
	sock := filepath.Join(socketDir(t), "proxy.sock")
	unixServer(t, sock, func(c net.Conn) { _, _ = c.Write([]byte(fixture.Reply)) })
	l := loopback(t, "::1")
	bridging(t, l, sock)

	got, err := io.ReadAll(dial(t, l))

	// Require the answer over IPv6
	if err != nil || string(got) != fixture.Reply {
		t.Errorf("read over ::1 %q, %v, want %q", got, err, fixture.Reply)
	}
}

// query builds a DNS query for the name, with an EDNS record naming the UDP size where one is given.
func query(t *testing.T, name string, udpSize int) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 7, RecursionDesired: true})
	_ = b.StartQuestions()
	_ = b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(name + "."), Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET})
	// Name the UDP size the client takes where the test gives one
	if udpSize > 0 {
		_ = b.StartAdditionals()
		var h dnsmessage.ResourceHeader
		_ = h.SetEDNS0(udpSize, dnsmessage.RCodeSuccess, false)
		_ = b.OPTResource(h, dnsmessage.OPTResource{})
	}
	msg, err := b.Finish()
	// Stop when the query fails to pack
	if err != nil {
		t.Fatalf("Setup: query = %v", err)
	}
	return msg
}

// answer builds an answer to the query with TXT records adding up to about the size given.
func answer(t *testing.T, q []byte, size int) []byte {
	t.Helper()
	var p dnsmessage.Parser
	h, _ := p.Start(q)
	question, _ := p.Question()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: h.ID, Response: true, RecursionDesired: true, RecursionAvailable: true})
	_ = b.StartQuestions()
	_ = b.Question(question)
	_ = b.StartAnswers()
	// Add records until the answer reaches its size
	for written := 0; written < size; written += 200 {
		_ = b.TXTResource(dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET, TTL: 60}, dnsmessage.TXTResource{TXT: []string{strings.Repeat("o", 190)}})
	}
	msg, err := b.Finish()
	// Stop when the answer fails to pack
	if err != nil {
		t.Fatalf("Setup: answer = %v", err)
	}
	return msg
}

// frameOf puts the two-byte length in front of a DNS message.
func frameOf(msg []byte) []byte {
	return append(binary.BigEndian.AppendUint16(nil, uint16(min(len(msg), math.MaxUint16))), msg...)
}

// dnsServer answers every framed query on the socket with an answer of about the size given.
func dnsServer(t *testing.T, sock string, size int) {
	t.Helper()
	unixServer(t, sock, func(c net.Conn) {
		r := bufio.NewReader(c)
		// Answer each framed query until the stream ends
		for {
			var n [2]byte
			// Stop once the stream ends
			if _, err := io.ReadFull(r, n[:]); err != nil {
				return
			}
			q := make([]byte, binary.BigEndian.Uint16(n[:]))
			_, _ = io.ReadFull(r, q)
			_, _ = c.Write(frameOf(answer(t, q, size)))
		}
	})
}

// datagrams carries UDP queries on a loopback port to the socket until the test ends, and hands back the port.
func datagrams(t *testing.T, sock string) net.PacketConn {
	t.Helper()
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	// Stop when the loopback port fails to listen
	if err != nil {
		t.Fatalf("Setup: ListenPacket = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cagebridge.Datagrams(ctx, pc, sock)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return pc
}

// askUDP sends the query over UDP and hands back the answer read whole.
func askUDP(t *testing.T, pc net.PacketConn, q []byte) dnsmessage.Message {
	t.Helper()
	var d net.Dialer
	c, err := d.DialContext(t.Context(), "udp", pc.LocalAddr().String())
	// Stop when the port stays out of reach
	if err != nil {
		t.Fatalf("Setup: Dial = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = c.Write(q)
	buf := make([]byte, 65535)
	n, err := c.Read(buf)
	// Stop when the answer fails to arrive
	if err != nil {
		t.Fatalf("read answer = %v", err)
	}
	var m dnsmessage.Message
	// Stop when the answer fails to parse
	if err := m.Unpack(buf[:n]); err != nil {
		t.Fatalf("unpack answer = %v", err)
	}
	return m
}

// A UDP query reaches the socket framed, and an answer that fits comes back whole.
func TestDatagramsCarryAQueryThatFits(t *testing.T) {
	sock := filepath.Join(socketDir(t), "dns.sock")
	dnsServer(t, sock, 300)
	pc := datagrams(t, sock)

	m := askUDP(t, pc, query(t, fixture.Host, 0))

	// Require the whole answer with truncation off
	if m.Truncated || len(m.Answers) == 0 || m.ID != 7 {
		t.Errorf("answer truncated %v with %d records and ID %d, want whole with ID 7", m.Truncated, len(m.Answers), m.ID)
	}
}

// An answer larger than the client takes over UDP comes back cut short with truncation set, so the client asks over
// TCP.
func TestDatagramsTruncateAnAnswerPastTheClientsSize(t *testing.T) {
	tests := map[string]int{"classic 512": 0, "EDNS 1232": 1232}
	// Ask with each client size on its own
	for label, size := range tests {
		t.Run(label, func(t *testing.T) {
			sock := filepath.Join(socketDir(t), "dns.sock")
			dnsServer(t, sock, 4000)
			pc := datagrams(t, sock)

			m := askUDP(t, pc, query(t, fixture.Host, size))

			// Require the truncation flag, the question echoed and an empty answer section
			if !m.Truncated || len(m.Answers) != 0 || len(m.Questions) != 1 {
				t.Errorf("answer truncated %v with %d records and %d questions, want truncated with 0 and 1", m.Truncated, len(m.Answers), len(m.Questions))
			}
		})
	}
}

// connectProxy is a proxy on the socket that opens a tunnel for a CONNECT to the target and answers inside with the
// words, and records the target asked for.
func connectProxy(t *testing.T, sock string, target chan<- string) {
	t.Helper()
	unixServer(t, sock, func(c net.Conn) {
		r := bufio.NewReader(c)
		req, err := http.ReadRequest(r)
		// Stop when the request fails to parse
		if err != nil {
			return
		}
		target <- req.Method + " " + req.Host
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
		line, _ := r.ReadString('\n')
		_, _ = io.WriteString(c, "echo "+line)
	})
}

// Connect opens a tunnel to the host and port through the proxy socket and carries the standard streams through it.
func TestConnectTunnelsTheStandardStreams(t *testing.T) {
	sock := filepath.Join(socketDir(t), "proxy.sock")
	target := make(chan string, 1)
	connectProxy(t, sock, target)
	var out bytes.Buffer

	err := cagebridge.Connect(t.Context(), sock, fixture.Host, "22", strings.NewReader(fixture.Message+"\n"), &out)

	// Require the CONNECT to the target and the far side's answer
	if err != nil || <-target != "CONNECT "+net.JoinHostPort(fixture.Host, "22") || out.String() != "echo "+fixture.Message+"\n" {
		t.Errorf("Connect = %v, wrote %q, want the tunnel's echo", err, out.String())
	}
}

// A tunnel the proxy refuses ends with an error that names the refusal.
func TestConnectReportsARefusal(t *testing.T) {
	sock := filepath.Join(socketDir(t), "proxy.sock")
	unixServer(t, sock, func(c net.Conn) {
		_, _ = http.ReadRequest(bufio.NewReader(c))
		_, _ = io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
	})

	err := cagebridge.Connect(t.Context(), sock, fixture.RefusedName, "22", strings.NewReader(""), io.Discard)

	// Require the refusal named
	if !errors.Is(err, cagebridge.ErrRefused) {
		t.Errorf("Connect to a refused host = %v, want ErrRefused", err)
	}
}

// Waiting for sockets holds while one is missing and ends once every one of them has appeared.
func TestWaitForReturnsOnceTheSocketsAppear(t *testing.T) {
	dir := socketDir(t)
	sock := filepath.Join(dir, "proxy.sock")
	done := make(chan error, 1)

	go func() { done <- cagebridge.WaitFor(t.Context(), sock) }()
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("WaitFor = %v while the socket was still missing, want it waiting", err)
	default:
	}
	unixServer(t, sock, func(net.Conn) {})

	// Require the wait to end soon after the socket appears
	select {
	case err := <-done:
		// Report a wait that ended with a failure
		if err != nil {
			t.Errorf("WaitFor = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitFor keeps waiting after the socket appeared")
	}
}

// An answer past the classic size that fits the EDNS size the client names comes back whole.
func TestDatagramsKeepAnAnswerWithinTheClientsEDNSSize(t *testing.T) {
	sock := filepath.Join(socketDir(t), "dns.sock")
	dnsServer(t, sock, 1000)
	pc := datagrams(t, sock)

	m := askUDP(t, pc, query(t, fixture.Host, 4096))

	// Require the whole answer with truncation off
	if m.Truncated || len(m.Answers) == 0 {
		t.Errorf("answer truncated %v with %d records, want whole", m.Truncated, len(m.Answers))
	}
}
