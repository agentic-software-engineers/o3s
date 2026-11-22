package dns_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/dns"
	"github.com/hansehart/o3s/internal/fixture"
)

// queryID is the ID every test query carries.
const queryID = 0x4f33

// answerTTL is the time to live a test upstream gives its records.
const answerTTL = 300

// frameOf puts the two-byte length in front of a DNS message, holding a test message to the length the field carries.
func frameOf(msg []byte) []byte {
	return append(binary.BigEndian.AppendUint16(nil, uint16(min(len(msg), math.MaxUint16))), msg...)
}

// writeFrame sends one DNS message with its two-byte length.
func writeFrame(w io.Writer, msg []byte) error {
	_, err := w.Write(frameOf(msg))
	return err
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

// name reads a domain name in its absolute form.
func name(n string) dnsmessage.Name {
	return dnsmessage.MustNewName(strings.TrimSuffix(n, ".") + ".")
}

// queryOpt shapes a test query.
type queryOpt func(*dnsmessage.Builder, *dnsmessage.Header) error

// withOPT adds an EDNS record to the query, with the DNSSEC bit set.
func withOPT() queryOpt {
	return func(b *dnsmessage.Builder, _ *dnsmessage.Header) error {
		var h dnsmessage.ResourceHeader
		// Hand back a record that fails to shape
		if err := h.SetEDNS0(1232, dnsmessage.RCodeSuccess, true); err != nil {
			return err
		}
		return b.OPTResource(h, dnsmessage.OPTResource{})
	}
}

// query builds a recursive query for the name and type, with the questions and additional records the options add.
func query(t *testing.T, n string, typ dnsmessage.Type, opts ...queryOpt) []byte {
	t.Helper()
	return build(t, dnsmessage.Header{ID: queryID, RecursionDesired: true}, []dnsmessage.Question{
		{Name: name(n), Type: typ, Class: dnsmessage.ClassINET},
	}, opts...)
}

// build packs a message with the header, the questions and the additional records the options add.
func build(t *testing.T, h dnsmessage.Header, qs []dnsmessage.Question, opts ...queryOpt) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, h)
	_ = b.StartQuestions()
	// Add each question
	for _, q := range qs {
		_ = b.Question(q)
	}
	_ = b.StartAdditionals()
	// Add what each option asks for
	for _, o := range opts {
		// Stop when an option fails to apply
		if err := o(&b, &h); err != nil {
			t.Fatalf("Setup: build query = %v", err)
		}
	}
	msg, err := b.Finish()
	// Stop when the query fails to pack
	if err != nil {
		t.Fatalf("Setup: Finish = %v", err)
	}
	return msg
}

// upstream is a nameserver over TCP that answers from its table and keeps every query it reads.
type upstream struct {
	addr   string
	mu     sync.Mutex
	got    [][]byte
	answer func(q dnsmessage.Question, raw []byte) []byte
}

// queries hands back every query the nameserver read.
func (u *upstream) queries() [][]byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([][]byte(nil), u.got...)
}

// nameserver starts a nameserver on the loopback that answers each query with what the function builds.
func nameserver(t *testing.T, answer func(q dnsmessage.Question, raw []byte) []byte) *upstream {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	// Stop when the nameserver fails to listen
	if err != nil {
		t.Fatalf("Setup: Listen = %v", err)
	}
	u := &upstream{addr: l.Addr().String(), answer: answer}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		// Serve every connection until the listener closes
		for {
			c, err := l.Accept()
			// Stop once the listener closes
			if err != nil {
				return
			}
			go u.serve(c)
		}
	}()
	return u
}

// serve answers every query on one connection.
func (u *upstream) serve(c net.Conn) {
	defer c.Close()
	// Answer queries until the connection ends
	for {
		raw, err := readFrame(c)
		// Stop once the connection ends
		if err != nil {
			return
		}
		u.mu.Lock()
		u.got = append(u.got, raw)
		u.mu.Unlock()
		var p dnsmessage.Parser
		_, _ = p.Start(raw)
		q, _ := p.Question()
		_ = writeFrame(c, u.answer(q, raw))
	}
}

