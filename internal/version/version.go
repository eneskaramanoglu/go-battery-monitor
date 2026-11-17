// Package version holds build metadata, stamped by build.ps1 / CI through -ldflags -X.
package version

import "runtime/debug"

// Version is the release tag ("v0.1.0"); "dev" for local builds.
var Version = "dev"

// Commit is the short git hash the binary was built from.
var Commit = ""

// String returns "v0.1.0 (abc1234)", falling back to the VCS info Go embeds itself.
func String() string {
	c := Commit
	if c == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			}
		}
	}
	if c == "" {
		return Version
	}
	return Version + " (" + c + ")"
}
