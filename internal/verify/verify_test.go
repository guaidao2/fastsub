package verify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestProbeAgainstALocalServer is the whole stage end to end: a server that
// answers, a host that resolves to it, and a result that says so.
func TestProbeAgainstALocalServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "test-server/1.0")
		fmt.Fprint(w, `<html><head><title>Hello &amp; welcome</title></head><body>hi</body></html>`)
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}

	p := New(Options{Ports: []int{port}, Timeout: 5 * time.Second, Title: true})
	res := p.Probe(context.Background(), parsed.Hostname())

	if !res.Alive() {
		t.Fatalf("the server answered, so the host is alive; got %+v", res)
	}
	got := res.URLs[0]
	if got.StatusCode != 200 {
		t.Errorf("status = %d, want 200", got.StatusCode)
	}
	if got.Title != "Hello & welcome" {
		t.Errorf("title = %q; the entity should have been decoded", got.Title)
	}
	if got.Server != "test-server/1.0" {
		t.Errorf("server = %q", got.Server)
	}
	if got.Scheme != "http" || got.Port != port {
		t.Errorf("scheme/port = %s/%d, want http/%d", got.Scheme, got.Port, port)
	}
}

// TestProbeCountsAnErrorPageAsAlive: a 403 or a 404 is a server answering. The
// question this stage asks is whether anything is there, not whether it likes
// the request.
func TestProbeCountsAnErrorPageAsAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer srv.Close()

	parsed, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(parsed.Port())

	p := New(Options{Ports: []int{port}, Timeout: 5 * time.Second})
	res := p.Probe(context.Background(), parsed.Hostname())

	if !res.Alive() {
		t.Fatal("a 403 is still an answer")
	}
	if res.URLs[0].StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.URLs[0].StatusCode)
	}
}

func TestProbeOnADeadPortIsNotAlive(t *testing.T) {
	p := New(Options{Ports: []int{9}, Timeout: 500 * time.Millisecond, Concurrency: 2})
	res := p.Probe(context.Background(), "127.0.0.1")

	if res.Alive() {
		t.Errorf("nothing listens on port 9, got %+v", res.URLs)
	}
}

func TestPageTitle(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"plain", "<title>Admin</title>", "Admin"},
		{"attributes and case", `<TITLE class="x">Admin</TITLE>`, "Admin"},
		{"whitespace collapsed", "<title>\n  Hello   world\n</title>", "Hello world"},
		{"entity decoded", "<title>a &amp; b</title>", "a & b"},
		{"missing", "<html><body>no title here</body></html>", ""},
		{"unterminated", "<title>cut off here", "cut off here"},
		{"empty", "<title></title>", ""},
	}
	for _, tc := range cases {
		if got := pageTitle(strings.NewReader(tc.body)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPageTitleIsBounded(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := pageTitle(strings.NewReader("<title>" + long + "</title>"))

	if len(got) > 210 {
		t.Errorf("title should be truncated, got %d characters", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a truncated title should say so, got %q", got[len(got)-10:])
	}
}

func TestTruncateKeepsUTF8Intact(t *testing.T) {
	// 100 Chinese characters, three bytes each.
	got := truncate(strings.Repeat("中", 100), 10)

	if !strings.HasSuffix(got, "...") {
		t.Fatal("expected a truncation marker")
	}
	if strings.Contains(got, "\ufffd") {
		t.Error("truncation should not leave a half character behind")
	}
}

func TestParsePorts(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"", nil},
		{"80", []int{80}},
		{"80,443", []int{80, 443}},
		{" 80 , 443 ", []int{80, 443}},
		{"8000-8003", []int{8000, 8001, 8002, 8003}},
		{"80,80", []int{80}},
		{"81-82,80", []int{81, 82, 80}},
	}
	for _, tc := range cases {
		got, err := ParsePorts(tc.in)
		if err != nil {
			t.Errorf("ParsePorts(%q) failed: %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("ParsePorts(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("ParsePorts(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestParsePortsRejectsNonsense(t *testing.T) {
	for _, spec := range []string{"0", "65536", "http", "80-", "-80", "90-80", "80,abc", "1-2-3"} {
		if got, err := ParsePorts(spec); err == nil {
			t.Errorf("ParsePorts(%q) should have failed, got %v", spec, got)
		}
	}
}