// recordAnswer builds an answer to the query that holds the one record the function adds.
func recordAnswer(add func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error) func(dnsmessage.Question, []byte) []byte {
	return func(q dnsmessage.Question, raw []byte) []byte {
		var p dnsmessage.Parser
		qh, _ := p.Start(raw)
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: qh.ID, Response: true, RecursionDesired: true, RecursionAvailable: true})
		b.EnableCompression()
		_ = b.StartQuestions()
		_ = b.Question(q)
		_ = b.StartAnswers()
		// Add the record where the test names one
		if add != nil {
			_ = add(&b, dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: answerTTL})
		}
		msg, _ := b.Finish()
		return msg
	}
}

// rcodeAnswer builds an empty answer with the response code.
func rcodeAnswer(rc dnsmessage.RCode) func(dnsmessage.Question, []byte) []byte {
	return func(q dnsmessage.Question, raw []byte) []byte {
		var p dnsmessage.Parser
		qh, _ := p.Start(raw)
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: qh.ID, Response: true, RCode: rc})
		_ = b.StartQuestions()
		_ = b.Question(q)
		msg, _ := b.Finish()
		return msg
	}
}

// addressAnswer answers every query with the fixture's address.
func addressAnswer() func(dnsmessage.Question, []byte) []byte {
	return recordAnswer(func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
		return b.AResource(h, dnsmessage.AResource{A: netip.MustParseAddr(fixture.Address).As4()})
	})
}

// servers is an upstream list a test can change between queries.
type servers struct {
	mu   sync.Mutex
	list []string
	err  error
}

// set replaces the list.
func (s *servers) set(list ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = list
}

// Nameservers hands back the list as it stands.
func (s *servers) Nameservers() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.list...), s.err
}

// home is the DNS server under test with the cage's end of one stream to it.
type home struct {
	cage net.Conn
	log  *bytes.Buffer
	done chan struct{}
}

// options are what a test changes about the server.
type options struct {
	policy   dns.Policy
	idle     time.Duration
	attempt  time.Duration
	inFlight int
}

// serving starts a DNS server that asks the upstreams in order, and hands back the cage's end of a stream to it.
func serving(t *testing.T, up dns.Upstream, o options) *home {
	t.Helper()
	var log bytes.Buffer
	s := dns.New(dns.Options{
		Policy:   o.policy,
		Upstream: up,
		Log:      slog.New(slog.NewJSONHandler(&syncWriter{w: &log}, nil)),
		Idle:     o.idle,
		Attempt:  o.attempt,
		InFlight: o.inFlight,
	})
	cage, host := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer host.Close()
		s.Handle(ctx, host)
	}()
	t.Cleanup(func() {
		cancel()
		_ = cage.Close()
		<-done
	})
	return &home{cage: cage, log: &log, done: done}
}

// syncWriter lets the server's goroutines write the log one at a time.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write writes the bytes while every other write waits.
func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}

// listing is a policy that holds the fixture's host and the wildcard below the other host.
func listing() dns.Policy {
	return &config.Config{Hosts: []config.Host{
		{Name: fixture.Host, Ports: []int{fixture.Port}},
		{Name: fixture.OtherWildcard, Ports: []int{fixture.Port}},
	}}
}

// askRaw sends the query on the stream and hands back the raw answer.
func (h *home) askRaw(t *testing.T, msg []byte) []byte {
	t.Helper()
	_ = h.cage.SetDeadline(time.Now().Add(10 * time.Second))
	// Stop when the query fails to go out
	if err := writeFrame(h.cage, msg); err != nil {
		t.Fatalf("write query = %v", err)
	}
	raw, err := readFrame(h.cage)
	// Stop when the answer fails to arrive
	if err != nil {
		t.Fatalf("read answer = %v", err)
	}
	return raw
}

// ask sends the query on the stream and hands back the answer read whole.
func (h *home) ask(t *testing.T, msg []byte) dnsmessage.Message {
	t.Helper()
	var m dnsmessage.Message
	// Stop when the answer fails to parse
	if err := m.Unpack(h.askRaw(t, msg)); err != nil {
		t.Fatalf("unpack answer = %v", err)
	}
	return m
}

// summary is what a test compares of an answer.
type summary struct {
	ID       uint16
	RCode    dnsmessage.RCode
	Answers  int
	SOATTL   []uint32
	HasOPT   bool
	Question string
}

