// Package scope decides which names a run may report. A scan is defined as much
// by what it leaves out as by what it includes: a shared host, a domain parked
// under the target by a third party, or a range somebody asked you not to
// touch.
package scope

import "strings"

// List is a set of exclusions.
type List struct {
	exact    map[string]bool
	suffixes []string // ".example.com": the subdomains, not the domain itself
	domains  []string // "example.com": the domain and everything under it
}

// Parse reads exclusion patterns.
//
// The spellings mean what they mean everywhere else. "example.com" is that one
// name. "*.example.com" is its subdomains, and not the domain itself — that is
// what a wildcard covers in DNS and in a certificate. ".example.com" is both,
// the way a leading dot reads in a zone file. Guessing which one somebody meant
// is how a scan reports a host they asked you to skip.
func Parse(patterns []string) *List {
	l := &List{exact: make(map[string]bool)}
	for _, raw := range patterns {
		p := normalize(raw)
		if p == "" || p == "*" || p == "." {
			continue
		}

		switch {
		case strings.HasPrefix(p, "*."):
			l.addSuffix(strings.TrimPrefix(p, "*."))
		case strings.HasPrefix(p, "."):
			l.addDomain(strings.TrimPrefix(p, "."))
		default:
			l.exact[p] = true
		}
	}
	return l
}

func (l *List) addSuffix(domain string) {
	domain = normalize(domain)
	if domain == "" {
		return
	}
	l.suffixes = append(l.suffixes, "."+domain)
}

func (l *List) addDomain(domain string) {
	domain = normalize(domain)
	if domain == "" {
		return
	}
	l.domains = append(l.domains, domain)
}

// Excludes reports whether a host is out of scope.
func (l *List) Excludes(host string) bool {
	if l == nil || l.Empty() {
		return false
	}

	host = normalize(host)
	if host == "" {
		return false
	}
	if l.exact[host] {
		return true
	}

	for _, suffix := range l.suffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	for _, domain := range l.domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// Empty reports whether anything is excluded at all.
func (l *List) Empty() bool {
	return l == nil || (len(l.exact) == 0 && len(l.suffixes) == 0 && len(l.domains) == 0)
}

// Len counts the patterns, for a status line that says what was applied.
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.exact) + len(l.suffixes) + len(l.domains)
}

// Split breaks a comma-separated --exclude value into patterns.
func Split(spec string) []string {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalize(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	return strings.TrimSuffix(p, ".")
}
