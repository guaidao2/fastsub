// Package model is the shape every stage shares: what fastsub knows about one
// name, and what the hosts answering for it turned out to be.
//
// The JSON tags are part of the contract with the rest of the toolchain, so
// they are added to but never renamed. Machine-readable output is language
// neutral: a field name does not change with --lang.
package model

// SchemaVersion is written into machine-readable output. A consumer can refuse
// a version it does not understand rather than guess at the field names.
const SchemaVersion = "fastsub/v1"

// Record types, one per JSONL line, so a reader can tell a host from the
// summary without depending on position.
const (
	TypeHost    = "host"
	TypeSummary = "summary"
)

// URL is one HTTP(S) endpoint that answered for a host.
type URL struct {
	URL        string `json:"url"`
	Scheme     string `json:"scheme"`
	Port       int    `json:"port"`
	StatusCode int    `json:"status_code"`
	Title      string `json:"title,omitempty"`
	Server     string `json:"server,omitempty"`
	ContentLen int64  `json:"content_length,omitempty"`
	Redirect   string `json:"redirect,omitempty"`
	// FaviconHash is the MurmurHash3 of the icon, the number Shodan and FOFA
	// group hosts by. It belongs to the application rather than the host.
	FaviconHash int32 `json:"favicon_hash,omitempty"`
}

// Host is one name and everything fastsub learned about it.
type Host struct {
	// Host is the fully qualified name, without a trailing dot.
	Host string `json:"host"`
	// Sources names the passive sources that reported it, sorted.
	Sources []string `json:"sources,omitempty"`
	// IPs are the A and AAAA answers, sorted, deduplicated.
	IPs []string `json:"ips,omitempty"`
	// CNAME is the canonical name when one was followed.
	CNAME string `json:"cname,omitempty"`
	// Wildcard is true when this name only resolved because the zone answers
	// every name under it. Such a result is not evidence that the name exists.
	Wildcard bool `json:"wildcard,omitempty"`
	// Alive is true when at least one URL answered.
	Alive bool `json:"alive,omitempty"`
	// URLs are the endpoints that answered, in probe order.
	URLs []URL `json:"urls,omitempty"`
}

// HostRecord is one JSONL host line.
type HostRecord struct {
	Type string `json:"type"`
	Host
}

// Record wraps a host in its JSONL envelope.
func (h Host) Record() HostRecord {
	return HostRecord{Type: TypeHost, Host: h}
}

// Summary is the last JSONL line, and the envelope of a JSON document.
type Summary struct {
	Type           string  `json:"type"`
	Schema         string  `json:"schema"`
	Hosts          int     `json:"hosts"`
	Alive          int     `json:"alive"`
	Wildcard       int     `json:"wildcard,omitempty"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
}

// NewSummary counts what was found.
func NewSummary(hosts []Host, elapsed float64) Summary {
	s := Summary{
		Type:           TypeSummary,
		Schema:         SchemaVersion,
		Hosts:          len(hosts),
		ElapsedSeconds: elapsed,
	}
	for _, h := range hosts {
		if h.Alive {
			s.Alive++
		}
		if h.Wildcard {
			s.Wildcard++
		}
	}
	return s
}
