package resolve

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestLookupReturnsAnAddressAsItsOwnAnswer: a scan that starts from a port
// scanner's output is a normal way to use this, and asking DNS about an address
// is a question with no useful reply. The resolver here is unreachable on
// purpose — an address must not need it.
func TestLookupReturnsAnAddressAsItsOwnAnswer(t *testing.T) {
	r := New(Options{
		Servers:     []string{"192.0.2.1:53"},
		Timeout:     50 * time.Millisecond,
		Concurrency: 2,
	})

	for _, address := range []string{"10.0.0.1", "2001:db8::1"} {
		res, err := r.Lookup(context.Background(), address)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", address, err)
		}
		if len(res.IPs) != 1 || res.IPs[0] != address {
			t.Errorf("Lookup(%q) = %v, want the address back", address, res.IPs)
		}
	}
}

// TestWildcardCovers is the judgement the whole wordlist depends on: a name
// that only resolved because the zone answers for everything must not be
// reported as if it existed.
func TestWildcardCovers(t *testing.T) {
	w := &Wildcard{Domain: "example.com", IPs: []string{"1.2.3.4", "5.6.7.8"}}

	cases := []struct {
		name string
		res  Result
		want bool
	}{
		{"every address is the wildcard's", Result{IPs: []string{"1.2.3.4"}}, true},
		{"both wildcard addresses", Result{IPs: []string{"1.2.3.4", "5.6.7.8"}}, true},
		{"one address of its own", Result{IPs: []string{"1.2.3.4", "9.9.9.9"}}, false},
		{"no addresses at all", Result{}, false},
		{"a CNAME alone proves nothing about the addresses", Result{CNAME: "x.example.net"}, false},
	}
	for _, tc := range cases {
		if got := w.Covers(tc.res); got != tc.want {
			t.Errorf("%s: Covers = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWildcardCoversNilAndEmpty(t *testing.T) {
	var nilWildcard *Wildcard
	if nilWildcard.Covers(Result{IPs: []string{"1.2.3.4"}}) {
		t.Error("a zone with no wildcard covers nothing")
	}

	empty := &Wildcard{Domain: "example.com"}
	if empty.Covers(Result{IPs: []string{"1.2.3.4"}}) {
		t.Error("a wildcard with no addresses covers nothing")
	}
}

func TestRandomLabelsAreUniqueAndDNSShaped(t *testing.T) {
	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		label, err := randomLabel()
		if err != nil {
			t.Fatal(err)
		}
		if len(label) < 8 {
			t.Fatalf("label %q is too short to be unguessable", label)
		}
		if strings.ToLower(label) != label {
			t.Fatalf("label %q should be lower case for DNS", label)
		}
		for _, r := range label {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz234567", r) {
				t.Fatalf("label %q is not base32", label)
			}
		}
		if seen[label] {
			t.Fatalf("label %q came back twice in 200 draws", label)
		}
		seen[label] = true
	}
}

func TestFinishDedupesAndSorts(t *testing.T) {
	got := finish(Result{IPs: []string{"2.2.2.2", "1.1.1.1", "2.2.2.2", "", "1.1.1.1"}})

	want := []string{"1.1.1.1", "2.2.2.2"}
	if len(got.IPs) != len(want) {
		t.Fatalf("got %v, want %v", got.IPs, want)
	}
	for i := range want {
		if got.IPs[i] != want[i] {
			t.Fatalf("got %v, want %v", got.IPs, want)
		}
	}
}

func TestResultFound(t *testing.T) {
	if (Result{}).Found() {
		t.Error("an empty result is not found")
	}
	if !(Result{IPs: []string{"1.1.1.1"}}).Found() {
		t.Error("an address is found")
	}
	if !(Result{CNAME: "a.example.net"}).Found() {
		t.Error("a canonical name is found")
	}
}

// TestResolverFallsBackWhenAServerIsDead is the behaviour a scan needs on a
// network where one of the default resolvers is blocked: the first timeout
// retires it instead of paying that timeout on every name.
func TestResolverFallsBackWhenAServerIsDead(t *testing.T) {
	r := New(Options{
		// 192.0.2.0/24 is reserved for documentation and never answers.
		Servers:     []string{"192.0.2.1:53", "223.5.5.5:53"},
		Timeout:     300 * time.Millisecond,
		Concurrency: 2,
		Attempts:    2,
	})

	if got := r.pick(); got.address == "" {
		t.Fatal("pick returned nothing")
	}
	r.noteFailure("192.0.2.1:53")

	// With one server carrying a failure, every pick should now avoid it.
	for i := 0; i < 5; i++ {
		if got := r.pick(); got.address == "192.0.2.1:53" {
			t.Fatalf("a failed resolver was picked again on draw %d", i)
		}
	}

	// A success clears the record, so an intermittent server comes back.
	r.noteSuccess("192.0.2.1:53")
	if f := r.Failures(); len(f) != 0 {
		t.Errorf("a success should clear the failure record, got %v", f)
	}
}
