package output

import (
	"strings"
	"testing"

	"github.com/guaidao2/fastsub/internal/model"
)

// unmeasured is a host from a run that resolved nothing and probed nothing —
// --only-passive, for instance.
func unmeasured() []model.Host {
	return []model.Host{{Host: "a.example.com", Sources: []string{"urlscan"}}}
}

// TestUnmeasuredFieldsAreNotWrittenAsFalse is the rule the CSV format used to
// break: --only-passive says "never resolve or probe", so a wildcard field of
// "false" was an assertion that nothing had checked.
func TestUnmeasuredFieldsAreNotWrittenAsFalse(t *testing.T) {
	got := render(t, CSV, unmeasured(), false)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	row := strings.Split(lines[1], ",")
	if len(row) != len(csvHeader) {
		t.Fatalf("row has %d fields, want %d: %q", len(row), len(csvHeader), lines[1])
	}
	wildcard, alive := row[4], row[5]
	if wildcard != "" || alive != "" {
		t.Errorf("an unmeasured field must be empty, got wildcard=%q alive=%q", wildcard, alive)
	}
}

func TestJSONLOmitsUnmeasuredFields(t *testing.T) {
	// The first line is the host record; the summary line below it carries
	// counts, which are always present.
	hostLine := strings.Split(render(t, JSONL, unmeasured(), false), "\n")[0]

	if strings.Contains(hostLine, "wildcard") || strings.Contains(hostLine, "alive") {
		t.Errorf("an unmeasured field should not appear at all: %s", hostLine)
	}
	if !strings.Contains(hostLine, `"host":"a.example.com"`) {
		t.Errorf("the host itself must still be there: %s", hostLine)
	}
}

// TestMeasuredFalseIsWritten is the other half: a check that ran and answered
// "no" is a claim, and dropping it would lose information.
func TestMeasuredFalseIsWritten(t *testing.T) {
	hosts := []model.Host{{
		Host:     "a.example.com",
		Wildcard: model.Bool(false),
		Alive:    model.Bool(false),
	}}

	csvOut := render(t, CSV, hosts, false)
	row := strings.Split(strings.Split(strings.TrimRight(csvOut, "\n"), "\n")[1], ",")
	if row[4] != "false" || row[5] != "false" {
		t.Errorf("a measured false should be written, got wildcard=%q alive=%q", row[4], row[5])
	}

	jsonlOut := render(t, JSONL, hosts, false)
	for _, want := range []string{`"wildcard":false`, `"alive":false`} {
		if !strings.Contains(jsonlOut, want) {
			t.Errorf("expected %s in %s", want, jsonlOut)
		}
	}
}

// TestSummaryCountsOnlyMeasuredTrue: the counts describe what was found, so a
// nil flag contributes nothing to either side.
func TestSummaryCountsOnlyMeasuredTrue(t *testing.T) {
	hosts := []model.Host{
		{Host: "a.example.com", Alive: model.Bool(true)},
		{Host: "b.example.com", Alive: model.Bool(false)},
		{Host: "c.example.com"},
	}

	got := model.NewSummary(hosts, 1)
	if got.Alive != 1 {
		t.Errorf("alive = %d, want 1", got.Alive)
	}
	if got.Hosts != 3 {
		t.Errorf("hosts = %d, want 3", got.Hosts)
	}
}

// TestTextVerboseMarksOnlyRealWildcards keeps the human format honest too.
func TestTextVerboseMarksOnlyRealWildcards(t *testing.T) {
	hosts := []model.Host{
		{Host: "a.example.com", Wildcard: model.Bool(true)},
		{Host: "b.example.com", Wildcard: model.Bool(false)},
		{Host: "c.example.com"},
	}

	got := render(t, Text, hosts, true)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if !strings.Contains(lines[0], "[wildcard]") {
		t.Errorf("a measured wildcard should be marked: %q", lines[0])
	}
	for _, line := range lines[1:] {
		if strings.Contains(line, "[wildcard]") {
			t.Errorf("only a real wildcard gets the mark: %q", line)
		}
	}
}
