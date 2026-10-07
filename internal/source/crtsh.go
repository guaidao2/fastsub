package source

import (
	"context"
	"net/url"
)

// crtsh reads certificate transparency logs through crt.sh. It is the most
// productive free source and also the flakiest: the endpoint answers 502 when
// it is busy, which is why it gets more attempts than the others.
type crtsh struct{}

func (crtsh) Name() string   { return "crtsh" }
func (crtsh) NeedsKey() bool { return false }

func (crtsh) Run(ctx context.Context, domain string, out chan<- string) error {
	// The leading % is a SQL wildcard: every name that ends in the domain.
	endpoint := "https://crt.sh/?q=%25." + url.QueryEscape(domain) + "&output=json"

	body, err := shared.get(ctx, endpoint, 3)
	if err != nil {
		return err
	}

	var entries []struct {
		CommonName string `json:"common_name"`
		NameValue  string `json:"name_value"`
	}
	if err := decodeJSON("crtsh", body, &entries); err != nil {
		return err
	}

	for _, e := range entries {
		// name_value carries every name on the certificate, one per line.
		if !emit(ctx, out, domain, splitLines(e.NameValue)...) {
			return ctx.Err()
		}
		if !emit(ctx, out, domain, e.CommonName) {
			return ctx.Err()
		}
	}
	return nil
}

// certspotter is a certificate transparency search with a documented free
// quota. It answers the same question as crt.sh and stays up while crt.sh does
// not, so the two together cover the CT logs far better than either alone.
type certspotter struct{}

func (certspotter) Name() string   { return "certspotter" }
func (certspotter) NeedsKey() bool { return false }

type certspotterIssuance struct {
	DNSNames []string `json:"dns_names"`
}

func (certspotter) Run(ctx context.Context, domain string, out chan<- string) error {
	endpoint := "https://api.certspotter.com/v1/issuances?domain=" + url.QueryEscape(domain) +
		"&include_subdomains=true&expand=dns_names"

	body, err := shared.get(ctx, endpoint, 2)
	if err != nil {
		return err
	}

	var issuances []certspotterIssuance
	if err := decodeJSON("certspotter", body, &issuances); err != nil {
		return err
	}

	for _, iss := range issuances {
		if !emit(ctx, out, domain, iss.DNSNames...) {
			return ctx.Err()
		}
	}
	return nil
}