// summarize reads the parts of an answer a test compares.
func summarize(m dnsmessage.Message) summary {
	s := summary{ID: m.ID, RCode: m.RCode, Answers: len(m.Answers)}
	// Name the question the answer carries
	if len(m.Questions) == 1 {
		s.Question = m.Questions[0].Name.String() + " " + m.Questions[0].Type.String()
	}
	// Keep the TTLs of the authority's SOA records, and its MINIMUM next to it
	for _, r := range m.Authorities {
		// Read each SOA in the authority section
		if soa, ok := r.Body.(*dnsmessage.SOAResource); ok {
			s.SOATTL = append(s.SOATTL, r.Header.TTL, soa.MinTTL)
		}
	}
	// Note an EDNS record among the additionals
	for _, r := range m.Additionals {
		// Mark the EDNS record
		if r.Header.Type == dnsmessage.TypeOPT {
			s.HasOPT = true
		}
	}
	return s
}

// Each record type of a listed name reaches the cage exactly as the nameserver sent it.
func TestDNSPassesEveryRecordTypeOfAListedName(t *testing.T) {
	long := strings.Repeat("o3s ", 100)
	records := map[string]struct {
		typ dnsmessage.Type
		add func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error
	}{
		"A": {dnsmessage.TypeA, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.AResource(h, dnsmessage.AResource{A: netip.MustParseAddr(fixture.Address).As4()})
		}},
		"AAAA": {dnsmessage.TypeAAAA, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.AAAAResource(h, dnsmessage.AAAAResource{AAAA: netip.MustParseAddr(fixture.AddressV6).As16()})
		}},
		"MX": {dnsmessage.TypeMX, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.MXResource(h, dnsmessage.MXResource{Pref: 10, MX: name(fixture.Subdomain)})
		}},
		"TXT": {dnsmessage.TypeTXT, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.TXTResource(h, dnsmessage.TXTResource{TXT: []string{long[:255], long[255:]}})
		}},
		"SRV": {dnsmessage.TypeSRV, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.SRVResource(h, dnsmessage.SRVResource{Priority: 1, Weight: 1, Port: fixture.Port, Target: name(fixture.Subdomain)})
		}},
		"CAA": {dnsmessage.Type(257), func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.UnknownResource(h, dnsmessage.UnknownResource{Type: dnsmessage.Type(257), Data: []byte("\x00\x05issue" + fixture.OtherHost)})
		}},
		"PTR": {dnsmessage.TypePTR, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.PTRResource(h, dnsmessage.PTRResource{PTR: name(fixture.Subdomain)})
		}},
		"SOA": {dnsmessage.TypeSOA, func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
			return b.SOAResource(h, dnsmessage.SOAResource{NS: name(fixture.Subdomain), MBox: name(fixture.Subdomain), Serial: 1, Refresh: 60, Retry: 60, Expire: 60, MinTTL: 60})
		}},
	}
	// Ask for each type on its own
	for label, r := range records {
		t.Run(label, func(t *testing.T) {
			ns := nameserver(t, recordAnswer(r.add))
			h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})
			want := recordAnswer(r.add)(dnsmessage.Question{Name: name(fixture.Host), Type: r.typ, Class: dnsmessage.ClassINET}, query(t, fixture.Host, r.typ))

			got := h.askRaw(t, query(t, fixture.Host, r.typ))

			// Require the answer byte for byte
			if !bytes.Equal(got, want) {
				t.Errorf("%s answer differs from the nameserver's:\n got %x\nwant %x", label, got, want)
			}
		})
	}
}

// A type a listed name lacks comes back as NODATA, telling the name exists.
func TestDNSKeepsNoDataApartFromAMissingName(t *testing.T) {
	ns := nameserver(t, recordAnswer(nil))
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})

	got := summarize(h.ask(t, query(t, fixture.Host, dnsmessage.TypeMX)))

	// Require success with an empty answer
	want := summary{ID: queryID, RCode: dnsmessage.RCodeSuccess, Question: fixture.Host + ". TypeMX"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("answer (-want +got):\n%s", diff)
	}
}

