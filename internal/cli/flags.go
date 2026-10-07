package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/guaidao2/fastsub/internal/i18n"
)

// flagKind is how an option's value is read and validated.
type flagKind int

const (
	kindBool flagKind = iota
	kindString
	kindStrings
	kindInt
	kindDuration
)

// flagDef is one option. The same struct drives parsing, help output and the
// bilingual catalogue key (flag.<long>), so an option cannot exist without
// documented help in every language.
type flagDef struct {
	long    string   // "domain"
	short   string   // "d", without the dash; empty when there is none
	aliases []string // extra accepted spellings, e.g. "oJ" for --output-json
	kind    flagKind
	meta    string // value placeholder in help, e.g. "FILE"
	group   string // help section slug
	values  []string
	set     bool
}

// Alias adds extra accepted short spellings. argus spells its outputs -oN, -oJ
// and -oA, and fastsub keeps that so muscle memory carries over.
func (d *flagDef) Alias(names ...string) *flagDef {
	d.aliases = append(d.aliases, names...)
	return d
}

// FlagSet parses a command line. Options may be written as --long value,
// --long=value, -s value, -s=value or -svalue; a single "-" is a positional
// argument, because that is how stdin is named.
type FlagSet struct {
	Lang    i18n.Lang
	defs    []*flagDef
	byLong  map[string]*flagDef
	byShort map[string]*flagDef
	pos     []string
}

// NewFlagSet returns an empty set that renders its errors in lang.
func NewFlagSet(lang i18n.Lang) *FlagSet {
	return &FlagSet{
		Lang:    lang,
		byLong:  make(map[string]*flagDef),
		byShort: make(map[string]*flagDef),
	}
}

func (fs *FlagSet) add(d *flagDef) *flagDef {
	fs.defs = append(fs.defs, d)
	fs.byLong[d.long] = d
	if d.short != "" {
		fs.byShort[d.short] = d
	}
	for _, a := range d.aliases {
		fs.byShort[a] = d
	}
	return d
}

// Bool declares a switch.
func (fs *FlagSet) Bool(long, short, group string) *flagDef {
	return fs.add(&flagDef{long: long, short: short, kind: kindBool, group: group})
}

// String declares an option taking one value; the last one wins.
func (fs *FlagSet) String(long, short, meta, group string) *flagDef {
	return fs.add(&flagDef{long: long, short: short, kind: kindString, meta: meta, group: group})
}

// Strings declares an option that accumulates every value it is given.
func (fs *FlagSet) Strings(long, short, meta, group string) *flagDef {
	return fs.add(&flagDef{long: long, short: short, kind: kindStrings, meta: meta, group: group})
}

// Int declares a whole-number option.
func (fs *FlagSet) Int(long, short, meta, group string) *flagDef {
	return fs.add(&flagDef{long: long, short: short, kind: kindInt, meta: meta, group: group})
}

// Duration declares an option such as 5s or 500ms.
func (fs *FlagSet) Duration(long, short, meta, group string) *flagDef {
	return fs.add(&flagDef{long: long, short: short, kind: kindDuration, meta: meta, group: group})
}

// Parse walks the command line.
func (fs *FlagSet) Parse(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			fs.pos = append(fs.pos, args[i+1:]...)
			return nil

		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := splitLong(arg)
			d, ok := fs.byLong[name]
			if !ok {
				return fs.errf("error.unknown_flag", "--"+name)
			}
			if d.kind == kindBool {
				if !hasValue {
					d.setTrue()
					continue
				}
				b, err := strconv.ParseBool(value)
				if err != nil {
					return fs.errf("error.bad_bool", "--"+name, value)
				}
				d.setBool(b)
				continue
			}
			if !hasValue {
				if i+1 >= len(args) {
					return fs.errf("error.missing_value", "--"+name)
				}
				i++
				value = args[i]
			}
			if err := d.assign(value, fs.Lang); err != nil {
				return err
			}

		case strings.HasPrefix(arg, "-") && arg != "-":
			if err := fs.parseShort(arg, args, &i); err != nil {
				return err
			}

		default:
			fs.pos = append(fs.pos, arg)
		}
	}
	return nil
}

