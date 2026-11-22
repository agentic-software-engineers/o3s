package proxy

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// The sites of a request a host may carry its secret in.
const (
	// Header carries the secret in a header, in the clear or inside a Basic credential.
	Header = "header"
	// Query carries the secret in the address's query.
	Query = "query"
	// Body carries the secret in the request's body.
	Body = "body"
)

// Secret is the value a host carries and the sites it carries it in.
type Secret struct {
	// Value is the real secret, empty while it waits to be filled in.
	Value string
	// Sites are where in a request the host takes the secret.
	Sites []string
}

// maxSwapBody is the largest body the proxy reads whole to swap a secret into.
const maxSwapBody = 8 << 20

// minCore is the shortest base64 form worth masking, below which a match is likely chance.
const minCore = 12

var (
	// errBodyTooLarge reports a body too large to read whole for a secret.
	errBodyTooLarge = errors.New("body too large to carry a secret")
	// errUnreadableEncoding reports an answer whose encoding hides its content from the masking.
	errUnreadableEncoding = errors.New("answer in an encoding the proxy cannot read")
)

// swap puts the secret where the marker stands, in each site the host carries it in.
func swap(r *http.Request, marker string, sec Secret) error {
	// Leave the request as sent while the marker is empty
	if marker == "" {
		return nil
	}
	// Look in every site the host carries its secret in
	for _, site := range sec.Sites {
		// Swap the marker in the site's own way
		switch site {
		case Header:
			swapHeaders(r.Header, marker, sec.Value)
		case Query:
			r.URL.RawQuery = strings.ReplaceAll(r.URL.RawQuery, marker, url.QueryEscape(sec.Value))
		case Body:
			// Hand back a body too large to swap in
			if err := swapBody(r, marker, sec.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// swapHeaders swaps the marker in every header value, in the clear or inside a Basic credential.
func swapHeaders(h http.Header, marker, value string) {
	// Look at every value of every header
	for _, vs := range h {
		// Swap each value on its own
		for i, v := range vs {
			vs[i] = swapHeader(v, marker, value)
		}
	}
}

// swapHeader swaps the marker in one header value, in the clear or inside a Basic credential.
func swapHeader(v, marker, value string) string {
	// Swap a marker in the clear
	if strings.Contains(v, marker) {
		return strings.ReplaceAll(v, marker, value)
	}
	scheme, blob, ok := strings.Cut(strings.TrimSpace(v), " ")
	// Leave every other scheme as sent
	if !ok || !strings.EqualFold(scheme, "basic") {
		return v
	}
	blob = strings.TrimSpace(blob)
	raw, err := base64.StdEncoding.DecodeString(blob + strings.Repeat("=", (4-len(blob)%4)%4))
	// Leave a credential as sent when its decoded form lacks the marker
	if err != nil || !bytes.Contains(raw, []byte(marker)) {
		return v
	}
	return scheme + " " + base64.StdEncoding.EncodeToString(bytes.ReplaceAll(raw, []byte(marker), []byte(value)))
}

// swapBody reads the body whole and swaps the marker in it.
func swapBody(r *http.Request, marker, value string) error {
	// Leave an empty or encoded body as sent
	if r.Body == nil || r.Body == http.NoBody || r.Header.Get("Content-Encoding") != "" {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxSwapBody+1))
	_ = r.Body.Close()
	// Turn away a body that fails or outgrows the limit
	if err != nil || len(b) > maxSwapBody {
		return fmt.Errorf("%w: over %d bytes", errBodyTooLarge, maxSwapBody)
	}
	b = bytes.ReplaceAll(b, []byte(marker), []byte(value))
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	return nil
}

// secretForms lists the secret as an answer can carry it: itself, and inside any base64 that holds it. Base64 packs
// three bytes into four characters, so the secret lands on one of three alignments, and the middle of each encoding
// is fixed by the secret alone.
func secretForms(secret string) [][]byte {
	// Hand back an empty list while the secret waits for its value
	if secret == "" {
		return nil
	}
	forms := [][]byte{[]byte(secret)}
	// Encode the secret on each alignment
	for pad := range 3 {
		enc := base64.StdEncoding.EncodeToString(append(make([]byte, pad), secret...))
		start := 0
		// Drop the group the bytes before share
		if pad > 0 {
			start = 4
		}
		// Keep a core long enough to match by more than chance
		if core := enc[start : len(enc)-4]; len(core) >= minCore {
			forms = append(forms, []byte(core))
		}
	}
	return forms
}

// mask hides the secret in every header and the body of an answer from the host.
func (p *Proxy) mask(resp *http.Response, host string, forms [][]byte) error {
	// Leave the answer as sent while the secret waits for its value
	if len(forms) == 0 {
		return nil
	}
	// Mask each header value
	for _, vs := range resp.Header {
		// Mask each value on its own
		for i, v := range vs {
			vs[i] = string(maskAll([]byte(v), forms))
		}
	}
	// Leave a bodiless answer as sent
	if resp.Body == nil || resp.Body == http.NoBody {
		return nil
	}
	// Turn away a body in an encoding beyond the one the transport unpacks itself
	if enc := strings.ToLower(resp.Header.Get("Content-Encoding")); enc != "" && enc != "identity" {
		p.log.Warn("unmasked", "host", host, "encoding", enc)
		return fmt.Errorf("%w: %s", errUnreadableEncoding, enc)
	}
	resp.Body = &masker{src: resp.Body, forms: forms}
	return nil
}

// maskAll replaces every form in the bytes with filler of the same length.
func maskAll(b []byte, forms [][]byte) []byte {
	// Replace each form in turn
	for _, f := range forms {
		b = bytes.ReplaceAll(b, f, bytes.Repeat([]byte("*"), len(f)))
	}
	return b
}

// masker reads a body and masks the secret in it, holding back a tail that could open a form until the next read
// shows whether it does.
type masker struct {
	src   io.ReadCloser
	forms [][]byte
	ready []byte
	held  []byte
	err   error
	buf   [32 << 10]byte
}

// Read hands over masked bytes, reading more from the body as needed.
func (m *masker) Read(p []byte) (int, error) {
	// Read until there is something to hand over or the body ends
	for len(m.ready) == 0 {
		// Hand back the end once all is handed over
		if m.err != nil {
			return 0, m.err
		}
		n, err := m.src.Read(m.buf[:])
		data := maskAll(append(m.held, m.buf[:n]...), m.forms)
		keep := 0
		// Hold back a tail that could open a form while more may follow
		if err == nil {
			keep = overlap(data, m.forms)
		}
		m.ready, m.held, m.err = data[:len(data)-keep], data[len(data)-keep:], err
	}
	n := copy(p, m.ready)
	m.ready = m.ready[n:]
	return n, nil
}

// Close closes the body beneath.
func (m *masker) Close() error {
	return m.src.Close()
}

// overlap is the length of the longest tail of the bytes that could open one of the forms.
func overlap(b []byte, forms [][]byte) int {
	most := 0
	// Look at each form
	for _, f := range forms {
		// Take the longest tail first, which is the most that could still be held back
		for k := min(len(b), len(f)-1); k > most; k-- {
			// Keep the tail that opens the form
			if bytes.HasSuffix(b, f[:k]) {
				most = k
				break
			}
		}
	}
	return most
}
