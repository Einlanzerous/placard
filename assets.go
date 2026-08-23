// Package placard embeds the asset store — the service dirs and the manifest —
// so the binary is self-contained (construct-server house style: one static
// binary, nothing read from the deploy filesystem).
//
// embed directives are static: a NEW service directory must be added to the
// list below or the binary serves 404s for marks that exist in the repo.
// catalog's tests compare the embedded tree against services.json, so
// forgetting fails `go test`, not production.
package placard

import "embed"

//go:embed services.json
//go:embed all:argosy all:switchyard all:lyceum all:placard
var Assets embed.FS
