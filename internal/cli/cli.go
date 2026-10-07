// Package cli parses the command line and drives the pipeline.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/guaidao2/fastsub/internal/baseline"
	"github.com/guaidao2/fastsub/internal/i18n"
	"github.com/guaidao2/fastsub/internal/output"
	"github.com/guaidao2/fastsub/internal/pipeline"
	"github.com/guaidao2/fastsub/internal/resolve"
	"github.com/guaidao2/fastsub/internal/scope"
	"github.com/guaidao2/fastsub/internal/source"
	"github.com/guaidao2/fastsub/internal/verify"
	"github.com/guaidao2/fastsub/internal/version"
)

// Exit codes, deliberately the same as argus so a wrapper treats both tools
// alike: 0 success, 1 runtime error, 2 usage error, 130 interrupted.
const (
	ExitOK        = 0
	ExitError     = 1
	ExitUsage     = 2
	ExitInterrupt = 130
)

// Run is the whole program. It takes its streams as arguments and returns an
// exit code rather than calling os.Exit, so every path stays testable.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// The language has to be known before anything can be printed, including a
	// parse error, so it is read once ahead of the real parse.
	lang := i18n.Resolve(prescanLang(args))

	fs := newFlagSet(lang)
	if err := fs.Parse(args); err != nil {
		return usageError(stderr, lang, err)
	}

	// A language that was asked for but does not exist is an error, not a
	// silent fallback: the user typed something and deserves to hear about it.
	if v := fs.StringValue("lang"); v != "" {
		l, ok := i18n.Parse(v)
		if !ok {
			return usageError(stderr, lang, fmt.Errorf("%s", i18n.T(lang, "error.invalid_lang", v, i18n.Listing())))
		}
		lang = l
		fs.Lang = l
	}

	switch {
	case fs.BoolValue("help"):
		renderHelp(stdout, fs, lang)
		return ExitOK
	case fs.BoolValue("version"):
		fmt.Fprintln(stdout, version.Long())
		return ExitOK
	case fs.BoolValue("list-langs"):
		fmt.Fprintln(stdout, i18n.Listing())
		return ExitOK
	}

	return run(fs, lang, stdin, stdout, stderr)
}

// run is everything after the command line has been understood: it builds the
// stages, wires the outputs and runs the pipeline.
func run(fs *FlagSet, lang i18n.Lang, stdin io.Reader, stdout, stderr io.Writer) int {
	targets, err := collectTargets(fs, stdin)
	if err != nil {
		return usageError(stderr, lang, err)
	}

	sources, err := source.Select(fs.StringValue("sources"))
	if err != nil {
		return usageError(stderr, lang, err)
	}
	sources = source.Exclude(sources, fs.StringValue("exclude-sources"))

	if fs.BoolValue("list-sources") {
		for _, name := range source.Names() {
			fmt.Fprintln(stdout, name)
		}
		return ExitOK
	}

	if len(targets) == 0 {
		return usageError(stderr, lang, errors.New(i18n.T(lang, "error.no_target")))
	}

	if missing := unwired(fs, lang); len(missing) > 0 {
		fmt.Fprintf(stderr, "%s: %s\n", version.Name,
			i18n.T(lang, "error.not_implemented", strings.Join(missing, ", ")))
		return ExitError
	}

	format := output.Text
	if spec := fs.StringValue("format"); spec != "" {
		f, ok := output.ParseFormat(spec)
		if !ok {
			return usageError(stderr, lang, fmt.Errorf("%s", i18n.T(lang, "error.bad_format", spec)))
		}
		format = f
	}

	sink, err := buildSink(fs, format, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", version.Name, err)
		return ExitError
	}
	defer sink.cleanup()

	words, err := collectWords(fs, stdin)
	if err != nil {
		return usageError(stderr, lang, err)
	}
	if fs.BoolValue("brute") && len(words) == 0 {
		return usageError(stderr, lang, errors.New(i18n.T(lang, "error.brute_needs_words")))
	}

	prober, err := buildProber(fs, format)
	if err != nil {
		return usageError(stderr, lang, err)
	}

	exclude, err := buildExclude(fs, stdin)
	if err != nil {
		return usageError(stderr, lang, err)
	}

	// A baseline is what makes a scheduled run useful: the report is the names
	// that were not there yesterday, not the twelve hundred that were.
	var known baseline.Set
	if path := fs.StringValue("baseline"); path != "" {
		known, err = baseline.Load(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", version.Name, err)
			return ExitError
		}
		if !fs.BoolValue("silent") {
			fmt.Fprintln(stderr, i18n.T(lang, "log.baseline_loaded", known.Len()))
		}
	}

	resolver, err := buildResolver(fs)
	if err != nil {
		return usageError(stderr, lang, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	p := pipeline.New(pipeline.Options{
		Lang:           lang,
		Domains:        targets,
		Sources:        sources,
		Wordlist:       words,
		Brute:          fs.BoolValue("brute"),
		Mutate:         fs.BoolValue("mutate"),
		Recursive:      fs.BoolValue("recursive"),
		Depth:          fs.IntValue("depth"),
		Resolver:       resolver,
		Prober:         prober,
		CertSAN:        fs.BoolValue("cert-san"),
		Exclude:        exclude,
		Baseline:       known,
		Concurrency:    fs.IntValue("concurrency"),
		OnlyPassive:    fs.BoolValue("only-passive"),
		WildcardFilter: !fs.BoolValue("no-wildcard-filter"),
		Silent:         fs.BoolValue("silent"),
		Verbose:        fs.BoolValue("verbose"),
		Log:            stderr,
	})

	if _, err := p.Run(ctx, sink); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintf(stderr, "%s: %s\n", version.Name, i18n.T(lang, "error.interrupted"))
			return ExitInterrupt
		}
		fmt.Fprintf(stderr, "%s: %v\n", version.Name, err)
		return ExitError
	}
	return ExitOK
}

