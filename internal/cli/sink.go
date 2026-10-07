package cli

import (
	"io"
	"os"

	"github.com/guaidao2/fastsub/internal/model"
	"github.com/guaidao2/fastsub/internal/output"
)

// sink fans one confirmed host out to every destination the user asked for:
// stdout always, plus one printer per -o, -oJ, -oN and -oA. Results still go
// only to stdout and the named files; progress stays on stderr.
type sink struct {
	printers []*output.Printer
	files    []io.Closer
}

func (s *sink) Host(h model.Host) error {
	for _, p := range s.printers {
		if err := p.Host(h); err != nil {
			return err
		}
	}
	return nil
}

// Close finishes every printer and reports the first failure, because a broken
// pipe or a full disk must not look like a clean run.
func (s *sink) Close(sum model.Summary) error {
	var firstErr error
	for _, p := range s.printers {
		if err := p.Close(sum); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *sink) cleanup() {
	for _, c := range s.files {
		_ = c.Close()
	}
}

// buildSink collects the destinations. -o follows --format, -oJ is JSONL, -oN
// is plain text, and -oA is all three at once under one base name — the same
// spellings argus uses, so a script ported from it keeps working.
func buildSink(fs *FlagSet, format output.Format, stdout io.Writer) (*sink, error) {
	verbose := fs.BoolValue("verbose")
	s := &sink{
		printers: []*output.Printer{
			output.New(stdout, output.Options{Format: format, Verbose: verbose}),
		},
	}

	add := func(path string, f output.Format) error {
		fh, err := os.Create(path)
		if err != nil {
			return err
		}
		s.files = append(s.files, fh)
		s.printers = append(s.printers, output.New(fh, output.Options{Format: f, Verbose: verbose}))
		return nil
	}

	if path := fs.StringValue("output"); path != "" {
		if err := add(path, format); err != nil {
			return nil, err
		}
	}
	if path := fs.StringValue("output-json"); path != "" {
		if err := add(path, output.JSONL); err != nil {
			return nil, err
		}
	}
	if path := fs.StringValue("output-normal"); path != "" {
		if err := add(path, output.Text); err != nil {
			return nil, err
		}
	}
	if base := fs.StringValue("output-all"); base != "" {
		for _, target := range []struct {
			suffix string
			format output.Format
		}{
			{".txt", output.Text},
			{".jsonl", output.JSONL},
			{".csv", output.CSV},
		} {
			if err := add(base+target.suffix, target.format); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}
