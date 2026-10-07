package source

import (
	"testing"
)

// TestCleanIsTheDomainFilter matters because every source funnels through it:
// a certificate names hosts from other domains, a search index answers with
// whatever matched, and an HTML page contains everything on it.
func TestCleanIsTheDomainFilter(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a.example.com", "a.example.com"},
		{"A.Example.COM", "a.example.com"},
		{"*.a.example.com", "a.example.com"},
		{"a.example.com.", "a.example.com"},
		{".a.example.com", "a.example.com"},
		{"deep.a.example.com", "deep.a.example.com"},
		{"example.com", ""},
		{"evil.com", ""},
		{"a.evil.com", ""},
		{"example.com.evil.com", ""},
		{"a b.example.com", ""},
		{"user@example.com", ""},
		{"a/b.example.com", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := clean(tc.in, "example.com"); got != tc.want {
			t.Errorf("clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSelect(t *testing.T) {
	all, err := Select("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(catalogue) {
		t.Errorf("empty spec should mean every source, got %d of %d", len(all), len(catalogue))
	}

	both, err := Select("all")
	if err != nil || len(both) != len(catalogue) {
		t.Errorf("all = %d sources, err %v", len(both), err)
	}

	two, err := Select("crtsh, urlscan")
	if err != nil {
		t.Fatal(err)
	}
	if len(two) != 2 || two[0].Name() != "crtsh" || two[1].Name() != "urlscan" {
		t.Errorf("got %v", names(two))
	}

	if _, err := Select("nope"); err == nil {
		t.Error("an unknown source should be an error, not a silent omission")
	}
}

func TestExclude(t *testing.T) {
	all, _ := Select("all")
	kept := Exclude(all, "crtsh,otx")

	if len(kept) != len(all)-2 {
		t.Fatalf("got %d sources, want %d", len(kept), len(all)-2)
	}
	for _, s := range kept {
		if s.Name() == "crtsh" || s.Name() == "otx" {
			t.Errorf("%s should have been excluded", s.Name())
		}
	}
	if len(Exclude(all, "")) != len(all) {
		t.Error("an empty exclusion list should change nothing")
	}
}

func TestNamesAreSortedAndUnique(t *testing.T) {
	got := Names()
	if len(got) != len(catalogue) {
		t.Fatalf("got %d names for %d sources", len(got), len(catalogue))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("names should be sorted and unique: %v", got)
			break
		}
	}
}

// TestComplaint covers the endpoint that reports failure inside a 200.
func TestComplaint(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"www.example.com,1.2.3.4", ""},
		{"www.example.com,1.2.3.4\nmail.example.com,1.2.3.5", ""},
		{"API count exceeded", "API count exceeded"},
		{"error check your search parameter", "error check your search parameter"},
		{"", "empty response"},
		{"   ", "empty response"},
	}
	for _, tc := range cases {
		if got := complaint(tc.in); got != tc.want {
			t.Errorf("complaint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHostOf(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://a.example.com/path", "a.example.com"},
		{"https://a.example.com:8443/x?y=1", "a.example.com"},
		{"http://a.example.com", "a.example.com"},
		{"example.com", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := hostOf(tc.in); got != tc.want {
			t.Errorf("hostOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNameRegexFindsNamesInHTML covers the one source with no API: the page is
// searched for anything name-shaped and the domain filter does the rest.
func TestNameRegexFindsNamesInHTML(t *testing.T) {
	page := `<tr><td>api.example.com</td><td>1.2.3.4</td></tr>
	          <tr><td>mail.example.com</td><td>1.2.3.5</td></tr>
	          <script src="https://cdn.other.net/app.js"></script>`

	got := nameRE.FindAllString(page, -1)

	var kept []string
	for _, n := range got {
		if c := clean(n, "example.com"); c != "" {
			kept = append(kept, c)
		}
	}
	if len(kept) != 2 {
		t.Fatalf("want the two names under the domain, got %v (raw: %v)", kept, got)
	}
	for _, want := range []string{"api.example.com", "mail.example.com"} {
		found := false
		for _, k := range kept {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q should have been found in %v", want, kept)
		}
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines("a.example.com\nb.example.com\r\nc.example.com")
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	if got := splitLines("only.example.com"); len(got) != 1 {
		t.Errorf("a single name should come back as one element, got %v", got)
	}
}

func names(sources []Source) []string {
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.Name())
	}
	return out
}
