// Package verify probes hosts over HTTP and HTTPS. A DNS answer says a name
// exists; only a response says something is serving it, and the difference is
// what a scan is really asking about.
package verify

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/guaidao2/fastsub/internal/model"
	"github.com/guaidao2/fastsub/internal/version"
)

// DefaultPorts are probed when --ports was not given.
var DefaultPorts = []int{80, 443}

// bodyLimit is how much of a page is read while looking for its title. A title
// lives in the head, and reading further would download sites for no reason.
const bodyLimit = 64 << 10

// Options configures a Prober.
type Options struct {
	// Ports to probe. DefaultPorts when empty.
	Ports []int
	// Timeout is the whole-request budget for one probe.
	Timeout time.Duration
	// Concurrency bounds probes in flight at once.
	Concurrency int
	// Title reads the page title.
	Title bool
	// Favicon fingerprints /favicon.ico, which identifies the application
	// rather than the host.
	Favicon bool
	// Redirects follows redirects. The address finally reached is recorded on
	// the result either way, so a http-to-https hop is visible rather than lost.
	Redirects bool
	// CertSAN collects the subject alternative names from the certificate seen.
	CertSAN bool
	// UserAgent identifies the prober.
	UserAgent string
}

// Result is what answered for one host.
type Result struct {
	// URLs are the endpoints that answered, ordered by port then scheme.
	URLs []model.URL
	// SANs are the names in the certificates seen, when CertSAN was on.
	SANs []string
}

// Alive reports whether anything answered.
func (r Result) Alive() bool { return len(r.URLs) > 0 }

// Prober probes hosts. It is safe for concurrent use.
type Prober struct {
	opts     Options
	follow   *http.Client
	noFollow *http.Client
	sem      chan struct{}
}

// New returns a prober with the defaults filled in.
func New(opts Options) *Prober {
	if len(opts.Ports) == 0 {
		opts.Ports = DefaultPorts
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 50
	}
	if opts.UserAgent == "" {
		opts.UserAgent = fmt.Sprintf("%s/%s (+%s)", version.Name, version.Version, version.Repo)
	}

	follow := newClient(opts.Timeout, true)
	noFollow := newClient(opts.Timeout, false)

	return &Prober{
		opts:     opts,
		follow:   follow,
		noFollow: noFollow,
		sem:      make(chan struct{}, opts.Concurrency),
	}
}

func newClient(timeout time.Duration, follow bool) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: timeout}).DialContext,
		// A host that answers on a self-signed certificate is alive, which is
		// the question here; whether the certificate is trusted is a different
		// one, and one the certificate itself is recorded for.
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   4,
		ForceAttemptHTTP2:     true,
	}

	c := &http.Client{Transport: transport, Timeout: timeout}
	if !follow {
		c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return c
}

// Probe tries every configured port and returns what answered.
func (p *Prober) Probe(ctx context.Context, host string) Result {
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		res Result
	)

	for _, port := range p.opts.Ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()

			select {
			case p.sem <- struct{}{}:
				defer func() { <-p.sem }()
			case <-ctx.Done():
				return
			}

			u, sans, ok := p.probePort(ctx, host, port)
			if !ok {
				return
			}

			mu.Lock()
			res.URLs = append(res.URLs, u)
			res.SANs = append(res.SANs, sans...)
			mu.Unlock()
		}(port)
	}
	wg.Wait()

	sort.Slice(res.URLs, func(i, j int) bool {
		if res.URLs[i].Port != res.URLs[j].Port {
			return res.URLs[i].Port < res.URLs[j].Port
		}
		return res.URLs[i].Scheme < res.URLs[j].Scheme
	})
	res.SANs = dedupe(res.SANs)
	return res
}

// probePort tries the scheme that port usually speaks first, then the other,
// and stops at the first answer. A port that has never been on the internet's
// standard list is tried as HTTPS first, because that is what it usually is.
func (p *Prober) probePort(ctx context.Context, host string, port int) (model.URL, []string, bool) {
	order := []string{"https", "http"}
	if port == 80 {
		order = []string{"http", "https"}
	}

	for _, scheme := range order {
		if ctx.Err() != nil {
			return model.URL{}, nil, false
		}
		if u, sans, ok := p.probeOne(ctx, scheme, host, port); ok {
			return u, sans, true
		}
	}
	return model.URL{}, nil, false
}

