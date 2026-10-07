package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/guaidao2/fastsub/internal/model"
)

func sampleHosts() []model.Host {
	return []model.Host{
		{
			Host:    "a.example.com",
			Sources: []string{"crtsh", "urlscan"},
			IPs:     []string{"1.2.3.4"},
			CNAME:   "lb.example.net",
			Alive:   model.Bool(true),
			URLs: []model.URL{
				{URL: "https://a.example.com/", Scheme: "https", Port: 443, StatusCode: 200, Title: "Example", Server: "nginx"},
			},
		},
		{Host: "b.example.com", Sources: []string{"crtsh"}, Wildcard: model.Bool(true)},
	}
}

func render(t *testing.T, f Format, hosts []model.Host, verbose bool) string {
	t.Helper()
	var buf bytes.Buffer
	p := New(&buf, Options{Format: f, Verbose: verbose})
	for _, h := range hosts {
		if err := p.Host(h); err != nil {
			t.Fatalf("Host: %v", err)
		}
	}
	if err := p.Close(model.NewSummary(hosts, 1.5)); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.String()
}

// TestTextIsPipeSafe is the property the format exists for: one bare host per
// line, so `fastsub ... | argus -iL -` gets names and nothing else.
func TestTextIsPipeSafe(t *testing.T) {
	got := render(t, Text, sampleHosts(), false)

	want := "a.example.com\nb.example.com\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextVerboseCarriesDetail(t *testing.T) {
	got := render(t, Text, sampleHosts(), true)

	for _, want := range []string{"1.2.3.4", "[200]", "https://a.example.com/", "[wildcard]"} {
		if !strings.Contains(got, want) {
			t.Errorf("verbose text should contain %q, got %q", want, got)
		}
	}
	if !strings.HasPrefix(got, "a.example.com") {
		t.Errorf("the host should still lead the line, got %q", got)
	}
}

// TestJSONLIsOneRecordPerLine covers the machine contract: a stream a reader
// can consume line by line, with the schema named on the summary.
func TestJSONLIsOneRecordPerLine(t *testing.T) {
	got := render(t, JSONL, sampleHosts(), false)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("want 2 hosts and a summary, got %d lines: %q", len(lines), got)
	}

	var first model.HostRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("host line is not JSON: %v", err)
	}
	if first.Type != model.TypeHost || first.Host.Host != "a.example.com" {
		t.Errorf("unexpected first record: %+v", first)
	}

	var last model.Summary
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil {
		t.Fatalf("summary line is not JSON: %v", err)
	}
	if last.Type != model.TypeSummary {
		t.Errorf("last line should be the summary, got %q", last.Type)
	}
	if last.Schema != model.SchemaVersion {
		t.Errorf("schema = %q, want %q", last.Schema, model.SchemaVersion)
	}
	if last.Hosts != 2 || last.Alive != 1 || last.Wildcard != 1 {
		t.Errorf("summary counts are wrong: %+v", last)
	}
}

func TestJSONIsOneDocument(t *testing.T) {
	got := render(t, JSON, sampleHosts(), false)

	var doc struct {
		Schema  string        `json:"schema"`
		Hosts   []model.Host  `json:"hosts"`
		Summary model.Summary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("output is not one JSON document: %v\n%s", err, got)
	}
	if doc.Schema != model.SchemaVersion {
		t.Errorf("schema = %q", doc.Schema)
	}
	if len(doc.Hosts) != 2 {
		t.Errorf("want 2 hosts, got %d", len(doc.Hosts))
	}
	if doc.Summary.Hosts != 2 {
		t.Errorf("summary should count both hosts, got %+v", doc.Summary)
	}
}

// TestJSONWithNoHostsStillValid guards the lazy array: a run that found nothing
// must still produce a document a parser accepts.
func TestJSONWithNoHostsStillValid(t *testing.T) {
	got := render(t, JSON, nil, false)

	var doc map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("empty output is not valid JSON: %v\n%s", err, got)
	}
}

func TestCSVHasHeaderAndOneRowPerEndpoint(t *testing.T) {
	got := render(t, CSV, sampleHosts(), false)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("want header plus two rows, got %d: %q", len(lines), got)
	}
	if lines[0] != strings.Join(csvHeader, ",") {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "a.example.com,") {
		t.Errorf("first row should start with the host, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "https://a.example.com/") {
		t.Errorf("the endpoint should be on its own row, got %q", lines[1])
	}
	// A host with no endpoint still gets a row, so nothing is silently lost.
	if !strings.HasPrefix(lines[2], "b.example.com,") {
		t.Errorf("host without an endpoint should still appear, got %q", lines[2])
	}
}

func TestURLFormatOnlyPrintsURLs(t *testing.T) {
	got := render(t, URL, sampleHosts(), false)

	if got != "https://a.example.com/\n" {
		t.Errorf("got %q", got)
	}
}

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"text", "json", "jsonl", "csv", "url", "JSONL"} {
		if _, valid := ParseFormat(ok); !valid {
			t.Errorf("%q should be accepted", ok)
		}
	}
	if _, valid := ParseFormat("yaml"); valid {
		t.Error("yaml is not a format")
	}
}
