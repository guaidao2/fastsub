// Package output renders results in the formats the rest of the toolchain
// reads back. Results go to stdout and progress goes to stderr, so a pipe
// carries data and nothing else.
package output

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/guaidao2/fastsub/internal/model"
)

// Format is an output format.
type Format string

// The formats --format accepts.
const (
	Text  Format = "text"
	JSON  Format = "json"
	JSONL Format = "jsonl"
	CSV   Format = "csv"
	URL   Format = "url"
)

// ParseFormat validates a --format value.
func ParseFormat(s string) (Format, bool) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case Text, JSON, JSONL, CSV, URL:
		return f, true
	}
	return "", false
}

// All lists the formats, for an error message that names the alternatives.
func All() string { return "text, json, jsonl, csv or url" }

// csvHeader is the column order of -f csv.
var csvHeader = []string{
	"host", "ips", "cname", "sources", "wildcard", "alive",
	"url", "scheme", "port", "status_code", "title", "server", "favicon_hash",
}

// Printer writes results as they are found, so a long scan can be piped
// somewhere while it is still running.
type Printer struct {
	w       *bufio.Writer
	format  Format
	verbose bool

	started   bool // json: the array has been opened
	wroteHost bool // json: a host has been written, so the next one needs a comma
	csv       *csv.Writer
	err       error
}

// Options configures a Printer.
type Options struct {
	Format  Format
	Verbose bool
}

// New returns a printer for the given format.
func New(w io.Writer, opts Options) *Printer {
	p := &Printer{
		w:       bufio.NewWriter(w),
		format:  opts.Format,
		verbose: opts.Verbose,
	}
	if p.format == CSV {
		p.csv = csv.NewWriter(p.w)
	}
	return p
}

// Host writes one host. What that means depends on the format: a bare name for
// text, a record per line for JSONL, one row per endpoint for CSV.
func (p *Printer) Host(h model.Host) error {
	if p.err != nil {
		return p.err
	}
	switch p.format {
	case Text:
		p.writeText(h)
	case URL:
		p.writeURL(h)
	case JSONL:
		p.writeJSONL(h)
	case JSON:
		p.writeJSON(h)
	case CSV:
		p.writeCSV(h)
	}
	return p.err
}

// Close finishes the format and writes the summary where the format carries
// one. It must be called exactly once.
func (p *Printer) Close(s model.Summary) error {
	if p.err != nil {
		return p.err
	}
	switch p.format {
	case JSON:
		p.openJSON()
		p.writeString("],\"summary\":")
		p.writeJSONValue(s)
		p.writeString("}")
		p.writeString("\n")
	case JSONL:
		p.writeJSONValue(s)
		p.writeString("\n")
	case CSV:
		if p.csv != nil {
			p.csv.Flush()
			if err := p.csv.Error(); err != nil {
				p.err = err
			}
		}
	}
	if err := p.w.Flush(); err != nil && p.err == nil {
		p.err = err
	}
	return p.err
}

func (p *Printer) writeText(h model.Host) {
	if !p.verbose {
		p.writeString(h.Host + "\n")
		return
	}
	var b strings.Builder
	b.WriteString(h.Host)
	if len(h.IPs) > 0 {
		b.WriteString("  " + strings.Join(h.IPs, ","))
	}
	if model.Measured(h.Wildcard) {
		b.WriteString("  [wildcard]")
	}
	for _, u := range h.URLs {
		fmt.Fprintf(&b, "  [%d] %s", u.StatusCode, u.URL)
		if u.Title != "" {
			fmt.Fprintf(&b, "  %q", u.Title)
		}
		if u.FaviconHash != 0 {
			fmt.Fprintf(&b, "  favicon:%d", u.FaviconHash)
		}
	}
	p.writeString(b.String() + "\n")
}

func (p *Printer) writeURL(h model.Host) {
	for _, u := range h.URLs {
		p.writeString(u.URL + "\n")
	}
}

func (p *Printer) writeJSONL(h model.Host) {
	p.writeJSONValue(h.Record())
	p.writeString("\n")
}

// writeJSON streams the array, which means opening it lazily: a host has to
// arrive before the document can claim to hold any.
func (p *Printer) writeJSON(h model.Host) {
	p.openJSON()
	if p.wroteHost {
		p.writeString(",")
	}
	p.writeJSONValue(h)
	p.wroteHost = true
}

// openJSON writes the document head on first use. Close calls it as well, so a
// run that found nothing still produces a document rather than a fragment.
func (p *Printer) openJSON() {
	if p.started {
		return
	}
	p.writeString("{\"schema\":\"" + model.SchemaVersion + "\",\"hosts\":[")
	p.started = true
}

func (p *Printer) writeCSV(h model.Host) {
	if p.csv == nil {
		return
	}
	if !p.started {
		if err := p.csv.Write(csvHeader); err != nil {
			p.err = err
			return
		}
		p.started = true
	}
	if len(h.URLs) == 0 {
		p.writeCSVRow(h, model.URL{})
		return
	}
	for _, u := range h.URLs {
		p.writeCSVRow(h, u)
	}
}

func (p *Printer) writeCSVRow(h model.Host, u model.URL) {
	row := []string{
		h.Host,
		strings.Join(h.IPs, " "),
		h.CNAME,
		strings.Join(h.Sources, " "),
		boolString(h.Wildcard),
		boolString(h.Alive),
		u.URL,
		u.Scheme,
		portString(u.Port),
		statusString(u.StatusCode),
		u.Title,
		u.Server,
		faviconString(u.FaviconHash),
	}
	if err := p.csv.Write(row); err != nil {
		p.err = err
	}
}

func (p *Printer) writeJSONValue(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		p.err = err
		return
	}
	p.writeBytes(b)
}

func (p *Printer) writeString(s string) { p.writeBytes([]byte(s)) }

func (p *Printer) writeBytes(b []byte) {
	if p.err != nil {
		return
	}
	if _, err := p.w.Write(b); err != nil {
		p.err = err
	}
}

func portString(port int) string {
	if port == 0 {
		return ""
	}
	return strconv.Itoa(port)
}

// statusString leaves a status that was never probed empty rather than writing
// the zero value, which a reader would take for a real answer.
func statusString(code int) string {
	if code == 0 {
		return ""
	}
	return strconv.Itoa(code)
}

// faviconString is empty when no icon was hashed, for the same reason: zero is
// a legitimate hash value.
func faviconString(hash int32) string {
	if hash == 0 {
		return ""
	}
	return strconv.FormatInt(int64(hash), 10)
}

// boolString leaves a value that was never measured empty. Writing "false" for
// an unmeasured field is an assertion nothing checked — a --only-passive run
// would claim every name is not a wildcard, when no wildcard check was made.
func boolString(v *bool) string {
	if v == nil {
		return ""
	}
	return strconv.FormatBool(*v)
}
