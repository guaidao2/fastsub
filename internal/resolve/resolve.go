// Package resolve answers two questions about a name: what does it point at,
// and does the zone answer for names that were never issued. The second is what
// keeps a wordlist from manufacturing thousands of results that look real.
package resolve

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DefaultServers are the resolvers a run uses when none were configured.
//
// They are ordered by how likely they are to answer from where a scan usually
// runs, and the resolver pool gives up on one that fails rather than asking it
// again for every name. The DoH endpoint is last on purpose: it works where UDP
// to port 53 is filtered or rewritten, and it is slower than the plain ones.
var DefaultServers = []string{
	"223.5.5.5:53",                     // AliDNS
	"119.29.29.29:53",                  // DNSPod
	"8.8.8.8:53",                       // Google
	"1.1.1.1:53",                       // Cloudflare, most often blocked
	"https://dns.alidns.com/dns-query", // DoH, for networks that mangle UDP/53
}

// server is one upstream: a DNS resolver, or a DNS-over-HTTPS endpoint.
type server struct {
	address string // host:port for DNS, the full URL for DoH
	doh     bool
}

// parseServer reads a resolver line or a --doh value. A bare address gets the
// standard port, because nobody types :53 twice.
func parseServer(spec string) server {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://") {
		return server{address: spec, doh: true}
	}
	if !strings.Contains(spec, ":") {
		spec += ":53"
	}
	return server{address: spec}
}

// Options configures a Resolver.
type Options struct {
	// Servers is the resolver list, host:port. DefaultServers when empty.
	Servers []string
	// Timeout is the per-query timeout.
	Timeout time.Duration
	// Concurrency bounds how many queries are in flight at once.
	Concurrency int
	// Attempts is how many resolvers one lookup may try.
	Attempts int
}

// Result is what a name resolved to.
type Result struct {
	IPs   []string
	CNAME string
}

// Found reports whether the name resolved to anything.
func (r Result) Found() bool { return len(r.IPs) > 0 || r.CNAME != "" }

// Resolver queries a pool of DNS servers.
type Resolver struct {
	servers  []server
	timeout  time.Duration
	attempts int
	client   *dns.Client
	sem      chan struct{}

	mu       sync.Mutex
	next     int
	failures map[string]int // consecutive failures, reset by a success
}

// New returns a resolver. A zero concurrency means 100.
func New(opts Options) *Resolver {
	raw := opts.Servers
	if len(raw) == 0 {
		raw = DefaultServers
	}
	servers := make([]server, 0, len(raw))
	for _, spec := range raw {
		servers = append(servers, parseServer(spec))
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 100
	}
	attempts := opts.Attempts
	if attempts <= 0 {
		attempts = 2
	}

	return &Resolver{
		servers:  servers,
		timeout:  timeout,
		attempts: attempts,
		client:   &dns.Client{Timeout: timeout, Net: "udp"},
		sem:      make(chan struct{}, concurrency),
		failures: make(map[string]int),
	}
}

// Lookup resolves a name to its addresses and its canonical name.
//
// An address is already its own answer: asking DNS about "10.0.0.1" is a
// question with no useful reply, and a scan that starts from a port scanner's
// output is a normal way to use this. Those come back as they were given.
//
// NXDOMAIN is not an error: "this name does not exist" is the answer, and the
// caller decides what it means. Only a transport failure on every resolver
// comes back as an error, because that is a fact about the network rather than
// about the name.
func (r *Resolver) Lookup(ctx context.Context, name string) (Result, error) {
	if ip := net.ParseIP(name); ip != nil {
		return Result{IPs: []string{name}}, nil
	}

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}

	msg, err := r.query(ctx, name, dns.TypeA)
	if err != nil {
		return Result{}, err
	}
	res := readAnswer(msg)

	// An NXDOMAIN settles every record type at once, and a name with an A
	// answer has already proven it exists. Only NODATA leaves a question open
	// worth a second query, which halves the load on public resolvers.
	if res.Found() || msg.Rcode == dns.RcodeNameError {
		return finish(res), nil
	}

	if msg2, err := r.query(ctx, name, dns.TypeAAAA); err == nil {
		res = merge(res, readAnswer(msg2))
	}
	return finish(res), nil
}

// query asks one resolver, moving to another when one fails.
func (r *Resolver) query(ctx context.Context, name string, qtype uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.RecursionDesired = true

	var lastErr error
	for i := 0; i < r.attempts && i < len(r.servers); i++ {
		srv := r.pick()

		qctx, cancel := context.WithTimeout(ctx, r.timeout)
		var resp *dns.Msg
		var err error
		if srv.doh {
			resp, err = r.queryDoH(qctx, srv.address, m)
		} else {
			resp, _, err = r.client.ExchangeContext(qctx, m, srv.address)
		}
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			r.noteFailure(srv.address)
			continue
		}
		if resp == nil {
			lastErr = errors.New("empty response")
			r.noteFailure(srv.address)
			continue
		}
		if resp.Rcode == dns.RcodeServerFailure || resp.Rcode == dns.RcodeRefused {
			lastErr = fmt.Errorf("%s answered %s", srv.address, dns.RcodeToString[resp.Rcode])
			r.noteFailure(srv.address)
			continue
		}

		r.noteSuccess(srv.address)
		// NXDOMAIN and NODATA are answers, not failures.
		return resp, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no resolver available")
	}
	return nil, lastErr
}

