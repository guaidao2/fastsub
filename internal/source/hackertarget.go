package source

import (
	"context"
	"net/url"
	"strings"
)

// hackertarget answers with "host,ip" lines. The free endpoint is limited by
// the day and says so in the body rather than in the status code, so a
// complaint is checked for and reported instead of being parsed as data.
type hackertarget struct{}

func (hackertarget) Name() string   { return "hackertarget" }
func (hackertarget) NeedsKey() bool { return false }

func (hackertarget) Run(ctx context.Context, domain string, out chan<- string) error {
	endpoint := "https://api.hackertarget.com/hostsearch/?q=" + url.QueryEscape(domain)

	body, err := shared.get(ctx, endpoint, 2)
	if err != nil {
		return err
	}

	text := string(body)
	if msg := complaint(text); msg != "" {
		return &SourceError{Source: "hackertarget", Message: msg}
	}

	var names []string
	for _, line := range splitLines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		host, _, _ := strings.Cut(line, ",")
		names = append(names, host)
	}
	if !emit(ctx, out, domain, names...) {
		return ctx.Err()
	}
	return nil
}

// complaint returns the endpoint's own words when a plain-text answer is not
// data. A real answer is "host,ip" lines; anything else is the service
// explaining itself, and saying so beats reporting zero results as success.
func complaint(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "empty response"
	}
	first, _, _ := strings.Cut(trimmed, "\n")
	first = strings.TrimSpace(first)
	if first == "" {
		return "empty response"
	}
	if strings.Contains(first, ",") {
		return ""
	}
	return first
}

// SourceError is a source declining to answer for a reason worth showing the
// user verbatim, rather than a shape the parser could not read.
type SourceError struct {
	Source  string
	Message string
}

func (e *SourceError) Error() string { return e.Source + ": " + e.Message }
