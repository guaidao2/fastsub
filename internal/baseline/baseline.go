// Package baseline remembers what an earlier run found, so a later run reports
// only what is new. That comparison is the point of running an enumeration on a
// schedule: the useful output of Monday's run is the three names Tuesday did
// not have, not the twelve hundred it did.
//
// There is no database here on purpose. A snapshot is a file the previous run
// wrote, and this package reads it back.
package baseline

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Set is the host names an earlier run reported.
type Set map[string]bool

// Load reads a snapshot. The format is detected from the content rather than
// the file name, because the snapshot is whichever file the user points at:
// fastsub's own JSONL, a JSON document, a CSV export, or a plain host list.
func Load(path string) (Set, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	set := make(Set)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "{"), strings.HasPrefix(line, "["):
			// One JSONL record, or the opening line of a JSON document. Every
			// host field anywhere in it is a name the earlier run reported.
			for _, m := range hostFieldRE.FindAllStringSubmatch(line, -1) {
				set.add(m[1])
			}

		case isHeader(line):
			continue

		default:
			// A bare list, or a CSV row, or a verbose text line: the name is
			// the first field of it.
			field := line
			if i := strings.IndexByte(line, ','); i >= 0 {
				field = line[:i]
			}
			if fields := strings.Fields(field); len(fields) > 0 {
				set.add(fields[0])
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return set, nil
}

// hostFieldRE matches the host field of a JSON record. Parsing the document
// properly would need to know which of the four shapes it is in; the field name
// is fixed by the schema, so looking for it is both simpler and more tolerant.
var hostFieldRE = regexp.MustCompile(`"host"\s*:\s*"([^"]+)"`)

// isHeader recognises the CSV header row, which names columns rather than a
// host and would otherwise be read as one.
func isHeader(line string) bool {
	return strings.HasPrefix(strings.ToLower(line), "host,") ||
		strings.HasPrefix(strings.ToLower(line), "\"host\",")
}

func (s Set) add(host string) {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return
	}
	s[host] = true
}

// Has reports whether the earlier run already knew this host.
func (s Set) Has(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return s[host]
}

// Len is the number of names in the snapshot.
func (s Set) Len() int { return len(s) }
