// Package i18n holds every user-facing string in one place, and resolves the
// output language the way the rest of the toolchain does: English unless a
// language is asked for explicitly, and never because of the host locale.
package i18n

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Lang is an output language code.
type Lang string

const (
	// EN is English, the default.
	EN Lang = "en"
	// ZH is Simplified Chinese, always an explicit choice.
	ZH Lang = "zh"
)

// EnvVar selects a language when --lang is absent.
const EnvVar = "FASTSUB_LANG"

var catalogues = map[Lang]map[string]string{
	EN: en,
	ZH: zh,
}

// displayNames is what --list-langs prints next to each code. It is written in
// the language itself, so a reader who cannot read the current one can still
// find theirs.
var displayNames = map[Lang]string{
	EN: "English",
	ZH: "简体中文",
}

// T renders a message and substitutes args through fmt.Sprintf.
func T(lang Lang, key string, args ...any) string {
	msg := lookup(lang, key)
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// lookup finds a key in lang, falling back to English and finally to the key
// itself, so a missing translation is visible rather than silently blank.
func lookup(lang Lang, key string) string {
	if c, ok := catalogues[lang]; ok {
		if s, ok := c[key]; ok {
			return s
		}
	}
	if s, ok := en[key]; ok {
		return s
	}
	return key
}

// Parse turns a language code into a Lang.
func Parse(code string) (Lang, bool) {
	l := Lang(strings.ToLower(strings.TrimSpace(code)))
	if _, ok := catalogues[l]; ok {
		return l, true
	}
	return "", false
}

// Resolve applies the documented order: --lang, then FASTSUB_LANG, then English.
// An unreadable value at either step is ignored rather than fatal, because a
// stray environment variable should not stop a scan.
func Resolve(flagValue string) Lang {
	if l, ok := Parse(flagValue); ok {
		return l
	}
	if env := os.Getenv(EnvVar); env != "" {
		if l, ok := Parse(env); ok {
			return l
		}
	}
	return EN
}

// Codes lists the available language codes in a stable order.
func Codes() []Lang {
	out := make([]Lang, 0, len(catalogues))
	for l := range catalogues {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// DisplayName is the language's own name, for --list-langs.
func (l Lang) DisplayName() string {
	if n, ok := displayNames[l]; ok {
		return n
	}
	return string(l)
}

// Listing renders "en (English), zh (简体中文)" for --list-langs and for the
// unknown-language error.
func Listing() string {
	parts := make([]string, 0, len(catalogues))
	for _, l := range Codes() {
		parts = append(parts, fmt.Sprintf("%s (%s)", l, l.DisplayName()))
	}
	return strings.Join(parts, ", ")
}
