// Package pipeline is the run itself: ask the sources, resolve what they
// answered, drop the names that exist only because the zone answers for
// everything, and hand the survivors to the output formats.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/guaidao2/fastsub/internal/baseline"
	"github.com/guaidao2/fastsub/internal/enum"
	"github.com/guaidao2/fastsub/internal/i18n"
	"github.com/guaidao2/fastsub/internal/model"
	"github.com/guaidao2/fastsub/internal/resolve"
	"github.com/guaidao2/fastsub/internal/scope"
	"github.com/guaidao2/fastsub/internal/source"
	"github.com/guaidao2/fastsub/internal/verify"
)

// Sink receives what a run confirms. *output.Printer is one; so is a fan-out
// that writes several formats at once.
type Sink interface {
	Host(model.Host) error
	Close(model.Summary) error
}

// Resolver is the DNS stage the pipeline needs. *resolve.Resolver implements
// it; stating the requirement as an interface is what lets the chain be tested
// without a network.
type Resolver interface {
	Lookup(ctx context.Context, name string) (resolve.Result, error)
	DetectWildcard(ctx context.Context, domain string) (*resolve.Wildcard, error)
}

// Options is everything a run needs.
type Options struct {
	Lang i18n.Lang
	// Domains are the roots to enumerate: sources are asked about them, and
	// every stage applies.
	Domains []string
	// Sources are the passive sources to query.
	Sources []source.Source
	// Wordlist holds every entry from every -w file, and Brute turns them into
	// candidates. Brute without a wordlist asks for nothing.
	Wordlist []string
	// Brute applies the wordlist to each domain.
	Brute bool
	// Mutate permutes the labels already found.
	Mutate bool
	// Recursive walks the subdomains a run reveals, because a name like
	// dev.example.com is a namespace of its own: nothing under it appears in
	// example.com's certificate or in a wordlist applied to example.com. Only
	// names that actually resolve are walked, so this follows what exists
	// rather than everything a source has ever seen.
	Recursive bool
	// Depth is how many levels of that walk to do. Zero means the top domain
	// only.
	Depth int
	// Resolver resolves what the sources reported.
	Resolver Resolver
	// Prober probes what resolved. Nil means names are reported on their DNS
	// answer alone.
	Prober *verify.Prober
	// CertSAN feeds the names found in the certificates back into the run. A
	// certificate is evidence that a name was issued for, including names no
	// passive source ever saw.
	CertSAN bool
	// Exclude lists names the run may not report. Nil excludes nothing.
	Exclude *scope.List
	// Baseline is what an earlier run already knew; those names are not
	// reported again. Nil means report everything.
	Baseline baseline.Set
	// Concurrency bounds parallel DNS lookups.
	Concurrency int
	// OnlyPassive reports what the sources said without touching the target at
	// all: no resolution, no probing.
	OnlyPassive bool
	// WildcardFilter drops names that resolve only because of a wildcard record.
	WildcardFilter bool
	// SourceTimeout bounds one source. A slow endpoint must not hold the whole
	// run open; it is reported and left behind.
	SourceTimeout time.Duration
	// Silent hides progress; results are unaffected.
	Silent bool
	// Verbose adds detail to the progress stream.
	Verbose bool
	// Log receives progress. It is stderr in practice, never stdout.
	Log io.Writer
}

// Pipeline runs one enumeration.
type Pipeline struct {
	opts Options
}

// New returns a pipeline with the defaults filled in.
func New(opts Options) *Pipeline {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 100
	}
	if opts.SourceTimeout <= 0 {
		opts.SourceTimeout = 90 * time.Second
	}
	if opts.Depth <= 0 {
		opts.Depth = 1
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	return &Pipeline{opts: opts}
}

// Run enumerates every domain and writes each host as it is confirmed.
func (p *Pipeline) Run(ctx context.Context, out Sink) (model.Summary, error) {
	start := time.Now()

	var all []model.Host
	walked := make(map[string]bool)
	for _, domain := range p.opts.Domains {
		hosts, err := p.walk(ctx, domain, 0, walked, out)
		if err != nil {
			return model.Summary{}, err
		}
		all = append(all, hosts...)
	}

	summary := model.NewSummary(all, time.Since(start).Seconds())
	if err := out.Close(summary); err != nil {
		return summary, err
	}
	return summary, nil
}

