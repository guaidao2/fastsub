package cli

import "github.com/guaidao2/fastsub/internal/i18n"

// newFlagSet declares every option fastsub accepts. The declaration order is
// the order help prints, so the two never drift apart.
func newFlagSet(lang i18n.Lang) *FlagSet {
	fs := NewFlagSet(lang)

	// TARGET
	fs.Strings("domain", "d", "DOMAIN", "target")
	fs.String("domain-list", "dL", "FILE", "target")
	fs.String("exclude", "", "LIST", "target")
	fs.String("exclude-file", "", "FILE", "target")

	// PASSIVE SOURCES
	fs.String("sources", "s", "LIST", "sources")
	fs.String("exclude-sources", "", "LIST", "sources")
	fs.String("provider-config", "pc", "FILE", "sources")
	fs.Bool("list-sources", "", "sources")
	fs.Bool("only-passive", "", "sources")

	// ACTIVE ENUMERATION
	fs.Strings("wordlist", "w", "FILE", "enum")
	fs.Bool("brute", "", "enum")
	fs.Bool("mutate", "", "enum")
	fs.Bool("recursive", "r", "enum")
	fs.Int("depth", "", "N", "enum")

	// RESOLUTION
	fs.String("resolvers", "rl", "FILE", "resolve")
	fs.Strings("doh", "", "URL", "resolve")
	fs.Bool("no-wildcard-filter", "", "resolve")
	fs.Int("concurrency", "c", "N", "resolve")
	fs.Duration("resolve-timeout", "", "DUR", "resolve")

	// LIVENESS
	fs.String("ports", "p", "SPEC", "verify")
	fs.Bool("probe", "", "verify")
	fs.Duration("probe-timeout", "", "DUR", "verify")
	fs.Bool("no-title", "", "verify")
	fs.Bool("no-favicon", "", "verify")
	fs.Bool("cert-san", "", "verify")
	fs.Bool("no-redirect", "", "verify")

	// OUTPUT
	fs.String("output", "o", "FILE", "output")
	fs.String("format", "f", "FMT", "output")
	fs.String("output-normal", "oN", "FILE", "output")
	fs.String("output-json", "oJ", "FILE", "output")
	fs.String("output-all", "oA", "BASE", "output")
	fs.Bool("jsonl", "", "output")
	fs.String("baseline", "", "FILE", "output")
	fs.Bool("silent", "", "output")
	fs.Bool("verbose", "v", "output")

	// MISC
	fs.String("lang", "", "LANG", "misc")
	fs.Bool("list-langs", "", "misc")
	fs.Bool("no-color", "", "misc")
	fs.Bool("help", "h", "misc")
	fs.Bool("version", "V", "misc")

	return fs
}