// A name the allowlist lacks gets NXDOMAIN with a root SOA that expires at once, a log line, and the nameserver
// stays unasked.
func TestDNSAnswersADeniedNameWithAnUncachedNXDOMAIN(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})

	got := summarize(h.ask(t, query(t, fixture.RefusedName, dnsmessage.TypeA)))

	want := summary{ID: queryID, RCode: dnsmessage.RCodeNameError, SOATTL: []uint32{0, 0}, Question: fixture.RefusedName + ". TypeA"}
	// Require the uncached denial and an untouched nameserver
	if diff := cmp.Diff(want, got); diff != "" || len(ns.queries()) != 0 {
		t.Errorf("denial (-want +got):\n%s\nnameserver read %d queries, want 0", diff, len(ns.queries()))
	}
}

// Every denial leaves one log line naming the name and the type.
func TestDNSLogsADenial(t *testing.T) {
	h := serving(t, &servers{}, options{policy: listing()})

	_ = h.ask(t, query(t, fixture.RefusedName, dnsmessage.TypeAAAA))

	var line map[string]any
	// Require one line that names the denied query
	if err := json.Unmarshal(h.log.Bytes(), &line); err != nil {
		t.Fatalf("log %q = %v, want one JSON line", h.log.String(), err)
	}
	delete(line, "time")
	want := map[string]any{"level": "INFO", "msg": "denied", "name": fixture.RefusedName, "type": "AAAA"}
	if diff := cmp.Diff(want, line); diff != "" {
		t.Errorf("denial log (-want +got):\n%s", diff)
	}
}

// A server still waiting for its policy denies every name.
func TestDNSDeniesEveryNameWithoutAPolicy(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{ns.addr}}, options{})

	got := h.ask(t, query(t, fixture.Host, dnsmessage.TypeA)).RCode

	// Require the denial and an untouched nameserver
	if got != dnsmessage.RCodeNameError || len(ns.queries()) != 0 {
		t.Errorf("answer without a policy = %v with %d queries upstream, want NXDOMAIN and none", got, len(ns.queries()))
	}
}

// A name matches the allowlist in any case and with any underscore label, and a wildcard leaves its apex out.
func TestDNSMatchesNamesAsTheAllowlistDoes(t *testing.T) {
	tests := map[string]dnsmessage.RCode{
		strings.ToUpper(fixture.Host):        dnsmessage.RCodeSuccess,
		"_sip._tcp.api." + fixture.OtherHost: dnsmessage.RCodeSuccess,
		"api." + fixture.OtherHost:           dnsmessage.RCodeSuccess,
		fixture.OtherHost:                    dnsmessage.RCodeNameError,
		"api." + fixture.Host:                dnsmessage.RCodeNameError,
	}
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})
	got := map[string]dnsmessage.RCode{}

	// Ask for each name in turn
	for n := range tests {
		got[n] = h.ask(t, query(t, n, dnsmessage.TypeA)).RCode
	}

	// Require each name answered as the allowlist reads it
	if diff := cmp.Diff(tests, got); diff != "" {
		t.Errorf("answers by name (-want +got):\n%s", diff)
	}
}

// A denied parent and a listed child asked in turn both get their own answer, since the denial expires at once.
func TestDNSAnswersAListedChildAfterItsDeniedParent(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})

	parent := h.ask(t, query(t, fixture.OtherHost, dnsmessage.TypeA)).RCode
	child := h.ask(t, query(t, "api."+fixture.OtherHost, dnsmessage.TypeA)).RCode

	// Require the parent denied and the child answered
	if parent != dnsmessage.RCodeNameError || child != dnsmessage.RCodeSuccess {
		t.Errorf("parent %v then child %v, want NXDOMAIN then success", parent, child)
	}
}

