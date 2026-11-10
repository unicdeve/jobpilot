//go:build webui

package webui

import (
	"embed"
	"io/fs"
)

// dist is desktop/dist, copied here by `make web` before a build with the
// webui tag (ADR-0035 §1).
//
//go:embed all:dist
var dist embed.FS

func files() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
