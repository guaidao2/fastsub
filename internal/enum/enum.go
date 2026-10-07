// Package enum invents names the passive sources never saw: a wordlist applied
// to the domain, and permutations of the labels already found.
//
// Everything it produces is a guess, which is why it only runs when asked for,
// and why the wildcard check has to have run first: in a zone that answers for
// everything, every guess resolves and none of them is evidence.
package enum

import (
	"sort"
	"strings"
)

// Prefixes are attached in front of an existing label: dev-api, staging-web.
var Prefixes = []string{
	"dev-", "test-", "staging-", "uat-", "qa-", "pre-", "beta-", "demo-",
	"old-", "new-", "internal-", "api-", "admin-", "app-", "web-", "mobile-",
	"vpn-", "git-", "ci-", "db-",
}

// Suffixes are attached behind one: api-dev, web-staging.
var Suffixes = []string{
	"-dev", "-test", "-staging", "-uat", "-qa", "-pre", "-beta", "-demo",
	"-old", "-new", "-internal", "-api", "-admin", "-app", "-web", "-backup",
	"-bak", "-v2",
}

// Numbers go on the end with no separator, which is how a second deployment
// usually turns up: api1, api2.
var Numbers = []string{"1", "2", "01", "02"}

// MaxMutateLabels bounds how many discovered labels are permuted. Each label
// becomes about forty candidates, so this is the difference between a scan and
// a flood aimed at whatever resolver you pointed it at.
const MaxMutateLabels = 60

// Word returns the candidates a wordlist produces for a domain.
//
// An entry that already is a name under the domain is kept as it is, so a list
// exported from another tool can be fed straight back in; anything else is
// treated as a label and gets the domain appended.
func Word(domain string, words []string) []string {
	out := make([]string, 0, len(words))
	for _, raw := range words {
		w := normalizeWord(raw)
		if w == "" {
			continue
		}
		if w == domain || strings.HasSuffix(w, "."+domain) {
			out = append(out, w)
			continue
		}
		out = append(out, w+"."+domain)
	}
	return dedupe(out)
}

// Mutate derives candidates from the labels already found under domain.
func Mutate(domain string, found []string) []string {
	labels := labelsOf(domain, found)
	if len(labels) > MaxMutateLabels {
		labels = labels[:MaxMutateLabels]
	}

	perLabel := len(Prefixes) + len(Suffixes) + len(Numbers)
	out := make([]string, 0, len(labels)*perLabel)

	for _, label := range labels {
		for _, p := range Prefixes {
			out = append(out, p+label+"."+domain)
		}
		for _, s := range Suffixes {
			out = append(out, label+s+"."+domain)
		}
		for _, n := range Numbers {
			out = append(out, label+n+"."+domain)
		}
	}
	return dedupe(out)
}

// labelsOf extracts the single-label names under domain: "api" from
// api.example.com. A name that is already several labels deep is skipped,
// because permuting every level multiplies the candidate set by its depth
// without adding anything a scan will reach.
func labelsOf(domain string, found []string) []string {
	seen := make(map[string]bool, len(found))
	out := make([]string, 0, len(found))

	for _, name := range found {
		name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
		label, ok := strings.CutSuffix(name, "."+domain)
		if !ok || label == "" || strings.Contains(label, ".") {
			continue
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}

	sort.Strings(out)
	return out
}

// normalizeWord cleans one wordlist line. Blank lines and comments are what a
// wordlist is mostly made of, and a stray space would turn into a name that
// never resolves.
func normalizeWord(w string) string {
	w = strings.TrimSpace(w)
	if w == "" || strings.HasPrefix(w, "#") {
		return ""
	}
	// A trailing comment is allowed, as in every other list fastsub reads.
	if i := strings.IndexByte(w, '#'); i >= 0 {
		w = strings.TrimSpace(w[:i])
	}
	w = strings.ToLower(w)
	w = strings.TrimSuffix(w, ".")
	w = strings.TrimPrefix(w, "*.")
	w = strings.TrimPrefix(w, ".")
	if strings.ContainsAny(w, " \t/\\@") {
		return ""
	}
	return w
}

// dedupe keeps the first occurrence of each candidate and the order they came
// in, so a run's candidate list is reproducible.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