// walk enumerates a domain, and then the subdomains that enumeration revealed,
// up to the configured depth. walked is shared across the whole run so a name
// is never enumerated twice, however many parents lead to it.
func (p *Pipeline) walk(ctx context.Context, domain string, depth int, walked map[string]bool, out Sink) ([]model.Host, error) {
	if walked[domain] {
		return nil, nil
	}
	walked[domain] = true

	if depth > 0 {
		p.logf("log.recursing", domain, depth)
	}

	hosts, err := p.enumerate(ctx, domain, out)
	if err != nil {
		return nil, err
	}

	if !p.opts.Recursive || depth >= p.opts.Depth {
		return hosts, nil
	}

	var all []model.Host
	for _, child := range p.children(domain, hosts, walked) {
		found, err := p.walk(ctx, child, depth+1, walked, out)
		if err != nil {
			return nil, err
		}
		all = append(all, found...)
	}
	return append(hosts, all...), nil
}

// MaxChildren bounds how many parent domains one level of recursion walks. A
// domain with hundreds of deep names would otherwise turn one enumeration into
// hundreds of them, which is a flood aimed at the sources rather than a scan
// of the target.
const MaxChildren = 25

// children lists the names one label deeper than domain that turned up under
// it: from api.dev.example.com, the one worth walking next is dev.example.com.
// Sorted, so a run's order does not depend on map iteration.
func (p *Pipeline) children(domain string, hosts []model.Host, walked map[string]bool) []string {
	seen := make(map[string]bool)
	for _, h := range hosts {
		parent := parentOf(h.Host, domain)
		if parent == "" || parent == domain || walked[parent] {
			continue
		}
		seen[parent] = true
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)

	if len(out) > MaxChildren {
		p.logf("log.recursion_capped", len(out), MaxChildren)
		out = out[:MaxChildren]
	}
	return out
}

// parentOf strips the leftmost label, as long as what is left is still inside
// root. It returns "" when the only thing above the name is root itself, which
// is what stops the walk from going in circles.
func parentOf(name, root string) string {
	if !strings.HasSuffix(name, "."+root) {
		return ""
	}
	trimmed := strings.TrimSuffix(name, "."+root)
	i := strings.IndexByte(trimmed, '.')
	if i < 0 {
		return ""
	}
	return trimmed[i+1:] + "." + root
}