// Malformed and unsupported queries get the code the DNS rules give them, and the nameserver stays unasked.
func TestDNSAnswersQueriesOutsideItsRulesWithTheirCode(t *testing.T) {
	q := dnsmessage.Question{Name: name(fixture.Host), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}
	tests := map[string]struct {
		msg  []byte
		want dnsmessage.RCode
	}{
		"two questions":                {build(t, dnsmessage.Header{ID: queryID}, []dnsmessage.Question{q, q}), dnsmessage.RCodeFormatError},
		"two OPT records":              {query(t, fixture.Host, dnsmessage.TypeA, withOPT(), withOPT()), dnsmessage.RCodeFormatError},
		"a notify":                     {build(t, dnsmessage.Header{ID: queryID, OpCode: 4}, []dnsmessage.Question{q}), dnsmessage.RCodeNotImplemented},
		"a zone transfer":              {query(t, fixture.Host, dnsmessage.TypeAXFR), dnsmessage.RCodeNotImplemented},
		"an incremental zone transfer": {query(t, fixture.Host, dnsmessage.Type(251)), dnsmessage.RCodeNotImplemented},
		"no question":                  {build(t, dnsmessage.Header{ID: queryID}, nil), dnsmessage.RCodeSuccess},
	}
	// Send each query on its own
	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			ns := nameserver(t, addressAnswer())
			h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})

			m := h.ask(t, tt.msg)

			// Require the code and an untouched nameserver
			if m.RCode != tt.want || len(m.Answers) != 0 || len(ns.queries()) != 0 {
				t.Errorf("%s = %v with %d answers and %d queries upstream, want %v, none and none", label, m.RCode, len(m.Answers), len(ns.queries()), tt.want)
			}
		})
	}
}

// A message that is itself an answer is dropped in silence.
func TestDNSDropsAnAnswerSentAsAQuery(t *testing.T) {
	h := serving(t, &servers{}, options{policy: listing()})
	answer := build(t, dnsmessage.Header{ID: queryID, Response: true}, []dnsmessage.Question{{Name: name(fixture.Host), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}})

	_ = writeFrame(h.cage, answer)
	_ = writeFrame(h.cage, query(t, fixture.RefusedName, dnsmessage.TypeA))
	_ = h.cage.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw, err := readFrame(h.cage)

	// Require the first reply to answer the second message
	var m dnsmessage.Message
	if err != nil || m.Unpack(raw) != nil || m.Questions[0].Name.String() != fixture.RefusedName+"." {
		t.Errorf("first reply = %x, %v, want the answer to %s", raw, err, fixture.RefusedName)
	}
}

// Every reply the server writes itself to a query with an EDNS record carries one back.
func TestDNSEchoesEDNSInTheRepliesItWrites(t *testing.T) {
	tests := map[string][]byte{
		"a denial":        query(t, fixture.RefusedName, dnsmessage.TypeA, withOPT()),
		"a zone transfer": query(t, fixture.Host, dnsmessage.TypeAXFR, withOPT()),
		"localhost":       query(t, "localhost", dnsmessage.TypeA, withOPT()),
		"a failure":       query(t, fixture.Host, dnsmessage.TypeA, withOPT()),
	}
	// Send each query on its own
	for label, msg := range tests {
		t.Run(label, func(t *testing.T) {
			h := serving(t, &servers{}, options{policy: listing()})

			got := summarize(h.ask(t, msg)).HasOPT

			// Require the EDNS record in the reply
			if !got {
				t.Errorf("reply to %s carries no EDNS record", label)
			}
		})
	}
}

// A forwarded query reaches the nameserver byte for byte, its EDNS record included.
func TestDNSForwardsAQueryUnchanged(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})
	msg := query(t, fixture.Host, dnsmessage.TypeA, withOPT())

	_ = h.ask(t, msg)

	// Require the nameserver to read the very query
	if diff := cmp.Diff([][]byte{msg}, ns.queries()); diff != "" {
		t.Errorf("queries upstream (-want +got):\n%s", diff)
	}
}

// localhost names answer with the loopback for addresses and an empty answer otherwise, all answered on the spot.
func TestDNSAnswersLocalhostItself(t *testing.T) {
	tests := map[string]struct {
		name string
		typ  dnsmessage.Type
		want string
	}{
		"A":     {"localhost", dnsmessage.TypeA, "127.0.0.1"},
		"AAAA":  {"localhost", dnsmessage.TypeAAAA, "::1"},
		"below": {"app.localhost", dnsmessage.TypeA, "127.0.0.1"},
		"MX":    {"localhost", dnsmessage.TypeMX, ""},
	}
	// Ask for each on its own
	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			ns := nameserver(t, addressAnswer())
			h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})

			m := h.ask(t, query(t, tt.name, tt.typ))

			got := ""
			// Read the address the answer gives
			for _, r := range m.Answers {
				// Take the address of each kind
				switch b := r.Body.(type) {
				case *dnsmessage.AResource:
					got = netip.AddrFrom4(b.A).String()
				case *dnsmessage.AAAAResource:
					got = netip.AddrFrom16(b.AAAA).String()
				}
			}
			// Require the loopback or an empty success, and an untouched nameserver
			if m.RCode != dnsmessage.RCodeSuccess || got != tt.want || len(ns.queries()) != 0 {
				t.Errorf("%s %s = %v %q with %d queries upstream, want success %q and none", tt.name, tt.typ, m.RCode, got, len(ns.queries()), tt.want)
			}
		})
	}
}

