package dns

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// errMismatch reports an answer that belongs to another query.
var errMismatch = errors.New("answer for another query")

// forward asks the host's nameservers in order, each within its attempt and all within the budget, and hands back the
// first answer that settles the query, or else the last failing answer, or else a failure of the server's own.
func (s *Server) forward(ctx context.Context, q query, question *dnsmessage.Question, raw []byte) []byte {
	var list []string
	// Read the nameservers afresh for every query, so a network change applies at once
	if s.o.Upstream != nil {
		list, _ = s.o.Upstream.Nameservers()
	}
	ctx, cancel := context.WithTimeout(ctx, s.o.Budget)
	defer cancel()
	var last []byte
	// Ask each nameserver in turn while the budget lasts
	for _, addr := range list {
		// Stop once the budget runs out
		if ctx.Err() != nil {
			break
		}
		answer, err := s.exchange(ctx, addr, raw, q.header.ID, *question)
		// Move on from a nameserver that stays silent
		if err != nil {
			continue
		}
		// Keep a failing answer and move on, so a later nameserver may settle the query
		if failing(answer) {
			last = answer
			continue
		}
		return answer
	}
	// Hand back the last failing answer a nameserver gave
	if last != nil {
		return last
	}
	return q.reply(dnsmessage.RCodeServerFailure, question, nil)
}

// exchange sends the query to one nameserver over TCP and hands back its answer, once the answer matches the query.
func (s *Server) exchange(ctx context.Context, addr string, raw []byte, id uint16, question dnsmessage.Question) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.o.Attempt)
	defer cancel()
	c, err := s.o.Dial(ctx, "tcp", addr)
	// Hand back a nameserver out of reach
	if err != nil {
		return nil, err
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	// Hand back a query that fails to go out
	if err := writeFrame(c, raw); err != nil {
		return nil, err
	}
	answer, err := readFrame(c)
	// Hand back an answer that fails to arrive
	if err != nil {
		return nil, err
	}
	// Take the answer once it proves to be for this very query
	if !matches(answer, id, question) {
		return nil, errMismatch
	}
	return answer, nil
}

// matches reports whether an answer carries the query's ID and its question.
func matches(answer []byte, id uint16, question dnsmessage.Question) bool {
	var p dnsmessage.Parser
	h, err := p.Start(answer)
	// Turn away an answer whose header fails to parse, or that is itself a query
	if err != nil || !h.Response || h.ID != id {
		return false
	}
	got, err := p.Question()
	return err == nil && got.Type == question.Type && got.Class == question.Class &&
		strings.EqualFold(got.Name.String(), question.Name.String())
}

// failing reports whether an answer is a failure another nameserver may mend.
func failing(answer []byte) bool {
	var p dnsmessage.Parser
	h, err := p.Start(answer)
	return err != nil || h.RCode == dnsmessage.RCodeServerFailure || h.RCode == dnsmessage.RCodeNotImplemented ||
		h.RCode == dnsmessage.RCodeRefused
}
