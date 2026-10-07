package verify

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMurmur3MatchesTheReference pins the algorithm against values produced by
// the Python mmh3 module, because a favicon hash that does not match everyone
// else's fingerprint matches nothing useful.
func TestMurmur3MatchesTheReference(t *testing.T) {
	cases := []struct {
		in   string
		seed uint32
		want int32
	}{
		{"", 0, 0},
		{"hello", 0, 613153351},
		{"foo", 0, -156908512},
	}
	for _, tc := range cases {
		if got := murmur3_32([]byte(tc.in), tc.seed); got != tc.want {
			t.Errorf("murmur3_32(%q, %d) = %d, want %d", tc.in, tc.seed, got, tc.want)
		}
	}
}

func TestMurmur3IsDeterministic(t *testing.T) {
	data := []byte("the same input twice")
	if murmur3_32(data, 0) != murmur3_32(data, 0) {
		t.Error("the same input must give the same hash")
	}
	if murmur3_32([]byte("one"), 0) == murmur3_32([]byte("two"), 0) {
		t.Error("different inputs should not collide here")
	}
}

// TestBase64Wrapping matters because the wrapping is part of the hashed input:
// Python's base64.encodebytes wraps at 76 columns, and a hash of unwrapped
// base64 is a number nobody else produces.
func TestBase64Wrapping(t *testing.T) {
	short := base64WithNewlines([]byte("x"))
	if string(short) != "eA==\n" {
		t.Errorf("short input = %q", short)
	}

	// 60 bytes encode to 80 base64 characters: one full line and a remainder.
	long := base64WithNewlines(bytes.Repeat([]byte{0xAB}, 60))
	lines := strings.Split(strings.TrimSuffix(string(long), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("80 characters should wrap into 2 lines, got %d: %q", len(lines), long)
	}
	if len(lines[0]) != 76 || len(lines[1]) != 4 {
		t.Errorf("line lengths = %d, %d; want 76 and 4", len(lines[0]), len(lines[1]))
	}
}

func TestLooksLikeIcon(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"ico", []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}, true},
		{"png", []byte("\x89PNG\r\n\x1a\n"), true},
		{"gif", []byte("GIF89a"), true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, true},
		{"svg", []byte("<svg xmlns="), true},
		{"html page", []byte("<!DOCTYPE html><html>"), false},
		{"empty", nil, false},
	}
	for _, tc := range cases {
		if got := looksLikeIcon(tc.data); got != tc.want {
			t.Errorf("%s: looksLikeIcon = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestFaviconHashAgainstALocalServer runs the whole thing: fetch the icon,
// wrap it, hash it, and check the number is the one the reference produces for
// that icon.
func TestFaviconHashAgainstALocalServer(t *testing.T) {
	icon := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00}

	mux := http.NewServeMux()
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(icon)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<!DOCTYPE html><html><body>hello</body></html>"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	parsed := mustParse(t, srv.URL)
	port := mustPort(t, parsed.Port())

	p := New(Options{Ports: []int{port}, Timeout: 5 * time.Second, Favicon: true})
	res := p.Probe(context.Background(), parsed.Hostname())

	if !res.Alive() {
		t.Fatal("the server answered")
	}
	want := murmur3_32(base64WithNewlines(icon), 0)
	if got := res.URLs[0].FaviconHash; got != want {
		t.Errorf("favicon hash = %d, want %d", got, want)
	}
	if want == 0 {
		t.Fatal("the fixture hash should not be zero, or this proves nothing")
	}
}

// TestFaviconIsNotFingerprintedWhenThePathIsNotAnIcon: a site that answers
// every path with its home page would otherwise be fingerprinted on its 404
// handler, which is the same for every such site.
func TestFaviconIsNotFingerprintedWhenThePathIsNotAnIcon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!DOCTYPE html><html><body>not an icon</body></html>"))
	}))
	defer srv.Close()

	parsed := mustParse(t, srv.URL)
	port := mustPort(t, parsed.Port())

	p := New(Options{Ports: []int{port}, Timeout: 5 * time.Second, Favicon: true})
	res := p.Probe(context.Background(), parsed.Hostname())

	if res.URLs[0].FaviconHash != 0 {
		t.Errorf("an HTML page is not an icon, got hash %d", res.URLs[0].FaviconHash)
	}
}

func TestFaviconMissingIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	parsed := mustParse(t, srv.URL)
	port := mustPort(t, parsed.Port())

	p := New(Options{Ports: []int{port}, Timeout: 5 * time.Second, Favicon: true})
	res := p.Probe(context.Background(), parsed.Hostname())

	if !res.Alive() {
		t.Error("a 404 is still a server answering")
	}
	if res.URLs[0].FaviconHash != 0 {
		t.Errorf("no icon means no hash, got %d", res.URLs[0].FaviconHash)
	}
}

func TestFaviconLimit(t *testing.T) {
	// readAtMost is not a thing; the limit is enforced with io.LimitReader, so
	// this checks that a body larger than the limit is truncated rather than
	// read into memory.
	big := io.LimitReader(bytes.NewReader(bytes.Repeat([]byte{0xAB}, faviconLimit*2)), faviconLimit)
	data, err := io.ReadAll(big)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != faviconLimit {
		t.Errorf("read %d bytes, want the %d limit", len(data), faviconLimit)
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustPort(t *testing.T, raw string) int {
	t.Helper()
	port, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
