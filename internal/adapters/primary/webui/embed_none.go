//go:build !webui

package webui

import "io/fs"

// files is nil in a build without the webui tag, which serves a page that
// says how to build one with the interface (ADR-0035 §1).
func files() fs.FS { return nil }
