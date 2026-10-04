package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// goBinaries are what every release builds from Go: the daemon and the CLI.
var goBinaries = []string{"./cmd/jobpilot", "./cmd/jobpilot-cli"}

// pdfiumModule embeds PDFium, compiled to WebAssembly, whose licenses and
// those of the libraries compiled into it are in curated/pdfium, taken for
// the release of PDFium this version of the module embeds (curated/README.md).
const pdfiumModule = "github.com/klippa-app/go-pdfium"

type goModule struct {
	Path    string
	Version string
	Dir     string
	Main    bool
	Replace *goModule
}

type goPackage struct {
	Module *goModule
}

// goComponents is every module linked into the daemon or the CLI for any
// of targets, which are GOOS/GOARCH pairs, with Go's own standard library
// and runtime first, and PDFium's libraries where go-pdfium is among them.
func goComponents(curated fs.FS, targets []string) ([]component, error) {
	modules := map[string]goModule{}
	for _, target := range targets {
		goos, goarch, ok := strings.Cut(target, "/")
		if !ok {
			return nil, fmt.Errorf("go target %q is not GOOS/GOARCH", target)
		}
		linked, err := linkedModules(goos, goarch)
		if err != nil {
			return nil, fmt.Errorf("listing what %s links: %w", target, err)
		}
		for _, module := range linked {
			modules[module.Path] = module
		}
	}

	goVersion, err := goModVersion()
	if err != nil {
		return nil, err
	}
	std, err := curatedTexts(curated, "go")
	if err != nil {
		return nil, err
	}
	components := []component{{
		Group:   groupGoStd,
		Name:    "Go standard library and runtime",
		Version: "go" + goVersion,
		URL:     "https://go.dev/",
		Texts:   std,
	}}

	paths := make([]string, 0, len(modules))
	for path := range modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		module := modules[path]
		texts, err := licenseTexts(module.Dir)
		if err != nil {
			return nil, fmt.Errorf("reading %s's license: %w", path, err)
		}
		if len(texts) == 0 {
			return nil, fmt.Errorf("%s@%s ships no license file at %s", path, module.Version, module.Dir)
		}
		components = append(components, component{
			Group:   groupGoModules,
			Name:    path,
			Version: module.Version,
			URL:     "https://pkg.go.dev/" + path + "@" + module.Version,
			Texts:   texts,
		})
		if path == pdfiumModule {
			pdfium, err := pdfiumComponent(curated, module.Version)
			if err != nil {
				return nil, err
			}
			components = append(components, pdfium)
		}
	}
	return components, nil
}

// pdfiumComponent is PDFium and what it compiles in, from curated/pdfium,
// which is refused once go-pdfium is a version it was not taken for: a new
// one can embed a different PDFium, with different libraries in it.
func pdfiumComponent(curated fs.FS, moduleVersion string) (component, error) {
	recorded, err := curatedVersion(curated, "pdfium")
	if err != nil {
		return component{}, err
	}
	forModule, pdfiumVersion, ok := strings.Cut(recorded, " ")
	if !ok || forModule != moduleVersion {
		return component{}, fmt.Errorf("%s is %s, and curated/pdfium holds the licenses of the PDFium %s embeds (%q); "+
			"take them again for the new version (internal/tools/gennotices/curated/README.md)", pdfiumModule, moduleVersion, forModule, recorded)
	}
	texts, err := curatedTexts(curated, "pdfium")
	if err != nil {
		return component{}, err
	}
	return component{
		Group:   groupPDFium,
		Name:    "PDFium, and the libraries compiled into it",
		Version: pdfiumVersion,
		URL:     "https://pdfium.googlesource.com/pdfium/",
		Texts:   texts,
	}, nil
}

// linkedModules runs `go list -deps` over the binaries as a release builds
// them for goos/goarch, and returns every module other than this one that
// a linked package belongs to. A module replaced in go.mod is read where
// it is replaced from.
func linkedModules(goos, goarch string) ([]goModule, error) {
	args := append([]string{"list", "-deps", "-json=Module"}, goBinaries...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=-mod=readonly")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return decodeLinkedModules(bytes.NewReader(out))
}

// decodeLinkedModules reads the stream of packages `go list -json` prints.
func decodeLinkedModules(r io.Reader) ([]goModule, error) {
	decoder := json.NewDecoder(r)
	seen := map[string]bool{}
	var modules []goModule
	for {
		var pkg goPackage
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			return modules, nil
		}
		if err != nil {
			return nil, err
		}
		module := pkg.Module
		if module == nil || module.Main || seen[module.Path] {
			continue
		}
		seen[module.Path] = true
		if module.Replace != nil {
			module.Dir = module.Replace.Dir
		}
		modules = append(modules, *module)
	}
}

// goModVersion is the Go version go.mod asks for, which the release
// workflow builds with: the same on every machine the notices are
// generated on, whatever Go each has installed.
func goModVersion() (string, error) {
	file, err := os.Open("go.mod")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if version, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "go "); ok {
			return version, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("go.mod has no go directive")
}
