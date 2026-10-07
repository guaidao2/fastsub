package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/guaidao2/fastsub/internal/i18n"
)

func parse(t *testing.T, args ...string) *FlagSet {
	t.Helper()
	fs := newFlagSet(i18n.EN)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse(%q) failed: %v", args, err)
	}
	return fs
}

// TestLongestShortMatchWins is the reason -oJ does not read as -o with the
// value "J". argus spells its outputs the same way, so this has to keep working.
func TestLongestShortMatchWins(t *testing.T) {
	fs := parse(t, "-oJ", "out.json")

	if got := fs.StringValue("output-json"); got != "out.json" {
		t.Errorf("--output-json = %q, want out.json", got)
	}
	if fs.IsSet("output") {
		t.Error("-oJ must not also set -o")
	}
}

func TestShortFormsAndEquals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"separate value", []string{"-o", "a.txt"}, "a.txt"},
		{"equals value", []string{"-o=a.txt"}, "a.txt"},
		{"glued value", []string{"-oa.txt"}, "a.txt"},
		{"long separate", []string{"--output", "a.txt"}, "a.txt"},
		{"long equals", []string{"--output=a.txt"}, "a.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parse(t, tc.args...).StringValue("output"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRepeatableKeepsOrder(t *testing.T) {
	fs := parse(t, "-d", "a.com", "--domain=b.com", "-d=c.com")

	got := fs.StringsValue("domain")
	want := []string{"a.com", "b.com", "c.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLastValueWinsForScalars(t *testing.T) {
	fs := parse(t, "-f", "text", "-f", "jsonl")
	if got := fs.StringValue("format"); got != "jsonl" {
		t.Errorf("got %q, want jsonl", got)
	}
}

func TestBoolCluster(t *testing.T) {
	fs := parse(t, "-vv")
	if !fs.BoolValue("verbose") {
		t.Error("-vv should set --verbose")
	}
}

func TestBoolExplicitFalse(t *testing.T) {
	fs := parse(t, "--no-color=false")
	if fs.BoolValue("no-color") {
		t.Error("--no-color=false should read as false")
	}
	if !fs.IsSet("no-color") {
		t.Error("--no-color=false was given, so it should count as set")
	}
}

func TestPositionalAndDoubleDash(t *testing.T) {
	fs := parse(t, "example.com", "--", "-weird.example.com")

	got := fs.Positional()
	if len(got) != 2 || got[0] != "example.com" || got[1] != "-weird.example.com" {
		t.Errorf("positional = %v", got)
	}
}

// A single dash names stdin, so it must survive as a positional argument
// instead of being read as an option.
func TestSingleDashIsPositional(t *testing.T) {
	fs := parse(t, "-dL", "-")
	if got := fs.StringValue("domain-list"); got != "-" {
		t.Errorf("-dL = %q, want -", got)
	}
}

// -dL has to win over -d, the same way -oJ wins over -o.
func TestDomainListBeatsDomain(t *testing.T) {
	fs := parse(t, "-dL", "domains.txt")

	if got := fs.StringValue("domain-list"); got != "domains.txt" {
		t.Errorf("-dL = %q, want domains.txt", got)
	}
	if fs.IsSet("domain") {
		t.Error("-dL must not also set -d")
	}
}

func TestDurationsAndInts(t *testing.T) {
	fs := parse(t, "--resolve-timeout", "2s", "-c", "50")
	if got := fs.DurationValue("resolve-timeout"); got != 2*time.Second {
		t.Errorf("duration = %v", got)
	}
	if got := fs.IntValue("concurrency"); got != 50 {
		t.Errorf("int = %d", got)
	}
}

func TestUnknownFlag(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	err := fs.Parse([]string{"--bogus"})
	if err == nil {
		t.Fatal("an unknown option should be an error")
	}
	if !strings.Contains(err.Error(), "--bogus") {
		t.Errorf("error should name the option, got %q", err)
	}
}

func TestMissingValue(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	if err := fs.Parse([]string{"--domain"}); err == nil {
		t.Fatal("an option at the end of the line with no value should be an error")
	}
}

func TestBadValues(t *testing.T) {
	cases := [][]string{
		{"-c", "lots"},
		{"--resolve-timeout", "soon"},
	}
	for _, args := range cases {
		fs := newFlagSet(i18n.EN)
		if err := fs.Parse(args); err == nil {
			t.Errorf("Parse(%q) should have failed", args)
		}
	}
}

func TestEveryOptionHasCatalogueHelp(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	for _, d := range fs.defs {
		key := "flag." + d.long
		if got := i18n.T(i18n.ZH, key); got == key {
			t.Errorf("option --%s has no zh help under %q", d.long, key)
		}
	}
}
