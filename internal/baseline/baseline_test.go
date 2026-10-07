package baseline

import (
	"os"
	"path/filepath"
	"testing"
)

func snapshot(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadReadsFastsubsOwnOutput is the case that matters: yesterday's run is
// today's baseline.
func TestLoadReadsFastsubsOwnOutput(t *testing.T) {
	path := snapshot(t, `{"type":"host","host":"a.example.com","ips":["1.2.3.4"]}
{"type":"host","host":"b.example.com"}
{"type":"summary","schema":"fastsub/v1","hosts":2,"alive":0,"elapsed_seconds":1.2}
`)

	set, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("a.example.com") || !set.Has("b.example.com") {
		t.Errorf("both hosts should be in the snapshot, got %v", keys(set))
	}
	if set.Has("summary") || set.Has("hosts") {
		t.Errorf("only host names count, got %v", keys(set))
	}
	if set.Len() != 2 {
		t.Errorf("len = %d, want 2", set.Len())
	}
}

func TestLoadReadsAJSONDocument(t *testing.T) {
	path := snapshot(t, `{"schema":"fastsub/v1","hosts":[{"host":"a.example.com"},{"host":"b.example.com"}],"summary":{"hosts":2}}`)

	set, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("a.example.com") || !set.Has("b.example.com") {
		t.Errorf("got %v", keys(set))
	}
}

func TestLoadReadsCSVWithoutTheHeader(t *testing.T) {
	path := snapshot(t, `host,ips,cname,sources,wildcard,alive,url,scheme,port,status_code,title,server
a.example.com,1.2.3.4,,crtsh,false,false,,,,,
b.example.com,,,,false,true,https://b.example.com/,https,443,200,Hi,nginx
`)

	set, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if set.Has("host") {
		t.Error("the header row names columns, not a host")
	}
	if !set.Has("a.example.com") || !set.Has("b.example.com") {
		t.Errorf("got %v", keys(set))
	}
}

func TestLoadReadsAPlainHostList(t *testing.T) {
	path := snapshot(t, "a.example.com\n\nb.example.com\n# a comment\n")

	set, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("a.example.com") || !set.Has("b.example.com") {
		t.Errorf("got %v", keys(set))
	}
}

// TestLoadReadsVerboseText: a previous run with -v has extra columns, and the
// name is still the first one.
func TestLoadReadsVerboseText(t *testing.T) {
	path := snapshot(t, "a.example.com  1.2.3.4  [200] https://a.example.com/  \"Hi\"\n")

	set, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("a.example.com") {
		t.Errorf("got %v", keys(set))
	}
}

func TestHasIgnoresCaseAndTrailingDot(t *testing.T) {
	set := Set{}
	set.add("API.Example.COM.")

	if !set.Has("api.example.com") {
		t.Error("a snapshot should match regardless of case")
	}
	if !set.Has("API.EXAMPLE.COM") {
		t.Error("the lookup side should be normalised too")
	}
	if set.Has("other.example.com") {
		t.Error("an unrelated host should not match")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("a missing snapshot should be an error, not an empty baseline")
	}
}

func TestEmptySnapshot(t *testing.T) {
	set, err := Load(snapshot(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if set.Len() != 0 {
		t.Errorf("len = %d", set.Len())
	}
}

func keys(set Set) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