// probeOne sends one request.
func (p *Prober) probeOne(ctx context.Context, scheme, host string, port int) (model.URL, []string, bool) {
	address := scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"
	if (scheme == "https" && port == 443) || (scheme == "http" && port == 80) {
		// Leaving the default port out keeps the output usable as a URL list.
		address = scheme + "://" + host + "/"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return model.URL{}, nil, false
	}
	req.Header.Set("User-Agent", p.opts.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	client := p.noFollow
	if p.opts.Redirects {
		client = p.follow
	}

	resp, err := client.Do(req)
	if err != nil {
		return model.URL{}, nil, false
	}
	defer resp.Body.Close()

	u := model.URL{
		URL:        address,
		Scheme:     scheme,
		Port:       port,
		StatusCode: resp.StatusCode,
		Server:     resp.Header.Get("Server"),
	}
	// A negative length means the response was chunked; recording -1 would
	// claim a size nobody measured, so the field stays absent instead.
	if resp.ContentLength > 0 {
		u.ContentLen = resp.ContentLength
	}

	// Where the client ended up: with redirects off this is the Location the
	// response named, with them on it is the page that finally answered.
	if final := finalURL(resp); final != "" && final != address {
		u.Redirect = final
	}

	if p.opts.Title {
		u.Title = pageTitle(resp.Body)
	}

	if p.opts.Favicon {
		if hash, ok := p.faviconHash(ctx, u); ok {
			u.FaviconHash = hash
		}
	}

	var sans []string
	if p.opts.CertSAN && resp.TLS != nil {
		for _, cert := range resp.TLS.PeerCertificates {
			sans = append(sans, cert.DNSNames...)
			if cert.Subject.CommonName != "" {
				sans = append(sans, cert.Subject.CommonName)
			}
		}
	}

	return u, sans, true
}

// finalURL is the address the response came from, or the one it points at.
func finalURL(resp *http.Response) string {
	if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.String() != "" {
		return resp.Request.URL.String()
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		return loc
	}
	return ""
}

// titleRE finds the document title. It is deliberately tolerant: real pages
// carry attributes on the tag, newlines inside it, and no closing tag at all
// when the body was cut short.
var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)(</title>|$)`)

// pageTitle reads enough of the body to find the title, and no more.
func pageTitle(body io.Reader) string {
	limited := io.LimitReader(body, bodyLimit)
	data, err := io.ReadAll(limited)
	if err != nil && len(data) == 0 {
		return ""
	}

	m := titleRE.FindSubmatch(data)
	if m == nil {
		return ""
	}

	title := html.UnescapeString(string(m[1]))
	title = strings.Join(strings.Fields(title), " ")
	return truncate(title, 200)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Do not cut a multi-byte character in half.
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

// ParsePorts reads a port spec: "80,443", "8000-8100", "T:80,U:53" is not
// accepted — the protocol prefix belongs to a port scanner, not to an HTTP
// probe.
func ParsePorts(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}

	seen := make(map[int]bool)
	var out []int

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		lo, hi, isRange := strings.Cut(part, "-")
		if !isRange {
			port, err := strconv.Atoi(lo)
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("bad port %q", part)
			}
			if !seen[port] {
				seen[port] = true
				out = append(out, port)
			}
			continue
		}

		first, err1 := strconv.Atoi(strings.TrimSpace(lo))
		last, err2 := strconv.Atoi(strings.TrimSpace(hi))
		if err1 != nil || err2 != nil || first < 1 || last > 65535 || first > last {
			return nil, fmt.Errorf("bad port range %q", part)
		}
		for port := first; port <= last; port++ {
			if !seen[port] {
				seen[port] = true
				out = append(out, port)
			}
		}
	}
	return out, nil
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