// enumerate runs one root domain through the whole chain.
func (p *Pipeline) enumerate(ctx context.Context, domain string, out Sink) ([]model.Host, error) {
	reported := p.querySources(ctx, domain)
	if len(reported) == 0 {
		p.logf("log.nothing_found", domain)
	}

	// Guesses join the reported set after the sources, marked with where they
	// came from: "a wordlist invented this" is worth knowing when reading a
	// result, and it is the difference between a finding and a candidate.
	p.addCandidates(domain, reported)

	// Out-of-scope and already-known names go before anything is resolved:
	// there is no reason to spend a DNS query on a host that will not be
	// reported either way.
	p.filter(reported)

	if len(reported) == 0 {
		return nil, nil
	}
	p.logf("log.sources_done", domain, len(reported))

	// Only-passive answers the question "what have people published about this
	// domain", and the answer is the raw list. Nothing is removed for being
	// unresolvable, because nothing was resolved.
	if p.opts.OnlyPassive {
		return p.emitPassive(reported, out)
	}

	// The wildcard has to be known before any name is judged: with one in
	// place every name a wordlist invents resolves too.
	if p.opts.Resolver == nil {
		return nil, errors.New("no resolver configured")
	}
	wildcard, wcErr := p.opts.Resolver.DetectWildcard(ctx, domain)
	// Only a check that actually completed can be recorded as a verdict. A
	// check that failed leaves every name's wildcard field unset, which is what
	// "nobody looked" means.
	wildcardChecked := wcErr == nil
	if wcErr != nil {
		p.logf("log.wildcard_check_failed", domain, wcErr)
	}
	if wildcard != nil {
		p.logf("log.wildcard", domain, len(wildcard.IPs))
	}

	hosts := p.resolveAll(ctx, domain, reported, wildcard, wildcardChecked)

	if p.opts.Prober != nil {
		sans := p.probeAll(ctx, hosts)
		if p.opts.CertSAN && len(sans) > 0 {
			hosts = append(hosts, p.adoptSANs(ctx, domain, sans, reported, wildcard, wildcardChecked)...)
		}
	}

	kept := hosts[:0]
	dropped := 0
	for _, h := range hosts {
		// A wildcard-sourced name is not evidence that the name exists, and a
		// probe does not change that: the wildcard answers the probe too, which
		// is exactly how a zone would produce hundreds of live-looking hosts.
		if model.Measured(h.Wildcard) && p.opts.WildcardFilter {
			dropped++
			continue
		}
		kept = append(kept, h)
	}
	if dropped > 0 {
		p.logf("log.wildcard_dropped", dropped)
	}

	for _, h := range kept {
		if err := out.Host(h); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// addCandidates folds the guesses into the reported set: the wordlist first,
// then permutations of everything known by that point, which lets the wordlist
// feed the permutation engine in the same run.
func (p *Pipeline) addCandidates(domain string, reported map[string]map[string]bool) {
	if p.opts.Brute && len(p.opts.Wordlist) > 0 {
		fresh := 0
		for _, name := range enum.Word(domain, p.opts.Wordlist) {
			if reported[name] == nil {
				reported[name] = make(map[string]bool)
				fresh++
			}
			reported[name]["brute"] = true
		}
		p.logf("log.brute_candidates", fresh)
	}

	if p.opts.Mutate && len(reported) > 0 {
		found := make([]string, 0, len(reported))
		for name := range reported {
			found = append(found, name)
		}

		fresh := 0
		for _, name := range enum.Mutate(domain, found) {
			if reported[name] == nil {
				reported[name] = make(map[string]bool)
				fresh++
			}
			reported[name]["mutate"] = true
		}
		p.logf("log.mutate_candidates", fresh)
	}
}

// filter drops what the run is not allowed or not asked to report.
func (p *Pipeline) filter(reported map[string]map[string]bool) {
	if p.opts.Exclude.Empty() && p.opts.Baseline.Len() == 0 {
		return
	}

	excluded, known := 0, 0
	for name := range reported {
		if p.opts.Exclude.Excludes(name) {
			delete(reported, name)
			excluded++
			continue
		}
		if p.opts.Baseline.Has(name) {
			delete(reported, name)
			known++
		}
	}

	if excluded > 0 {
		p.logf("log.excluded", excluded)
	}
	if known > 0 {
		p.logf("log.baseline", known)
	}
}

// probeAll asks every resolved host whether something answers over HTTP, and
// returns the names the certificates it saw were issued for.
func (p *Pipeline) probeAll(ctx context.Context, hosts []model.Host) []string {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		sans []string
	)

	for i := range hosts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := p.opts.Prober.Probe(ctx, hosts[i].Host)
			hosts[i].URLs = res.URLs
			hosts[i].Alive = model.Bool(res.Alive())

			if len(res.SANs) > 0 {
				mu.Lock()
				sans = append(sans, res.SANs...)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	alive := 0
	for _, h := range hosts {
		if model.Measured(h.Alive) {
			alive++
		}
	}
	p.logf("log.probed", alive, len(hosts))
	return sans
}

// adoptSANs resolves the names a certificate carried. It is one extra pass, not
// a loop: a new name can bring a new certificate, and following that to its end
// is how an enumeration walks off the edge into someone else's domain.
func (p *Pipeline) adoptSANs(ctx context.Context, domain string, sans []string, reported map[string]map[string]bool, wildcard *resolve.Wildcard, wildcardChecked bool) []model.Host {
	fresh := make(map[string]map[string]bool)

	for _, raw := range sans {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		// A wildcard on a certificate names a pattern, not a host. It is the
		// wildcard check's business, and reporting "*" as a name is nonsense.
		if name == "" || strings.HasPrefix(name, "*") {
			continue
		}
		if name == domain || !strings.HasSuffix(name, "."+domain) {
			continue
		}
		if reported[name] != nil || fresh[name] != nil {
			continue
		}
		if p.opts.Exclude.Excludes(name) || p.opts.Baseline.Has(name) {
			continue
		}
		// The name was on a certificate that a host in scope presented, which
		// is where it came from.
		fresh[name] = map[string]bool{"certificate": true}
	}

	if len(fresh) == 0 {
		return nil
	}
	p.logf("log.cert_names", len(fresh))

	for name, sources := range fresh {
		reported[name] = sources
	}
	return p.resolveAll(ctx, domain, fresh, wildcard, wildcardChecked)
}

// emitPassive writes the raw list, sorted, with the sources that named each
// host.
func (p *Pipeline) emitPassive(reported map[string]map[string]bool, out Sink) ([]model.Host, error) {
	names := sortedKeys(reported)
	hosts := make([]model.Host, 0, len(names))
	for _, name := range names {
		h := model.Host{Host: name, Sources: sortedKeys(reported[name])}
		if err := out.Host(h); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// querySources runs every source at once and collects what they report, along
// with which source said it. A source that fails is reported and the run
// continues: one dead endpoint is not a reason to return nothing.
func (p *Pipeline) querySources(ctx context.Context, domain string) map[string]map[string]bool {
	type found struct {
		name   string
		source string
	}

	ch := make(chan found, 256)

	var wg sync.WaitGroup
	for _, s := range p.opts.Sources {
		wg.Add(1)
		go func(s source.Source) {
			defer wg.Done()

			// Each source gets its own deadline, so one unresponsive endpoint
			// costs its own budget and not the run's.
			sctx, cancel := context.WithTimeout(ctx, p.opts.SourceTimeout)
			defer cancel()

			inner := make(chan string, 128)
			go func() {
				defer close(inner)
				if err := s.Run(sctx, domain, inner); err != nil {
					p.logf("log.source_failed", s.Name(), err)
				}
			}()

			count := 0
			for name := range inner {
				count++
				select {
				case ch <- found{name: name, source: s.Name()}:
				case <-ctx.Done():
					return
				}
			}
			p.logf("log.source_done", s.Name(), count)
		}(s)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	out := make(map[string]map[string]bool)
	for f := range ch {
		if out[f.name] == nil {
			out[f.name] = make(map[string]bool)
		}
		out[f.name][f.source] = true
	}
	return out
}

// resolveAll resolves every reported name in parallel. A name that does not
// resolve is not evidence that it exists, so it is left out of the result set
// and mentioned on the progress stream instead.
//
// wildcardChecked says whether the wildcard verdict is real: when the check
// never completed, no name gets a wildcard field rather than a false one.
func (p *Pipeline) resolveAll(ctx context.Context, domain string, reported map[string]map[string]bool, wildcard *resolve.Wildcard, wildcardChecked bool) []model.Host {
	names := sortedKeys(reported)
	hosts := make([]model.Host, len(names))
	resolved := make([]bool, len(names))

	var wg sync.WaitGroup
	sem := make(chan struct{}, p.opts.Concurrency)

	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := p.opts.Resolver.Lookup(ctx, name)
			if err != nil {
				p.logf("log.lookup_failed", name, err)
				return
			}
			// A name that does not resolve is not evidence that it exists, so
			// it is left out. resolveAll reports the count; printing each one
			// would bury the progress stream under hundreds of lines on a real
			// domain, which is exactly when the stream is worth reading.
			if !res.Found() {
				return
			}

			h := model.Host{
				Host:    name,
				Sources: sortedKeys(reported[name]),
				IPs:     res.IPs,
				CNAME:   res.CNAME,
			}
			if wildcardChecked {
				h.Wildcard = model.Bool(wildcard.Covers(res))
			}
			hosts[i] = h
			resolved[i] = true
		}(i, name)
	}
	wg.Wait()

	out := make([]model.Host, 0, len(hosts))
	for i := range hosts {
		if resolved[i] {
			out = append(out, hosts[i])
		}
	}
	p.logf("log.resolved", len(out), len(names))
	return out
}

func (p *Pipeline) logf(key string, args ...any) {
	if p.opts.Silent {
		return
	}
	fmt.Fprintln(p.opts.Log, i18n.T(p.opts.Lang, key, args...))
}

// sortedKeys returns the keys of a set-like map, sorted, so a run's output does
// not depend on map iteration order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