// Two queries sent back to back on one stream each get their own answer.
func TestDNSAnswersPipelinedQueries(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})
	first := build(t, dnsmessage.Header{ID: 1, RecursionDesired: true}, []dnsmessage.Question{{Name: name(fixture.Host), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}})
	second := build(t, dnsmessage.Header{ID: 2, RecursionDesired: true}, []dnsmessage.Question{{Name: name(fixture.RefusedName), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}})

	_ = h.cage.SetDeadline(time.Now().Add(10 * time.Second))
	_ = writeFrame(h.cage, first)
	_ = writeFrame(h.cage, second)
	got := map[uint16]dnsmessage.RCode{}
	// Read both answers in whatever order they come
	for range 2 {
		raw, err := readFrame(h.cage)
		var m dnsmessage.Message
		// Stop when an answer is missing
		if err != nil || m.Unpack(raw) != nil {
			t.Fatalf("read answer = %v", err)
		}
		got[m.ID] = m.RCode
	}

	// Require each query its own answer
	want := map[uint16]dnsmessage.RCode{1: dnsmessage.RCodeSuccess, 2: dnsmessage.RCodeNameError}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("answers by ID (-want +got):\n%s", diff)
	}
}

// An answer larger than a UDP datagram's classic limit reaches the cage whole.
func TestDNSCarriesALargeAnswerWhole(t *testing.T) {
	addrs := netip.MustParseAddr(fixture.AddressV6)
	ns := nameserver(t, recordAnswer(func(b *dnsmessage.Builder, h dnsmessage.ResourceHeader) error {
		// Add enough addresses to pass 512 bytes
		for range 40 {
			addrs = addrs.Next()
			// Stop at the first record that fails to add
			if err := b.AAAAResource(h, dnsmessage.AAAAResource{AAAA: addrs.As16()}); err != nil {
				return err
			}
		}
		return nil
	}))
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing()})

	raw := h.askRaw(t, query(t, fixture.Host, dnsmessage.TypeAAAA))

	var m dnsmessage.Message
	// Require every record of the large answer
	if err := m.Unpack(raw); err != nil || len(raw) <= 512 || len(m.Answers) != 40 {
		t.Errorf("answer of %d bytes with %d records, %v, want over 512 bytes and 40 records", len(raw), len(m.Answers), err)
	}
}

// A nameserver that is down gives way to the next one in the list.
func TestDNSFailsOverToTheNextNameserver(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{deadAddr(t), ns.addr}}, options{policy: listing(), attempt: 500 * time.Millisecond})

	got := h.ask(t, query(t, fixture.Host, dnsmessage.TypeA))

	// Require the second nameserver's answer
	if got.RCode != dnsmessage.RCodeSuccess || len(got.Answers) != 1 {
		t.Errorf("answer = %v with %d records, want success with one", got.RCode, len(got.Answers))
	}
}

// A nameserver that fails the query gives way to the next one, which answers.
func TestDNSFailsOverPastAFailingAnswer(t *testing.T) {
	failing := nameserver(t, rcodeAnswer(dnsmessage.RCodeServerFailure))
	ns := nameserver(t, addressAnswer())
	h := serving(t, &servers{list: []string{failing.addr, ns.addr}}, options{policy: listing()})

	got := h.ask(t, query(t, fixture.Host, dnsmessage.TypeA))

	// Require the second nameserver's answer
	if got.RCode != dnsmessage.RCodeSuccess || len(got.Answers) != 1 {
		t.Errorf("answer = %v with %d records, want success with one", got.RCode, len(got.Answers))
	}
}

