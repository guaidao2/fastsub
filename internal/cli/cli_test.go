package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/guaidao2/fastsub/internal/i18n"
)

func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersionExitsZero(t *testing.T) {
	code, out, _ := runCLI(t, "", "-V")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(out, "guaidao2 & coolmoon") {
		t.Errorf("--version should carry the authorship line, got %q", out)
	}
}

func TestHelpIsEnglishByDefault(t *testing.T) {
	code, out, _ := runCLI(t, "", "--help")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"TARGET", "--domain-list", "Authors: guaidao2 & coolmoon"} {
		if !strings.Contains(out, want) {
			t.Errorf("English help should contain %q", want)
		}
	}
}

func TestHelpHonoursLangFlag(t *testing.T) {
	_, out, _ := runCLI(t, "", "--lang", "zh", "--help")
	if !strings.Contains(out, "目标") {
		t.Errorf("--lang zh should produce Chinese help, got %q", out[:min(len(out), 200)])
	}
	if strings.Contains(out, "TARGET") {
		t.Error("Chinese help should not still print the English section titles")
	}
}

func TestHelpHonoursEnvVar(t *testing.T) {
	t.Setenv("FASTSUB_LANG", "zh")
	_, out, _ := runCLI(t, "", "--help")
	if !strings.Contains(out, "目标") {
		t.Error("FASTSUB_LANG=zh should produce Chinese help")
	}
}

func TestNoTargetIsUsageError(t *testing.T) {
	code, _, errs := runCLI(t, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errs, "no target") {
		t.Errorf("stderr should explain the missing target, got %q", errs)
	}
}

func TestUnknownLanguageIsUsageError(t *testing.T) {
	code, _, errs := runCLI(t, "", "--lang", "de", "-d", "example.com")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errs, "de") {
		t.Errorf("stderr should name the language, got %q", errs)
	}
}

func TestListLangs(t *testing.T) {
	code, out, _ := runCLI(t, "", "--list-langs")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "en") || !strings.Contains(out, "zh") {
		t.Errorf("--list-langs should list both languages, got %q", out)
	}
}

func TestTargetsComeFromFlagsAndArguments(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	if err := fs.Parse([]string{"-d", "a.com", "b.com"}); err != nil {
		t.Fatal(err)
	}
	got, err := collectTargets(fs, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.com", "b.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestDomainListIsTheBatchFormOfD: -dL reads root domains, and what it reads is
// enumerated exactly as -d would enumerate it.
func TestDomainListIsTheBatchFormOfD(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	if err := fs.Parse([]string{"-d", "a.com", "-dL", "-"}); err != nil {
		t.Fatal(err)
	}
	got, err := collectTargets(fs, strings.NewReader("# comment\nb.com\n\nc.com # trailing\nb.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.com", "b.com", "c.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The list file is also read by argus, so the two tools have to agree on what a
// blank line and a comment mean.
func TestListFileSharedWithArgus(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	if err := fs.Parse([]string{"-dL", "-"}); err != nil {
		t.Fatal(err)
	}
	got, err := collectTargets(fs, strings.NewReader("10.0.0.1\n# skip me\nexample.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "10.0.0.1" || got[1] != "example.com" {
		t.Errorf("got %v", got)
	}
}
