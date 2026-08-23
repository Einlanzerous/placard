// Package version carries the build identity stamped by deploy/Dockerfile via
// -ldflags. Both default to empty: an image built outside the publish workflow
// must not be able to claim it is a release, and empty maps to "dev" rather
// than a placeholder that could be mistaken for a version (the PRSR-32 rule).
package version

var (
	// Version is bare semver ("0.1.0"), no "v" prefix — compared with strict
	// equality against org.opencontainers.image.version, which is stamped bare.
	Version = ""
	// Commit is the full 40-char commit sha, reported verbatim.
	Commit = ""
)

// Resolved maps the empty build-arg case to "dev".
func Resolved() string {
	if Version == "" {
		return "dev"
	}
	return Version
}
