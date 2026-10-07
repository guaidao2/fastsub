package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guaidao2/fastsub/internal/i18n"
	"github.com/guaidao2/fastsub/internal/output"
)

// TestOutputAllWritesEveryFormat: the help says "write every format at once",
// and the formats are text, JSON, JSONL, CSV and URL — so all five have to
// land, not the three that used to.
func TestOutputAllWritesEveryFormat(t *testing.T) {
	base := filepath.Join(t.TempDir(), "out")

	fs := newFlagSet(i18n.EN)
	if err := fs.Parse([]string{"-oA", base}); err != nil {
		t.Fatal(err)
	}
	s, err := buildSink(fs, output.Text, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.cleanup()

	for _, suffix := range []string{".txt", ".json", ".jsonl", ".csv", ".url"} {
		if _, err := os.Stat(base + suffix); err != nil {
			t.Errorf("%s was not written: %v", suffix, err)
		}
	}

	// One printer for stdout, five for the files.
	if len(s.printers) != 6 {
		t.Errorf("got %d printers, want stdout plus five formats", len(s.printers))
	}
}

func TestOutputFollowsTheChosenFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")

	fs := newFlagSet(i18n.EN)
	if err := fs.Parse([]string{"-f", "jsonl", "-o", path}); err != nil {
		t.Fatal(err)
	}
	s, err := buildSink(fs, output.JSONL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.cleanup()

	if len(s.printers) != 2 {
		t.Fatalf("got %d printers, want stdout plus one file", len(s.printers))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("-o did not create its file: %v", err)
	}
}

// TestMistypedLongOptionGetsAHint: -version is --version with one dash too few,
// and reporting the leftover "-ersion" helps nobody.
func TestMistypedLongOptionGetsAHint(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	err := fs.Parse([]string{"-version"})
	if err == nil {
		t.Fatal("-version is not an option and should be an error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "-version") {
		t.Errorf("the error should quote what was typed, got %q", msg)
	}
	if !strings.Contains(msg, "--version") {
		t.Errorf("the error should name the option that was meant, got %q", msg)
	}
}

func TestUnknownShortOptionStillReportsItself(t *testing.T) {
	fs := newFlagSet(i18n.EN)
	err := fs.Parse([]string{"-zzz"})
	if err == nil {
		t.Fatal("-zzz is not an option")
	}
	if !strings.Contains(err.Error(), "-zzz") {
		t.Errorf("the error should quote the option, got %q", err)
	}
}
