// Package source queries passive sources — certificate transparency logs, DNS
// datasets, crawlers — for names under a domain. A source never touches the
// target itself, which is what makes passive enumeration quiet.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/guaidao2/fastsub/internal/version"
)

// Source is one place names can be found. Run reports names on out and returns
// when it is done; it must not close the channel, because the caller owns it.
type Source interface {
	// Name is the identifier used by --sources and recorded in results.
	Name() string
	// NeedsKey reports whether the source cannot work without credentials.
	// Such a source is skipped, with a reason, when no key is configured.
	NeedsKey() bool
	// Run queries for names under domain.
	Run(ctx context.Context, domain string, out chan<- string) error
}

// catalogue is every source this build knows about, in listing order.
var catalogue = []Source{
	crtsh{},
	certspotter{},
	subdomaincenter{},
	urlscan{},
	rapiddns{},
	hackertarget{},
	otx{},
}

// All returns every known source.
func All() []Source {
	out := make([]Source, len(catalogue))
	copy(out, catalogue)
	return out
}

// Names lists the known source names, sorted.
func Names() []string {
	out := make([]string, 0, len(catalogue))
	for _, s := range catalogue {
		out = append(out, s.Name())
	}
	sort.Strings(out)
	return out
}

// Select resolves a comma-separated --sources value. "all" or an empty value
// means every source.
func Select(spec string) ([]Source, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "all") {
		return All(), nil
	}

	byName := make(map[string]Source, len(catalogue))
	for _, s := range catalogue {
		byName[s.Name()] = s
	}

	var out []Source
	for _, part := range strings.Split(spec, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		s, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown source %q; available: %s", name, strings.Join(Names(), ", "))
		}
		out = append(out, s)
	}
	return out, nil
}

// Exclude removes sources by name from a list.
func Exclude(sources []Source, spec string) []Source {
	if strings.TrimSpace(spec) == "" {
		return sources
	}
	drop := make(map[string]bool)
	for _, part := range strings.Split(spec, ",") {
		drop[strings.ToLower(strings.TrimSpace(part))] = true
	}
	out := sources[:0:0]
	for _, s := range sources {
		if !drop[s.Name()] {
			out = append(out, s)
		}
	}
	return out
}

// client is the HTTP client every source shares. It retries, because the
// sources that matter most — crt.sh above all — answer with 502 under load and
// succeed on the next attempt.
type client struct {
	hc *http.Client
}

var shared = &client{
	hc: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	},
}

// userAgent identifies fastsub honestly, the way argus and crackweb do.
func userAgent() string {
	return fmt.Sprintf("%s/%s (+%s)", version.Name, version.Version, version.Repo)
}

// get fetches a URL, retrying transient failures with a growing pause. A 4xx
// is returned as-is: asking again will not fix it, and a source that is rate
// limiting us should be allowed to say so.
//
// attempts is per source rather than shared, because the cost of a retry
// differs: crt.sh answers 502 constantly and is worth three tries, while an
// endpoint that is merely slow is not worth holding the whole run open for.
func (c *client) get(ctx context.Context, url string, attempts int) ([]byte, error) {
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		body, retryable, err := c.once(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s: %w", url, lastErr)
}

// once performs one attempt and reports whether trying again could help.
func (c *client) once(ctx context.Context, url string) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := c.hc.Do(req)
	if err != nil {
		// A cancelled context is the caller's decision, not a transient fault.
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, true, err
	}
	defer resp.Body.Close()

	// 16 MB is far past any legitimate answer from these endpoints and stops a
	// misbehaving one from filling memory.
	body, err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, true, err
	}

	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	return body, false, nil
}

// decodeJSON unmarshals a source response, naming the source in the error so a
// shape change upstream is obvious.
func decodeJSON(name string, body []byte, v any) error {
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%s: cannot read the response: %w", name, err)
	}
	return nil
}

// clean turns what a source said into the form fastsub reports, or returns ""
// when the name is unusable: outside the domain being enumerated, a wildcard
// rather than a name, or the domain itself.
func clean(name, domain string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimSuffix(name, ".")
	name = strings.TrimPrefix(name, "*.")
	name = strings.TrimPrefix(name, ".")
	if name == "" || name == domain {
		return ""
	}
	if !strings.HasSuffix(name, "."+domain) {
		return ""
	}
	// A name with a space or a slash came from a field that was not a name.
	if strings.ContainsAny(name, " \t/\\@") {
		return ""
	}
	return name
}

// emit sends every usable name on the channel, stopping early when the context
// is cancelled. It returns false when the caller should stop.
func emit(ctx context.Context, out chan<- string, domain string, names ...string) bool {
	for _, raw := range names {
		n := clean(raw, domain)
		if n == "" {
			continue
		}
		select {
		case out <- n:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// splitLines breaks a field that may hold several names, which is what crt.sh
// does with name_value.
func splitLines(s string) []string {
	if !strings.ContainsAny(s, "\r\n") {
		return []string{s}
	}
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
}