// unwired names the options that are declared but have no stage behind them
// yet. A flag that silently does nothing is worse than a flag that is missing:
// the user would trust a result it never influenced.
func unwired(fs *FlagSet, _ i18n.Lang) []string {
	var missing []string
	// No built-in source needs a key, so there is nothing for a key file to
	// feed yet. Saying so beats reading it and quietly ignoring it.
	if fs.StringValue("provider-config") != "" {
		missing = append(missing, "--provider-config")
	}
	return missing
}

// collectWords reads every -w file. A wordlist is just another one-entry-per-
// line file, so it goes through the same reader as -iL.
func collectWords(fs *FlagSet, stdin io.Reader) ([]string, error) {
	var out []string
	for _, path := range fs.StringsValue("wordlist") {
		lines, err := readList(path, stdin)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return out, nil
}

// buildExclude assembles the exclusion list from --exclude and --exclude-file.
// Both name hosts or patterns, so both go through the same parser.
func buildExclude(fs *FlagSet, stdin io.Reader) (*scope.List, error) {
	patterns := scope.Split(fs.StringValue("exclude"))
	if path := fs.StringValue("exclude-file"); path != "" {
		lines, err := readList(path, stdin)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, lines...)
	}
	return scope.Parse(patterns), nil
}

// buildProber returns a prober when the run asks for liveness: --probe, the url
// format, which has nothing to print without one, or --cert-san, which has no
// certificate to read without one.
func buildProber(fs *FlagSet, format output.Format) (*verify.Prober, error) {
	if !fs.BoolValue("probe") && format != output.URL && !fs.BoolValue("cert-san") {
		return nil, nil
	}

	ports, err := verify.ParsePorts(fs.StringValue("ports"))
	if err != nil {
		return nil, err
	}

	return verify.New(verify.Options{
		Ports:       ports,
		Timeout:     fs.DurationValue("probe-timeout"),
		Concurrency: fs.IntValue("concurrency"),
		Title:       !fs.BoolValue("no-title"),
		Favicon:     !fs.BoolValue("no-favicon"),
		Redirects:   !fs.BoolValue("no-redirect"),
		CertSAN:     fs.BoolValue("cert-san"),
	}), nil
}

// buildResolver assembles the resolver pool. --doh endpoints and an -rl file
// both add to it; when neither was given, the built-in resolvers are used.
func buildResolver(fs *FlagSet) (*resolve.Resolver, error) {
	opts := resolve.Options{
		Timeout:     fs.DurationValue("resolve-timeout"),
		Concurrency: fs.IntValue("concurrency"),
	}

	// A named DoH endpoint replaces the default list rather than joining it:
	// it was asked for by name, and answering through the resolvers it was
	// meant to avoid would be the wrong kind of help.
	opts.Servers = append(opts.Servers, fs.StringsValue("doh")...)

	if path := fs.StringValue("resolvers"); path != "" {
		servers, err := readResolverFile(path)
		if err != nil {
			return nil, err
		}
		opts.Servers = append(opts.Servers, servers...)
	}
	return resolve.New(opts), nil
}

// readResolverFile reads one resolver per line. A bare address gets the
// standard port, because nobody types :53 twice.
func readResolverFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, ":") {
			line += ":53"
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

// collectTargets gathers the roots to enumerate: -d, positional arguments and
// -dL, in that order, with duplicates dropped.
func collectTargets(fs *FlagSet, stdin io.Reader) ([]string, error) {
	var out []string
	out = append(out, fs.StringsValue("domain")...)
	out = append(out, fs.Positional()...)

	if list := fs.StringValue("domain-list"); list != "" {
		lines, err := readList(list, stdin)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return dedupe(out), nil
}

// dedupe normalises and orders a target list, dropping blank entries and
// repeats. Host names are case insensitive and may carry a trailing dot.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		t = strings.TrimSuffix(t, ".")
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// readList reads one target per line. "-" means stdin, blank lines and
// '#' comments are skipped, and a trailing comment is dropped — the same rules
// argus applies to -iL, so one file serves both tools.
func readList(path string, stdin io.Reader) ([]string, error) {
	var r io.Reader
	if path == "-" {
		r = stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}

	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
			if line == "" {
				continue
			}
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// prescanLang reads --lang without consuming it, so parse errors can be
// reported in the language the user asked for instead of always in English.
func prescanLang(args []string) string {
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--lang="); ok {
			return v
		}
		if a == "--lang" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func usageError(stderr io.Writer, lang i18n.Lang, err error) int {
	fmt.Fprintf(stderr, "%s: %v\n", version.Name, err)
	fmt.Fprintln(stderr, i18n.T(lang, "error.usage_hint"))
	return ExitUsage
}
