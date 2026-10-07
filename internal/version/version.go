// Package version carries the identity of the build: name, version and
// authorship. Every user-facing surface takes these values from here so the
// name is written down exactly once.
package version

import (
	"fmt"
	"runtime"
)

const (
	// Name is the binary and command name.
	Name = "fastsub"
	// Repo is the canonical repository.
	Repo = "https://github.com/guaidao2/fastsub"
	// Authors is the authorship line, shared with crackweb and argus.
	Authors = "guaidao2 & coolmoon"
	// License is the license fastsub is distributed under.
	License = "MIT"
)

// Version is the release version. It is a variable rather than a constant so a
// release build can set it with -ldflags "-X .../internal/version.Version=v1.0.0".
var Version = "1.0.0"

// Line is the one-line identity printed at the top of help output and by
// --version.
func Line() string {
	return fmt.Sprintf("%s v%s (by %s)", Name, Version, Authors)
}

// Long is the multi-line form printed by --version.
func Long() string {
	return fmt.Sprintf(`%s v%s
%s
%s
%s
built with %s
%s`, Name, Version, Repo, Authors, License, runtime.Version(), "use only against systems you own or have explicit written authorization to test")
}
