package pipeline

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/guaidao2/fastsub/internal/baseline"
	"github.com/guaidao2/fastsub/internal/model"
	"github.com/guaidao2/fastsub/internal/resolve"
	"github.com/guaidao2/fastsub/internal/scope"
	"github.com/guaidao2/fastsub/internal/source"
)

// fakeSource reports whatever it was told to, so a run can be tested without a
// network or a source that might be having a bad day.
type fakeSource struct {
	name  string
	names []string
	calls []string // the domains this source was asked about
}

func (f *fakeSource) Name() string   { return f.name }
func (f *fakeSource) NeedsKey() bool { return false }

func (f *fakeSource) Run(ctx context.Context, domain string, out chan<- string) error {
	f.calls = append(f.calls, domain)
	for _, n := range f.names {
		select {
		case out <- n:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// fakeResolver answers from a table, and reports a wildcard when asked to.
type fakeResolver struct {
	answers  map[string]resolve.Result
	wildcard *resolve.Wildcard
	asked    []string
}

func (f *fakeResolver) Lookup(ctx context.Context, name string) (resolve.Result, error) {
	f.asked = append(f.asked, name)
	return f.answers[name], nil
}

func (f *fakeResolver) DetectWildcard(ctx context.Context, domain string) (*resolve.Wildcard, error) {
	return f.wildcard, nil
}

// recorder collects what a run reported.
type recorder struct {
	mu    sync.Mutex
	hosts []model.Host
	sum   model.Summary
}

func (r *recorder) Host(h model.Host) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hosts = append(r.hosts, h)
	return nil
}

func (r *recorder) Close(s model.Summary) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sum = s
	return nil
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.hosts))
	for _, h := range r.hosts {
		out = append(out, h.Host)
	}
	return out
}

// TestAdoptSANsTakesOnlyUsableNames covers what a certificate's subject
// alternative names are good for, and everything they are not: a wildcard is a
// pattern, the domain itself is already known, an unrelated domain is out of
// scope, and a name already reported is not new.
func TestAdoptSANsTakesOnlyUsableNames(t *testing.T) {
	res := &fakeResolver{answers: map[string]resolve.Result{
		"api.example.com": {IPs: []string{"1.1.1.1"}},
	}}
	p := New(Options{Domains: []string{"example.com"}, Resolver: res, Log: io.Discard})

	reported := map[string]map[string]bool{"www.example.com": {"crtsh": true}}
	hosts := p.adoptSANs(context.Background(), "example.com", []string{
		"*.example.com",        // a pattern, not a host
		"example.com",          // the domain itself
		"www.example.com",      // already known
		"api.example.com",      // the one that is new
		"api.example.com.",     // the same name with a dot
		"other.net",            // outside the domain
		"deep.api.example.com", // also new
	}, reported, nil)

	got := make([]string, 0, len(hosts))
	for _, h := range hosts {
		got = append(got, h.Host)
	}
	if len(got) != 1 || got[0] != "api.example.com" {
		t.Fatalf("got %v, want only api.example.com (deep.api has no answer)", got)
	}
	if reported["api.example.com"] == nil {
		t.Fatal("the adopted name should be added to the reported set")
	}
	if !reported["api.example.com"]["certificate"] {
		t.Error("the source of an adopted name is the certificate it came from")
	}
	if reported["*.example.com"] != nil {
		t.Error("a wildcard must not be adopted as a name")
	}
}

// TestParentOf is the arithmetic the recursive walk stands on.
func TestParentOf(t *testing.T) {
	cases := []struct{ name, root, want string }{
		{"api.dev.example.com", "example.com", "dev.example.com"},
		{"a.b.c.example.com", "example.com", "b.c.example.com"},
		{"dev.example.com", "example.com", ""},
		{"www.example.com", "example.com", ""},
		{"example.com", "example.com", ""},
		{"api.other.net", "example.com", ""},
		{"api.dev.example.com.evil.net", "example.com", ""},
	}
	for _, tc := range cases {
		if got := parentOf(tc.name, tc.root); got != tc.want {
			t.Errorf("parentOf(%q, %q) = %q, want %q", tc.name, tc.root, got, tc.want)
		}
	}
}

