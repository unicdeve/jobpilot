// Command gennotices writes THIRD-PARTY-NOTICES.txt: the license of every
// third-party component a release compiles into JobPilot, with where its
// source is published. The MIT, BSD and Apache licenses most of them use
// ask for their notices to travel with a copy, and the MPL for its source
// to be findable, so every CLI archive and desktop bundle carries the file
// (docs/08_development/packaging.md).
//
// What ships is read from each build's own tooling rather than from what
// is installed: `go list -deps` over the daemon and the CLI for every
// target `make dist` builds, `cargo metadata` over the desktop crate for
// every target `make bundle` builds, and the npm packages the desktop's
// Vite build bundled (desktop/bundled-packages.json). The licenses no
// package manager carries, Go's, Rust's and PDFium's, are in curated/.
//
// It is run by `make notices`, from the repository's root, and CI's `make
// notices-check` fails when the committed file is out of date.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"strings"
)

// versionFile records, in a curated directory, the version its licenses
// were taken for.
const versionFile = "VERSION"

func main() {
	out := flag.String("out", "THIRD-PARTY-NOTICES.txt", "the file to write")
	curatedDir := flag.String("curated", "internal/tools/gennotices/curated", "the licenses no package manager carries")
	goTargets := flag.String("go-targets", "", "the GOOS/GOARCH pairs the daemon and CLI are released for, space-separated")
	rustTargets := flag.String("rust-targets", "", "the Rust targets the desktop is released for, space-separated")
	refresh := flag.Bool("refresh-rust-std", false, "write curated/rust again from the pinned Rust toolchain first")
	flag.Parse()
	log.SetFlags(0)

	if *refresh {
		if err := refreshRustStd(*curatedDir); err != nil {
			log.Fatalf("gennotices: %v", err)
		}
	}
	if err := run(os.DirFS(*curatedDir), *out, strings.Fields(*goTargets), strings.Fields(*rustTargets)); err != nil {
		log.Fatalf("gennotices: %v", err)
	}
}

func run(curated fs.FS, out string, goTargets, rustTargets []string) error {
	if len(goTargets) == 0 || len(rustTargets) == 0 {
		return fmt.Errorf("both -go-targets and -rust-targets are needed")
	}
	goParts, err := goComponents(curated, goTargets)
	if err != nil {
		return err
	}
	rustParts, err := rustComponents(curated, rustTargets)
	if err != nil {
		return err
	}
	npmParts, err := npmComponents(curated)
	if err != nil {
		return err
	}
	components := append(append(goParts, rustParts...), npmParts...)
	if err := os.WriteFile(out, []byte(render(components)), 0o644); err != nil {
		return err
	}
	log.Printf("gennotices: %d components (%d Go, %d Rust, %d npm) in %s",
		len(components), len(goParts), len(rustParts), len(npmParts), out)
	return nil
}

// curatedVersion is the version a curated directory's licenses were taken
// for.
func curatedVersion(curated fs.FS, dir string) (string, error) {
	body, err := fs.ReadFile(curated, path.Join(dir, versionFile))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
