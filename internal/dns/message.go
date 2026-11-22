package dns

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// typeIXFR is the incremental zone transfer, which dnsmessage leaves unnamed.
const typeIXFR dnsmessage.Type = 251

// ednsSize is the message size the server's own EDNS record names.
const ednsSize = 1232

// query is what the server reads of a message before it decides how to answer.
type query struct {
	header    dnsmessage.Header
	questions []dnsmessage.Question
	opts      int
	dnssecOK  bool
}

// answer decides how to treat one message from the cage and hands back the reply, or nil for a message it drops.
func (s *Server) answer(ctx context.Context, raw []byte) []byte {
	q, ok, err := read(raw)
	// Drop a message too short to be one, or one that is itself an answer
	if !ok {
		return nil
	}
	// Turn away a message whose sections fail to parse, or that carries more than one EDNS record
	if err != nil || q.opts > 1 {
		return q.reply(dnsmessage.RCodeFormatError, nil, nil)
	}
	// Turn away every operation beyond the standard query
	if q.header.OpCode != 0 {
		return q.reply(dnsmessage.RCodeNotImplemented, q.first(), nil)
	}
	// Turn away a query that asks several questions at once
	if len(q.questions) > 1 {
		return q.reply(dnsmessage.RCodeFormatError, nil, nil)
	}
	// Answer a query that asks zero questions on the spot, as a valid query with an empty answer
	if len(q.questions) == 0 {
		return q.reply(dnsmessage.RCodeSuccess, nil, nil)
	}
	question := q.questions[0]
	// Turn away zone transfers, which belong to the nameservers of a zone
	if question.Type == dnsmessage.TypeAXFR || question.Type == typeIXFR {
		return q.reply(dnsmessage.RCodeNotImplemented, &question, nil)
	}
	name := strings.ToLower(strings.TrimSuffix(question.Name.String(), "."))
	// Answer localhost names on the spot with the loopback
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return q.reply(dnsmessage.RCodeSuccess, &question, loopback(question))
	}
	// Answer a name outside the allowlist as missing, in a form every resolver drops at once
	if s.o.Policy == nil || !s.o.Policy.Lists(name) {
		s.o.Log.Info("denied", "name", name, "type", strings.TrimPrefix(question.Type.String(), "Type"))
		return q.reply(dnsmessage.RCodeNameError, &question, nil)
	}
	return s.forward(ctx, q, &question, raw)
}

// read parses a message from the cage, reporting false for one to drop and an error for one that is malformed.
func read(raw []byte) (query, bool, error) {
	var p dnsmessage.Parser
	h, err := p.Start(raw)
	// Drop a message too short for its header, or one that is itself an answer
	if err != nil || h.Response {
		return query{}, false, nil
	}
	q := query{header: h}
	q.questions, err = p.AllQuestions()
	// Report questions that fail to parse
	if err != nil {
		return q, true, err
	}
	// Report answer and authority sections that fail to parse
	if err := p.SkipAllAnswers(); err != nil {
		return q, true, err
	}
	// Report an authority section that fails to parse
	if err := p.SkipAllAuthorities(); err != nil {
		return q, true, err
	}
	// Count the EDNS records among the additionals, keeping the DNSSEC bit of the first
	for {
		rh, err := p.AdditionalHeader()
		// Stop at the end of the section
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			return q, true, nil
		}
		// Report an additional that fails to parse
		if err != nil {
			return q, true, err
		}
		// Note each EDNS record
		if rh.Type == dnsmessage.TypeOPT {
			q.dnssecOK = q.dnssecOK || q.opts == 0 && rh.DNSSECAllowed()
			q.opts++
		}
		// Report an additional whose body fails to skip
		if err := p.SkipAdditional(); err != nil {
			return q, true, err
		}
	}
}

// first hands back the first question, or nil for a message with zero questions.
func (q query) first() *dnsmessage.Question {
	// Hand back nil for a message with zero questions
	if len(q.questions) == 0 {
		return nil
	}
	return &q.questions[0]
}

// reply builds the server's own answer: the code, the question echoed where one is given, the records the function
// adds, a root SOA that keeps a missing name from being kept, and an EDNS record where the query carried one.
func (q query) reply(rc dnsmessage.RCode, question *dnsmessage.Question, add func(*dnsmessage.Builder) error) []byte {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID:                 q.header.ID,
		Response:           true,
		OpCode:             q.header.OpCode,
		RecursionDesired:   q.header.RecursionDesired,
		RecursionAvailable: true,
		RCode:              rc,
	})
	b.EnableCompression()
	_ = b.StartQuestions()
	// Echo the question the answer is for
	if question != nil {
		_ = b.Question(*question)
	}
	_ = b.StartAnswers()
	// Add the records the answer carries
	if add != nil {
		_ = add(&b)
	}
	_ = b.StartAuthorities()
	// Prove a missing name with a root SOA whose TTL and minimum let the answer expire at once
	if rc == dnsmessage.RCodeNameError {
		_ = b.SOAResource(dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName("."), Class: dnsmessage.ClassINET},
			dnsmessage.SOAResource{NS: dnsmessage.MustNewName("."), MBox: dnsmessage.MustNewName(".")})
	}
	_ = b.StartAdditionals()
	// Answer an EDNS query with an EDNS record of the server's own
	if q.opts > 0 {
		var h dnsmessage.ResourceHeader
		_ = h.SetEDNS0(ednsSize, dnsmessage.RCodeSuccess, q.dnssecOK)
		_ = b.OPTResource(h, dnsmessage.OPTResource{})
	}
	msg, _ := b.Finish()
	return msg
}

// loopback adds the loopback address a localhost name has for an address query, and leaves the answer empty for
// every other type.
func loopback(question dnsmessage.Question) func(*dnsmessage.Builder) error {
	h := dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET}
	return func(b *dnsmessage.Builder) error {
		// Add the IPv4 loopback for an IPv4 address query
		if question.Type == dnsmessage.TypeA {
			return b.AResource(h, dnsmessage.AResource{A: netip.MustParseAddr("127.0.0.1").As4()})
		}
		// Add the IPv6 loopback for an IPv6 address query
		if question.Type == dnsmessage.TypeAAAA {
			return b.AAAAResource(h, dnsmessage.AAAAResource{AAAA: netip.IPv6Loopback().As16()})
		}
		return nil
	}
}
