package i18n

import (
	"regexp"
	"sort"
	"testing"
)

// verbRE matches the printf verbs Go's fmt understands.
var verbRE = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

func verbs(s string) []string {
	all := verbRE.FindAllString(s, -1)
	out := make([]string, 0, len(all))
	for _, v := range all {
		if v == "%%" {
			continue
		}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// TestCatalogueCoverage fails when a language is missing a key that English
// defines, or defines one English does not. A translation nobody can reach is
// as much of a bug as a missing one.
func TestCatalogueCoverage(t *testing.T) {
	for _, lang := range Codes() {
		if lang == EN {
			continue
		}
		cat := catalogues[lang]

		var missing []string
		for key := range en {
			if _, ok := cat[key]; !ok {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s is missing %d key(s): %v", lang, len(missing), missing)
		}

		var extra []string
		for key := range cat {
			if _, ok := en[key]; !ok {
				extra = append(extra, key)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			t.Errorf("%s defines %d key(s) English does not have: %v", lang, len(extra), extra)
		}
	}
}

// TestPlaceholdersMatch fails when a translation's printf verbs drift away from
// the English original, which is how a translated message ends up printing a
// literal %!d(MISSING) at a user.
func TestPlaceholdersMatch(t *testing.T) {
	for _, lang := range Codes() {
		if lang == EN {
			continue
		}
		for key, want := range en {
			got, ok := catalogues[lang][key]
			if !ok {
				continue // coverage test already reports this
			}
			a, b := verbs(want), verbs(got)
			if len(a) != len(b) {
				t.Errorf("%s: %q has verbs %v, English has %v", lang, key, b, a)
				continue
			}
			for i := range a {
				if a[i] != b[i] {
					t.Errorf("%s: %q has verbs %v, English has %v", lang, key, b, a)
					break
				}
			}
		}
	}
}

func TestResolveOrder(t *testing.T) {
	t.Setenv(EnvVar, "zh")

	if got := Resolve("en"); got != EN {
		t.Errorf("--lang should win over the environment, got %q", got)
	}
	if got := Resolve(""); got != ZH {
		t.Errorf("environment should be used when --lang is absent, got %q", got)
	}

	t.Setenv(EnvVar, "klingon")
	if got := Resolve(""); got != EN {
		t.Errorf("an unreadable environment value should fall back to English, got %q", got)
	}
}

func TestParseRejectsUnknown(t *testing.T) {
	if _, ok := Parse("de"); ok {
		t.Error("German is not in the catalogue yet and should not parse")
	}
	if l, ok := Parse("ZH"); !ok || l != ZH {
		t.Errorf("language codes should be case-insensitive, got %q %v", l, ok)
	}
}

func TestUnknownKeyFallsBackToEnglish(t *testing.T) {
	if got := T(ZH, "flag.domain"); got == "flag.domain" {
		t.Error("a known key should never fall through to itself")
	}
	if got := T(ZH, "no.such.key"); got != "no.such.key" {
		t.Errorf("an unknown key should be visible as itself, got %q", got)
	}
}