// parseShort reads a dash option. The longest registered spelling wins, so -oJ
// is --output-json rather than -o with the value "J".
func (fs *FlagSet) parseShort(arg string, args []string, i *int) error {
	body := arg[1:]
	key, d := fs.longestMatch(body)
	if d == nil {
		return fs.errf("error.unknown_flag", "--"+body)
	}
	rest := body[len(key):]

	if d.kind == kindBool {
		d.setTrue()
		if rest == "" {
			return nil
		}
		// A cluster such as -vv keeps going, which is what -vv means.
		return fs.parseShort("-"+rest, args, i)
	}

	if rest != "" {
		return d.assign(strings.TrimPrefix(rest, "="), fs.Lang)
	}
	if *i+1 >= len(args) {
		return fs.errf("error.missing_value", "-"+key)
	}
	*i++
	return d.assign(args[*i], fs.Lang)
}

func (fs *FlagSet) longestMatch(body string) (string, *flagDef) {
	var best string
	var bestDef *flagDef
	for k, d := range fs.byShort {
		if strings.HasPrefix(body, k) && len(k) > len(best) {
			best, bestDef = k, d
		}
	}
	return best, bestDef
}

func (fs *FlagSet) errf(key string, args ...any) error {
	return fmt.Errorf("%s", i18n.T(fs.Lang, key, args...))
}

// splitLong breaks --name=value into its parts.
func splitLong(arg string) (name, value string, hasValue bool) {
	body := arg[2:]
	if i := strings.IndexByte(body, '='); i >= 0 {
		return body[:i], body[i+1:], true
	}
	return body, "", false
}

func (d *flagDef) setTrue() {
	d.set = true
	d.values = append(d.values, "true")
}

func (d *flagDef) setBool(b bool) {
	d.set = true
	if b {
		d.values = append(d.values, "true")
		return
	}
	d.values = append(d.values, "false")
}

func (d *flagDef) assign(value string, lang i18n.Lang) error {
	switch d.kind {
	case kindString, kindStrings:
	case kindInt:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("%s", i18n.T(lang, "error.bad_int", "--"+d.long, value))
		}
	case kindDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%s", i18n.T(lang, "error.bad_duration", "--"+d.long, value))
		}
	}
	d.set = true
	d.values = append(d.values, value)
	return nil
}

// BoolValue reports a switch. An option given more than once keeps its last
// value, which is what --no-color=false after --no-color should do.
func (fs *FlagSet) BoolValue(long string) bool {
	d, ok := fs.byLong[long]
	if !ok || len(d.values) == 0 {
		return false
	}
	b, err := strconv.ParseBool(d.values[len(d.values)-1])
	return err == nil && b
}

// StringValue returns the last value given to an option.
func (fs *FlagSet) StringValue(long string) string {
	d, ok := fs.byLong[long]
	if !ok || len(d.values) == 0 {
		return ""
	}
	return d.values[len(d.values)-1]
}

// StringsValue returns every value given to an option, in order.
func (fs *FlagSet) StringsValue(long string) []string {
	d, ok := fs.byLong[long]
	if !ok {
		return nil
	}
	return d.values
}

// IntValue returns the last whole number given to an option, or 0.
func (fs *FlagSet) IntValue(long string) int {
	v := fs.StringValue(long)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// DurationValue returns the last duration given to an option, or 0.
func (fs *FlagSet) DurationValue(long string) time.Duration {
	v := fs.StringValue(long)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0
	}
	return d
}

// IsSet reports whether the option appeared on the command line at all, which
// is not the same as carrying a true value.
func (fs *FlagSet) IsSet(long string) bool {
	d, ok := fs.byLong[long]
	return ok && d.set
}

// Positional returns the arguments that were not options.
func (fs *FlagSet) Positional() []string {
	return fs.pos
}