// queryDoH sends one query to a DNS-over-HTTPS endpoint. RFC 8484 puts the same
// wire format that would have gone over UDP into the body of a POST, so nothing
// but the transport changes — which is the point of having it as a fallback.
func (r *Resolver) queryDoH(ctx context.Context, endpoint string, m *dns.Msg) (*dns.Msg, error) {
	packed, err := m.Pack()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(packed))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := dohHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	out := new(dns.Msg)
	if err := out.Unpack(body); err != nil {
		return nil, fmt.Errorf("cannot read the DoH answer: %w", err)
	}
	return out, nil
}

// dohHTTP has no timeout of its own: the query context already carries one, and
// a second deadline would only race the first.
var dohHTTP = &http.Client{}

// pick returns the resolver to use next: one of those with the fewest
// consecutive failures, rotating between them. A server that cannot be reached
// from here stops being asked after its first timeout instead of costing a
// timeout on every name.
func (r *Resolver) pick() server {
	r.mu.Lock()
	defer r.mu.Unlock()

	best := -1
	for _, s := range r.servers {
		f := r.failures[s.address]
		if best == -1 || f < best {
			best = f
		}
	}

	pool := make([]server, 0, len(r.servers))
	for _, s := range r.servers {
		if r.failures[s.address] == best {
			pool = append(pool, s)
		}
	}

	s := pool[r.next%len(pool)]
	r.next++
	return s
}

func (r *Resolver) noteFailure(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[address]++
}

func (r *Resolver) noteSuccess(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failures[address] != 0 {
		r.failures[address] = 0
	}
}

// Failures reports how many consecutive failures each resolver has had, which
// is what --verbose prints when a run is unexpectedly slow.
func (r *Resolver) Failures() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.failures))
	for k, v := range r.failures {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

// readAnswer pulls the addresses and the canonical name out of a response.
func readAnswer(msg *dns.Msg) Result {
	var res Result
	for _, rr := range msg.Answer {
		switch v := rr.(type) {
		case *dns.A:
			res.IPs = append(res.IPs, v.A.String())
		case *dns.AAAA:
			res.IPs = append(res.IPs, v.AAAA.String())
		case *dns.CNAME:
			// The last CNAME in the chain is the one that matters.
			res.CNAME = strings.TrimSuffix(v.Target, ".")
		}
	}
	return res
}

func merge(a, b Result) Result {
	a.IPs = append(a.IPs, b.IPs...)
	if b.CNAME != "" {
		a.CNAME = b.CNAME
	}
	return a
}

func finish(res Result) Result {
	res.IPs = dedupe(res.IPs)
	sort.Strings(res.IPs)
	return res
}

// Wildcard is a zone that answers for every name under it.
type Wildcard struct {
	// Domain is the zone the wildcard was found in.
	Domain string
	// IPs are the addresses a nonexistent name was given.
	IPs []string
	// CNAME is the canonical name those answers pointed at, when there was one.
	CNAME string
}

// Covers reports whether a result only exists because of this wildcard: every
// address it has was handed out to a name that does not exist either.
//
// A real host pointed at the same address as the wildcard is indistinguishable
// from a wildcard answer by address alone, so it is marked as wildcard-sourced
// rather than silently dropped — the flag is on the record, and --only-passive
// or --no-wildcard-filter keeps it in the output.
func (w *Wildcard) Covers(res Result) bool {
	if w == nil || len(w.IPs) == 0 || len(res.IPs) == 0 {
		return false
	}
	known := make(map[string]bool, len(w.IPs))
	for _, ip := range w.IPs {
		known[ip] = true
	}
	for _, ip := range res.IPs {
		if !known[ip] {
			return false
		}
	}
	return true
}

// DetectWildcard probes a zone with random labels. A zone that answers for all
// of them answers for anything, and every name a wordlist would invent is
// therefore worthless against it.
//
// The test is deliberately conservative: all probes must resolve, or the zone
// is treated as having no wildcard. Missing one costs a noisy result; inventing
// one costs every real name that shares an address with it.
func (r *Resolver) DetectWildcard(ctx context.Context, domain string) (*Wildcard, error) {
	const probes = 3

	w := &Wildcard{Domain: domain}
	answered := 0
	for i := 0; i < probes; i++ {
		label, err := randomLabel()
		if err != nil {
			return nil, err
		}
		res, err := r.Lookup(ctx, label+"."+domain)
		if err != nil {
			continue
		}
		if res.Found() {
			answered++
			w.IPs = append(w.IPs, res.IPs...)
			if res.CNAME != "" {
				w.CNAME = res.CNAME
			}
		}
	}

	if answered < probes {
		return nil, nil
	}
	w.IPs = dedupe(w.IPs)
	sort.Strings(w.IPs)
	return w, nil
}

// randomLabel returns a label no zone could plausibly have issued.
func randomLabel() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
