package source

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// subdomaincenter answers with a plain JSON array of names collected from a
// crawl of the public web. It sees names that never had a certificate, which
// is exactly what certificate transparency cannot.
type subdomaincenter struct{}

func (subdomaincenter) Name() string   { return "subdomaincenter" }
func (subdomaincenter) NeedsKey() bool { return false }

func (subdomaincenter) Run(ctx context.Context, domain string, out chan<- string) error {
	endpoint := "https://api.subdomain.center/?domain=" + url.QueryEscape(domain)

	body, err := shared.get(ctx, endpoint, 2)
	if err != nil {
		return err
	}

	var names []string
	if err := decodeJSON("subdomaincenter", body, &names); err != nil {
		return err
	}
	if !emit(ctx, out, domain, names...) {
		return ctx.Err()
	}
	return nil
}

// otx is AlienVault's passive DNS dataset. Free access is rate limited hard
// enough that a run often gets a 429, which is reported rather than hidden.
type otx struct{}

func (otx) Name() string   { return "otx" }
func (otx) NeedsKey() bool { return false }

type otxResponse struct {
	PassiveDNS []struct {
		Hostname string `json:"hostname"`
	} `json:"passive_dns"`
}

func (otx) Run(ctx context.Context, domain string, out chan<- string) error {
	endpoint := "https://otx.alienvault.com/api/v1/indicators/domain/" +
		url.PathEscape(domain) + "/passive_dns"

	body, err := shared.get(ctx, endpoint, 2)
	if err != nil {
		return err
	}

	var resp otxResponse
	if err := decodeJSON("otx", body, &resp); err != nil {
		return err
	}

	names := make([]string, 0, len(resp.PassiveDNS))
	for _, e := range resp.PassiveDNS {
		names = append(names, e.Hostname)
	}
	if !emit(ctx, out, domain, names...) {
		return ctx.Err()
	}
	return nil
}

// urlscan searches a public archive of crawled pages. Its index answers with
// the page's own host, which finds application subdomains that a certificate
// would only mention if they had TLS.
type urlscan struct{}

func (urlscan) Name() string   { return "urlscan" }
func (urlscan) NeedsKey() bool { return false }

type urlscanResponse struct {
	Results []struct {
		Page struct {
			Domain string `json:"domain"`
		} `json:"page"`
		Task struct {
			URL string `json:"url"`
		} `json:"task"`
	} `json:"results"`
}

func (urlscan) Run(ctx context.Context, domain string, out chan<- string) error {
	endpoint := "https://urlscan.io/api/v1/search/?q=" +
		url.QueryEscape("domain:"+domain) + "&size=1000"

	body, err := shared.get(ctx, endpoint, 2)
	if err != nil {
		return err
	}

	var resp urlscanResponse
	if err := decodeJSON("urlscan", body, &resp); err != nil {
		return err
	}

	for _, r := range resp.Results {
		if !emit(ctx, out, domain, r.Page.Domain) {
			return ctx.Err()
		}
		// The submitted URL names a host too, and it is the one thing the
		// index is guaranteed to have.
		if host := hostOf(r.Task.URL); host != "" {
			if !emit(ctx, out, domain, host) {
				return ctx.Err()
			}
		}
	}
	return nil
}

// hostOf pulls the host out of a URL without dragging in the url package's
// error handling for a field that is only ever a hint.
func hostOf(raw string) string {
	_, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return ""
	}
	host, _, _ := strings.Cut(rest, "/")
	if h, _, found := strings.Cut(host, ":"); found {
		host = h
	}
	return host
}

// rapiddns publishes a plain HTML table of subdomains. There is no JSON and no
// documented shape to depend on, so the page is searched for anything that
// looks like a name under the domain being enumerated; everything else the
// page contains is discarded by the domain check every name goes through.
type rapiddns struct{}

func (rapiddns) Name() string   { return "rapiddns" }
func (rapiddns) NeedsKey() bool { return false }

// nameRE matches a DNS-looking name. It is deliberately loose: the domain
// filter is what makes the result correct, and this is only the cheap first
// pass that keeps the HTML out of it.
var nameRE = regexp.MustCompile(`(?i)[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?)+`)

func (rapiddns) Run(ctx context.Context, domain string, out chan<- string) error {
	endpoint := "https://rapiddns.io/subdomain/" + url.PathEscape(domain) + "?full=1"

	body, err := shared.get(ctx, endpoint, 2)
	if err != nil {
		return err
	}

	names := nameRE.FindAllString(string(body), -1)
	if !emit(ctx, out, domain, names...) {
		return ctx.Err()
	}
	return nil
}
