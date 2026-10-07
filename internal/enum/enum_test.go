package enum

import (
	"strings"
	"testing"
)

func TestWordBuildsNames(t *testing.T) {
	got := Word("example.com", []string{"api", "mail.example.com", "  ", "# comment", "dev # inline"})

	want := []string{"api.example.com", "mail.example.com", "dev.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestWordAcceptsAFullName matters because a list exported from another tool is
// full names, and feeding it back should not produce name.name.domain.
func TestWordAcceptsAFullName(t *testing.T) {
	got := Word("example.com", []string{"deep.api.example.com"})

	if len(got) != 1 || got[0] != "deep.api.example.com" {
		t.Errorf("got %v", got)
	}
}

func TestWordDedupes(t *testing.T) {
	got := Word("example.com", []string{"api", "api", "API"})

	if len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestNormalizeWord(t *testing.T) {
	cases := []struct{ in, want string }{
		{"api", "api"},
		{"API", "api"},
		{"api.example.com.", "api.example.com"},
		{"*.api", "api"},
		{"api # comment", "api"},
		{"# whole line", ""},
		{"", ""},
		{"  ", ""},
		{"has space", ""},
		{"a/b", ""},
		{"user@host", ""},
	}
	for _, tc := range cases {
		if got := normalizeWord(tc.in); got != tc.want {
			t.Errorf("normalizeWord(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMutateUsesPrefixesSuffixesAndNumbers(t *testing.T) {
	got := Mutate("example.com", []string{"api.example.com"})

	joined := strings.Join(got, " ")
	for _, want := range []string{"dev-api.example.com", "api-dev.example.com", "api1.example.com"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q should have been generated", want)
		}
	}
	// Every candidate belongs to the domain it was generated for.
	for _, name := range got {
		if !strings.HasSuffix(name, ".example.com") {
			t.Fatalf("%q is not under the domain", name)
		}
	}
}

// TestMutateSkipsDeepLabels: permuting every level of a.b.example.com
// multiplies the candidate set by the depth without reaching anything a scan
// would not reach from the label alone.
func TestMutateSkipsDeepLabels(t *testing.T) {
	got := Mutate("example.com", []string{"deep.api.example.com"})

	if len(got) != 0 {
		t.Errorf("a multi-label name should not be permuted wholesale, got %v", got[:min(3, len(got))])
	}
}

func TestMutateIgnoresNamesOutsideTheDomain(t *testing.T) {
	got := Mutate("example.com", []string{"api.other.net", "example.com"})

	if len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestMutateIsBounded(t *testing.T) {
	found := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		found = append(found, strings.Repeat("a", 1+i%5)+itoa(i)+".example.com")
	}

	got := Mutate("example.com", found)

	perLabel := len(Prefixes) + len(Suffixes) + len(Numbers)
	if max := MaxMutateLabels * perLabel; len(got) > max {
		t.Errorf("generated %d candidates, above the %d bound", len(got), max)
	}
	if len(got) == 0 {
		t.Error("the bound should limit the labels, not empty the result")
	}
}

func TestLabelsOf(t *testing.T) {
	got := labelsOf("example.com", []string{
		"api.example.com",
		"deep.api.example.com", // not a single label
		"api.other.net",        // not under the domain
		"example.com",          // the domain itself
		"web.example.com",
		"WEB.EXAMPLE.COM", // same label, different case
	})

	want := []string{"api", "web"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
