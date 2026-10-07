package i18n

// en is the reference catalogue. Every key fastsub can print exists here, and
// the test suite fails the build when another language is missing a key or when
// its printf verbs drift away from the English original.
var en = map[string]string{
	// Identity.
	"app.tagline": "subdomain enumeration and liveness probing for offensive security",
	"app.intro": "fastsub turns a domain into a list of live hosts. It asks passive sources, optionally\n" +
		"brute forces with your wordlist, resolves what it finds, drops wildcard answers, and\n" +
		"probes the survivors over HTTP and HTTPS. Results go to stdout as plain text or JSONL,\n" +
		"so the next link in the chain can read them: argus, crackweb, anything that takes a\n" +
		"host list.",

	// Sections.
	"group.target":   "TARGET",
	"group.sources":  "PASSIVE SOURCES",
	"group.enum":     "ACTIVE ENUMERATION",
	"group.resolve":  "RESOLUTION",
	"group.verify":   "LIVENESS",
	"group.output":   "OUTPUT",
	"group.misc":     "MISC",
	"group.examples": "EXAMPLES",

	// Target.
	"flag.domain":       "root domain to enumerate (repeatable)",
	"flag.domain-list":  "read root domains to enumerate from a file (- for stdin), one per line, '#' comments allowed",
	"flag.exclude":      "hosts to leave out; *.example.com covers its subdomains, .example.com also the domain",
	"flag.exclude-file": "read the hosts to leave out from a file",

	// Passive sources.
	"flag.sources":         "sources to query, comma-separated, or 'all' (default all)",
	"flag.exclude-sources": "sources to skip, comma-separated",
	"flag.provider-config": "API keys in subfinder's provider-config.yaml format (reserved: no built-in source needs a key yet)",
	"flag.list-sources":    "print the available sources and exit",
	"flag.only-passive":    "never resolve or probe; report what the sources said",

	// Active enumeration.
	"flag.wordlist":  "wordlist used for brute force (repeatable)",
	"flag.brute":     "brute force with the wordlists",
	"flag.mutate":    "derive new names from the ones already found",
	"flag.recursive": "recurse into discovered subdomains",
	"flag.depth":     "maximum recursion depth (default 1)",

	// Resolution.
	"flag.resolvers":          "read DNS resolvers from a file",
	"flag.doh":                "resolve through this DNS-over-HTTPS endpoint (repeatable)",
	"flag.no-wildcard-filter": "keep the answers a wildcard record produced",
	"flag.concurrency":        "parallel DNS queries (default 100)",
	"flag.resolve-timeout":    "per-query DNS timeout (default 3s)",

	// Liveness.
	"flag.ports":         "HTTP ports to probe (default 80,443)",
	"flag.probe":         "probe resolved hosts over HTTP and HTTPS",
	"flag.probe-timeout": "per-request probe timeout (default 10s)",
	"flag.no-title":      "do not read page titles while probing",
	"flag.no-favicon":    "do not fingerprint the favicon while probing",
	"flag.cert-san":      "feed certificate SANs back into enumeration",
	"flag.no-redirect":   "do not follow redirects while probing",

	// Output.
	"flag.output":        "write results to this file as well as stdout",
	"flag.format":        "output format: text, json, jsonl, csv, url (default text)",
	"flag.output-normal": "write nmap-style plain text to this file",
	"flag.output-json":   "write JSON to this file",
	"flag.output-all":    "write every format at once using this output base name",
	"flag.jsonl":         "emit one JSON object per line",
	"flag.baseline":      "only report what this earlier snapshot does not already contain",
	"flag.silent":        "print results only; hide progress and status",
	"flag.verbose":       "more detail on stderr",

	// Misc.
	"flag.lang":       "output language: en (default) or zh",
	"flag.list-langs": "list the available output languages and exit",
	"flag.no-color":   "disable colour (fastsub prints none today; NO_COLOR is honoured either way)",
	"flag.help":       "print this help and exit",
	"flag.version":    "print version information and exit",

	// Help scaffolding.
	"help.positional_args": "root domains to enumerate, such as example.com",
	"help.examples": "  fastsub -d example.com\n" +
		"  fastsub -d example.com --brute -w words.txt --probe\n" +
		"  fastsub -dL domains.txt -oA out/all                  # enumerate a list of domains\n" +
		"  fastsub -d example.com | argus -iL -                 # hand the host list to argus\n" +
		"  fastsub -d example.com -f url | httpx -silent        # hand URLs to httpx\n" +
		"  fastsub -d example.com --baseline yesterday.jsonl    # report only what is new",
	"help.usage":           "Usage: %s [options] <domain ...>",
	"help.run_help":        "Run 'fastsub %s --help' for more information.",
	"help.available_langs": "available languages: %s",
	"help.author":          "Authors: %s",
	"help.license":         "License: %s",
	"help.legal":           "Legal notice: use fastsub only against systems you own or have explicit written authorization to test.",

	// Errors.
	"error.unknown_flag":      "unknown option %q",
	"error.bad_bool":          "option %s needs true or false, got %q",
	"error.missing_value":     "option %s needs a value",
	"error.invalid_lang":      "unknown language %q; available: %s",
	"error.no_target":         "no target given; pass -d <domain>, a domain argument, or -iL <file>",
	"error.bad_format":        "unknown output format %q; use text, json, jsonl, csv or url",
	"error.bad_int":           "option %s needs a whole number, got %q",
	"error.bad_duration":      "option %s needs a duration such as 5s or 500ms, got %q",
	"error.bad_ports":         "cannot read port list %q",
	"error.usage_hint":        "run 'fastsub --help' for usage",
	"error.interrupted":       "interrupted",
	"error.not_implemented":   "%s is declared but not wired yet; this build stops here",
	"error.brute_needs_words": "--brute needs a wordlist; pass -w <file>",
	"error.unknown_ports":     "%s",

	// Status lines (stderr).
	"log.nothing_found":         "%s: no source reported anything",
	"log.sources_done":          "%s: %d name(s) reported by the sources",
	"log.source_done":           "  %s: %d name(s)",
	"log.source_failed":         "  %s failed: %v",
	"log.resolved":              "%d of %d name(s) resolve",
	"log.unresolved":            "  %s does not resolve",
	"log.lookup_failed":         "  %s: lookup failed: %v",
	"log.wildcard":              "%s answers for names that do not exist (%d address(es)); such answers are marked as wildcard",
	"log.wildcard_check_failed": "%s: wildcard check failed: %v",
	"log.wildcard_dropped":      "%d wildcard-only name(s) dropped",
	"log.brute_candidates":      "wordlist produced %d new name(s)",
	"log.mutate_candidates":     "permutations produced %d new name(s)",
	"log.probed":                "%d of %d host(s) answered over HTTP/HTTPS",
	"log.excluded":              "%d name(s) left out by --exclude",
	"log.baseline":              "%d name(s) were already in the baseline",
	"log.baseline_loaded":       "baseline holds %d name(s)",
	"log.recursing":             "recursing into %s (depth %d)",
	"log.recursion_capped":      "%d parent domain(s) found; walking the first %d",
	"log.cert_names":            "%d name(s) came from a certificate",
}
