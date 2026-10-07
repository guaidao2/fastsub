package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/guaidao2/fastsub/internal/i18n"
	"github.com/guaidao2/fastsub/internal/version"
)

// groupOrder is the order help prints its sections in.
var groupOrder = []string{"target", "sources", "enum", "resolve", "verify", "output", "misc"}

type helpLine struct {
	left string
	text string
}

// renderHelp prints the bilingual help. The layout follows argus: sections in
// capitals, the option column padded so the descriptions line up.
func renderHelp(w io.Writer, fs *FlagSet, lang i18n.Lang) {
	fmt.Fprintln(w, version.Line())
	fmt.Fprintln(w, i18n.T(lang, "app.tagline"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T(lang, "help.usage", version.Name))
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T(lang, "app.intro"))
	fmt.Fprintln(w)

	groups := make(map[string][]helpLine, len(groupOrder))
	for _, d := range fs.defs {
		groups[d.group] = append(groups[d.group], helpLine{
			left: leftText(d),
			text: i18n.T(lang, "flag."+d.long),
		})
	}

	// The targets are positional, and help would be lying if it only showed -d.
	target := append([]helpLine{{
		left: "<domain ...>",
		text: i18n.T(lang, "help.positional_args"),
	}}, groups["target"]...)
	groups["target"] = target

	width := 0
	for _, slug := range groupOrder {
		for _, l := range groups[slug] {
			if n := len(l.left); n > width {
				width = n
			}
		}
	}

	for _, slug := range groupOrder {
		lines := groups[slug]
		if len(lines) == 0 {
			continue
		}
		fmt.Fprintln(w, i18n.T(lang, "group."+slug))
		for _, l := range lines {
			fmt.Fprintf(w, "  %-*s  %s\n", width, l.left, l.text)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, i18n.T(lang, "group.examples"))
	fmt.Fprintln(w, i18n.T(lang, "help.examples"))
	fmt.Fprintln(w)

	fmt.Fprintln(w, i18n.T(lang, "help.available_langs", i18n.Listing()))
	fmt.Fprintln(w, i18n.T(lang, "help.author", version.Authors))
	fmt.Fprintln(w, i18n.T(lang, "help.license", version.License))
	fmt.Fprintln(w, wrapText(i18n.T(lang, "help.legal"), 78, "  "))
}

// leftText builds the option column: "-d, --domain DOMAIN", "--input-list FILE"
// or "    --quiet" so a missing short name still lines up.
func leftText(d *flagDef) string {
	var b strings.Builder
	if d.short != "" {
		b.WriteString("-" + d.short + ", ")
	} else {
		b.WriteString("    ")
	}
	b.WriteString("--" + d.long)
	if d.meta != "" {
		b.WriteString(" " + d.meta)
	}
	return b.String()
}

// wrapText wraps on spaces, keeping the indent on every line but the first.
// Widths are display widths, not byte counts: a Chinese sentence counted in
// bytes breaks a line roughly twice as early as it should.
func wrapText(s string, width int, indent string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var out strings.Builder
	out.WriteString(words[0])
	line := displayWidth(words[0])
	for _, word := range words[1:] {
		w := displayWidth(word)
		if line+1+w > width {
			out.WriteString("\n" + indent + word)
			line = displayWidth(indent) + w
			continue
		}
		out.WriteString(" " + word)
		line += 1 + w
	}
	return out.String()
}

// displayWidth counts a character outside ASCII as two columns, which is how a
// terminal renders CJK text.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > 127 {
			w += 2
			continue
		}
		w++
	}
	return w
}