// When every nameserver is down the cage hears a failure within the time the client waits.
func TestDNSFailsWithinTheClientsWait(t *testing.T) {
	h := serving(t, &servers{list: []string{deadAddr(t), deadAddr(t)}}, options{policy: listing(), attempt: 300 * time.Millisecond})
	start := time.Now()

	got := h.ask(t, query(t, fixture.Host, dnsmessage.TypeA)).RCode

	// Require a failure well inside five seconds
	if took := time.Since(start); got != dnsmessage.RCodeServerFailure || took > 5*time.Second {
		t.Errorf("answer = %v after %v, want SERVFAIL within 5s", got, took)
	}
}

// With an empty nameserver list the cage hears a failure, and the server stays with the host's own resolvers.
func TestDNSFailsWithoutANameserver(t *testing.T) {
	h := serving(t, &servers{}, options{policy: listing()})

	got := h.ask(t, query(t, fixture.Host, dnsmessage.TypeA)).RCode

	// Require the failure
	if got != dnsmessage.RCodeServerFailure {
		t.Errorf("answer with no nameserver = %v, want SERVFAIL", got)
	}
}

// A change to the nameserver list applies from the next query on.
func TestDNSFollowsTheNameserversAsTheyChange(t *testing.T) {
	before := nameserver(t, addressAnswer())
	after := nameserver(t, addressAnswer())
	up := &servers{list: []string{before.addr}}
	h := serving(t, up, options{policy: listing()})
	_ = h.ask(t, query(t, fixture.Host, dnsmessage.TypeA))

	up.set(after.addr)
	_ = h.ask(t, query(t, fixture.Host, dnsmessage.TypeA))

	// Require one query at each nameserver
	if got := []int{len(before.queries()), len(after.queries())}; !cmp.Equal(got, []int{1, 1}) {
		t.Errorf("queries before and after the change = %v, want [1 1]", got)
	}
}

// A stream that stays idle past the idle period ends.
func TestDNSEndsAnIdleStream(t *testing.T) {
	h := serving(t, &servers{}, options{policy: listing(), idle: 100 * time.Millisecond})

	// Require the server to end the stream soon after
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream stays open past its idle period")
	}
}

// A query that trickles in slower than the idle period ends the stream instead of holding it.
func TestDNSEndsAStreamThatTricklesAQuery(t *testing.T) {
	h := serving(t, &servers{}, options{policy: listing(), idle: 200 * time.Millisecond})
	msg := query(t, fixture.RefusedName, dnsmessage.TypeA)
	frame := frameOf(msg)

	go func() {
		// Send one byte at a time, each well inside the idle period
		for _, b := range frame {
			// Stop once the server ends the stream
			if _, err := h.cage.Write([]byte{b}); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Require the stream to end before the whole query arrives
	select {
	case <-h.done:
	case <-time.After(time.Duration(len(frame)) * 50 * time.Millisecond):
		t.Fatal("the trickled query keeps the stream open")
	}
}

// deadAddr is a loopback address left free.
func deadAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	// Stop when the port stays taken
	if err != nil {
		t.Fatalf("Setup: Listen = %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// Queries past the cap on one stream wait their turn and are all answered.
func TestDNSQueuesQueriesPastItsCap(t *testing.T) {
	slow := func(q dnsmessage.Question, raw []byte) []byte {
		time.Sleep(50 * time.Millisecond)
		return addressAnswer()(q, raw)
	}
	ns := nameserver(t, slow)
	h := serving(t, &servers{list: []string{ns.addr}}, options{policy: listing(), inFlight: 1})
	_ = h.cage.SetDeadline(time.Now().Add(10 * time.Second))

	msgs := make([][]byte, 4)
	// Build the queries, each with its own ID
	for id := range msgs {
		msgs[id] = build(t, dnsmessage.Header{ID: uint16(id), RecursionDesired: true}, []dnsmessage.Question{{Name: name(fixture.Host), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}})
	}
	go func() {
		// Send the queries back to back while the answers come in
		for _, m := range msgs {
			_ = writeFrame(h.cage, m)
		}
	}()
	got := 0
	// Read an answer for each
	for range 4 {
		raw, err := readFrame(h.cage)
		var m dnsmessage.Message
		// Count each successful answer
		if err == nil && m.Unpack(raw) == nil && m.RCode == dnsmessage.RCodeSuccess {
			got++
		}
	}

	// Require every query answered
	if got != 4 {
		t.Errorf("%d of 4 queries answered, want all", got)
	}
}