func TestChildrenSkipsTheRootAndAlreadyWalked(t *testing.T) {
	p := New(Options{Log: io.Discard})
	hosts := []model.Host{
		{Host: "api.dev.example.com"},
		{Host: "www.example.com"},
		{Host: "a.b.ops.example.com"},
		{Host: "cdn.ops.example.com"},
	}

	got := p.children("example.com", hosts, map[string]bool{"ops.example.com": true})

	want := []string{"b.ops.example.com", "dev.example.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestChildrenIsCapped(t *testing.T) {
	p := New(Options{Log: io.Discard})

	hosts := make([]model.Host, 0, MaxChildren*2)
	for i := 0; i < MaxChildren*2; i++ {
		hosts = append(hosts, model.Host{Host: fmt.Sprintf("host%d.sub%d.example.com", i, i)})
	}

	got := p.children("example.com", hosts, map[string]bool{})
	if len(got) != MaxChildren {
		t.Errorf("got %d children, want the %d cap", len(got), MaxChildren)
	}
}

// TestRunReportsResolvedHosts is the chain end to end with nothing external:
// a source reports names, the resolver answers for some of them, and only those
// are reported.
func TestRunReportsResolvedHosts(t *testing.T) {
	src := &fakeSource{name: "fake", names: []string{"a.example.com", "b.example.com", "c.example.com"}}
	res := &fakeResolver{answers: map[string]resolve.Result{
		"a.example.com": {IPs: []string{"1.2.3.4"}},
		"c.example.com": {IPs: []string{"5.6.7.8"}},
	}}

	p := New(Options{
		Domains:  []string{"example.com"},
		Sources:  []source.Source{src},
		Resolver: res,
		Log:      io.Discard,
	})
	rec := &recorder{}
	summary, err := p.Run(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}

	got := rec.names()
	if len(got) != 2 || got[0] != "a.example.com" || got[1] != "c.example.com" {
		t.Errorf("got %v, want the two that resolve", got)
	}
	if summary.Hosts != 2 {
		t.Errorf("summary counts %d hosts, want 2", summary.Hosts)
	}
}

// TestWildcardOnlyNamesAreDropped is the judgement the wordlist depends on.
func TestWildcardOnlyNamesAreDropped(t *testing.T) {
	src := &fakeSource{name: "fake", names: []string{"real.example.com", "invented.example.com"}}
	res := &fakeResolver{
		answers: map[string]resolve.Result{
			"real.example.com":     {IPs: []string{"9.9.9.9"}},
			"invented.example.com": {IPs: []string{"1.2.3.4"}},
		},
		wildcard: &resolve.Wildcard{Domain: "example.com", IPs: []string{"1.2.3.4"}},
	}

	p := New(Options{
		Domains:        []string{"example.com"},
		Sources:        []source.Source{src},
		Resolver:       res,
		WildcardFilter: true,
		Log:            io.Discard,
	})
	rec := &recorder{}
	if _, err := p.Run(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	got := rec.names()
	if len(got) != 1 || got[0] != "real.example.com" {
		t.Errorf("got %v, want only the name with an address of its own", got)
	}
}

func TestExcludeAndBaselineFilterBeforeResolving(t *testing.T) {
	src := &fakeSource{name: "fake", names: []string{"keep.example.com", "skip.example.com", "known.example.com"}}
	res := &fakeResolver{answers: map[string]resolve.Result{
		"keep.example.com": {IPs: []string{"1.1.1.1"}},
	}}

	p := New(Options{
		Domains:  []string{"example.com"},
		Sources:  []source.Source{src},
		Resolver: res,
		Exclude:  scope.Parse([]string{"skip.example.com"}),
		Baseline: baseline.Set{"known.example.com": true},
		Log:      io.Discard,
	})
	rec := &recorder{}
	if _, err := p.Run(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	if len(res.asked) != 1 || res.asked[0] != "keep.example.com" {
		t.Errorf("only the surviving name should reach the resolver, got %v", res.asked)
	}
	if got := rec.names(); len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

// TestRecursionWalksOneLevelDeeper: a name three labels deep makes its parent
// worth enumerating, because nothing under it is visible from the root.
func TestRecursionWalksOneLevelDeeper(t *testing.T) {
	root := &fakeSource{name: "fake", names: []string{"api.dev.example.com"}}
	res := &fakeResolver{answers: map[string]resolve.Result{
		"api.dev.example.com": {IPs: []string{"1.1.1.1"}},
	}}
	// The source reports the same list whatever it is asked, so the second call
	// is what proves the walk happened.
	p := New(Options{
		Domains:   []string{"example.com"},
		Sources:   []source.Source{root},
		Resolver:  res,
		Recursive: true,
		Depth:     1,
		Log:       io.Discard,
	})
	rec := &recorder{}
	if _, err := p.Run(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	if len(root.calls) != 2 {
		t.Fatalf("expected the root and one parent to be enumerated, got %v", root.calls)
	}
	if root.calls[0] != "example.com" || root.calls[1] != "dev.example.com" {
		t.Errorf("walk order = %v", root.calls)
	}
}

func TestRecursionStopsAtDepth(t *testing.T) {
	src := &fakeSource{name: "fake", names: []string{"a.b.c.example.com"}}
	// The name has to resolve for its parent to be worth walking: recursion
	// follows what exists, not what a source once saw.
	res := &fakeResolver{answers: map[string]resolve.Result{
		"a.b.c.example.com": {IPs: []string{"1.1.1.1"}},
	}}

	p := New(Options{
		Domains:   []string{"example.com"},
		Sources:   []source.Source{src},
		Resolver:  res,
		Recursive: true,
		Depth:     1,
		Log:       io.Discard,
	})
	if _, err := p.Run(context.Background(), &recorder{}); err != nil {
		t.Fatal(err)
	}

	// example.com -> c.example.com, and no further: depth 1 means one step.
	if len(src.calls) != 2 {
		t.Errorf("got %v, want the root and one level", src.calls)
	}
}

func TestRunWithoutResolverFails(t *testing.T) {
	p := New(Options{
		Domains: []string{"example.com"},
		Sources: []source.Source{
			&fakeSource{name: "fake", names: []string{"a.example.com"}},
		},
		Log: io.Discard,
	})
	if _, err := p.Run(context.Background(), &recorder{}); err == nil {
		t.Error("a run that resolves names needs a resolver")
	}
}
