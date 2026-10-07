package scope

import "testing"

// TestExactPatternMatchesOnlyItself: writing "example.com" leaves out that one
// name. Guessing that the user meant the whole domain is how a scan reports a
// host they asked you to skip.
func TestExactPatternMatchesOnlyItself(t *testing.T) {
	l := Parse([]string{"example.com"})

	if !l.Excludes("example.com") {
		t.Error("the named host should be excluded")
	}
	if l.Excludes("api.example.com") {
		t.Error("an exact pattern must not reach into subdomains")
	}
}

// TestWildcardPatternCoversTheDomain covers the two spellings that mean a whole
// domain, and the lookalikes neither may swallow.
func TestWildcardPatternCoversTheDomain(t *testing.T) {
	// *.example.com is the subdomains, which is what a wildcard means in DNS
	// and in a certificate: the domain itself is not covered.
	subdomains := Parse([]string{"*.example.com"})
	for _, name := range []string{"api.example.com", "deep.api.example.com"} {
		if !subdomains.Excludes(name) {
			t.Errorf("*.example.com should cover %s", name)
		}
	}
	if subdomains.Excludes("example.com") {
		t.Error("*.example.com does not name the domain itself")
	}

	// .example.com is the domain and everything under it.
	whole := Parse([]string{".example.com"})
	for _, name := range []string{"example.com", "api.example.com", "deep.api.example.com"} {
		if !whole.Excludes(name) {
			t.Errorf(".example.com should cover %s", name)
		}
	}

	for _, l := range []*List{subdomains, whole} {
		for _, name := range []string{"notexample.com", "example.com.evil.net", "example.org"} {
			if l.Excludes(name) {
				t.Errorf("a pattern for example.com must not match %s", name)
			}
		}
	}
}

func TestCaseAndTrailingDotAreIgnored(t *testing.T) {
	l := Parse([]string{"API.Example.COM."})

	if !l.Excludes("api.example.com") {
		t.Error("matching should not depend on case or a trailing dot")
	}
	if !l.Excludes("API.EXAMPLE.COM") {
		t.Error("the host side should be normalised too")
	}
}

func TestSeveralPatterns(t *testing.T) {
	l := Parse([]string{"mail.example.com", "*.internal.example.com", "cdn.example.com"})

	if !l.Excludes("mail.example.com") {
		t.Error("mail should be excluded")
	}
	if !l.Excludes("api.internal.example.com") {
		t.Error("the internal wildcard should apply")
	}
	if l.Excludes("www.example.com") {
		t.Error("a host nobody named should survive")
	}
}

// TestNilAndEmptyExcludeNothing matters because the pipeline holds a nil list
// when --exclude was not given.
func TestNilAndEmptyExcludeNothing(t *testing.T) {
	var nilList *List
	if nilList.Excludes("a.example.com") {
		t.Error("a nil list excludes nothing")
	}
	if !nilList.Empty() {
		t.Error("a nil list is empty")
	}
	if nilList.Len() != 0 {
		t.Error("a nil list has no patterns")
	}

	empty := Parse(nil)
	if !empty.Empty() {
		t.Error("no patterns is empty")
	}
	if empty.Excludes("a.example.com") {
		t.Error("no patterns excludes nothing")
	}
}

func TestSplit(t *testing.T) {
	got := Split(" a.example.com , *.b.com ,, ")
	want := []string{"a.example.com", "*.b.com"}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if Split("  ") != nil {
		t.Error("blank input should produce no patterns")
	}
}

func TestBareWildcardsAreIgnored(t *testing.T) {
	// "*" and "." alone would exclude everything; that is a typo, not an
	// instruction, and silently reporting nothing is worse than ignoring it.
	if !Parse([]string{"*"}).Empty() {
		t.Error(`"*" should be ignored`)
	}
	if !Parse([]string{"."}).Empty() {
		t.Error(`"." should be ignored`)
	}
}
