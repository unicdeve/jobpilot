package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// desktopCrate is the desktop app's Rust crate.
const desktopCrate = "desktop/src-tauri"

type cargoMetadata struct {
	Packages []cargoPackage `json:"packages"`
	Resolve  struct {
		Root  string      `json:"root"`
		Nodes []cargoNode `json:"nodes"`
	} `json:"resolve"`
}

type cargoPackage struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	License      string        `json:"license"`
	LicenseFile  string        `json:"license_file"`
	Authors      []string      `json:"authors"`
	ManifestPath string        `json:"manifest_path"`
	Targets      []cargoTarget `json:"targets"`
}

type cargoTarget struct {
	Kind []string `json:"kind"`
}

type cargoNode struct {
	ID   string     `json:"id"`
	Deps []cargoDep `json:"deps"`
}

type cargoDep struct {
	Pkg      string         `json:"pkg"`
	DepKinds []cargoDepKind `json:"dep_kinds"`
}

type cargoDepKind struct {
	// Kind is null for a normal dependency, and "dev" or "build" for one
	// only tests or a build script use.
	Kind *string `json:"kind"`
}

// rustComponents is Rust's standard library and every crate linked into the
// desktop app for any of targets.
func rustComponents(curated fs.FS, targets []string) ([]component, error) {
	crates := map[string]cargoPackage{}
	for _, target := range targets {
		metadata, err := readCargoMetadata(target)
		if err != nil {
			return nil, fmt.Errorf("reading the desktop's crates for %s: %w", target, err)
		}
		for _, pkg := range linkedCrates(metadata) {
			crates[pkg.Name+"@"+pkg.Version] = pkg
		}
	}

	components, err := rustStdComponents(curated)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(crates))
	for key := range crates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		pkg := crates[key]
		dir := filepath.Dir(pkg.ManifestPath)
		texts, err := licenseTexts(dir)
		if err != nil {
			return nil, fmt.Errorf("reading %s's license: %w", key, err)
		}
		if pkg.LicenseFile != "" && len(texts) == 0 {
			body, err := os.ReadFile(filepath.Join(dir, pkg.LicenseFile))
			if err != nil {
				return nil, fmt.Errorf("reading %s's license file: %w", key, err)
			}
			texts = []text{{Name: filepath.Base(pkg.LicenseFile), Body: normalize(string(body))}}
		}
		if len(texts) == 0 {
			texts, err = declaredLicense(curated, pkg.License, pkg.Name, pkg.Authors)
			if err != nil {
				return nil, err
			}
		}
		components = append(components, component{
			Group:   groupRustCrates,
			Name:    pkg.Name,
			Version: pkg.Version,
			License: pkg.License,
			URL:     "https://crates.io/crates/" + pkg.Name + "/" + pkg.Version,
			Texts:   texts,
		})
	}
	return components, nil
}

// readCargoMetadata is the desktop crate's dependency graph as it is built
// for target, from Cargo.lock as committed.
func readCargoMetadata(target string) (cargoMetadata, error) {
	cmd := exec.Command("cargo", "metadata", "--format-version", "1", "--locked", "--filter-platform", target)
	cmd.Dir = desktopCrate
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return cargoMetadata{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var metadata cargoMetadata
	err = json.Unmarshal(out, &metadata)
	return metadata, err
}

// linkedCrates walks the graph from the desktop crate along normal
// dependencies, which are the ones linked into the app. A dev or build
// dependency, and a procedural macro, runs while the app is built and is
// not part of what ships.
func linkedCrates(metadata cargoMetadata) []cargoPackage {
	packages := map[string]cargoPackage{}
	for _, pkg := range metadata.Packages {
		packages[pkg.ID] = pkg
	}
	nodes := map[string]cargoNode{}
	for _, node := range metadata.Resolve.Nodes {
		nodes[node.ID] = node
	}

	root := metadata.Resolve.Root
	seen := map[string]bool{root: true}
	queue := []string{root}
	var linked []cargoPackage
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, dep := range nodes[id].Deps {
			if seen[dep.Pkg] || !isNormal(dep.DepKinds) {
				continue
			}
			pkg := packages[dep.Pkg]
			if isProcMacro(pkg) {
				continue
			}
			seen[dep.Pkg] = true
			linked = append(linked, pkg)
			queue = append(queue, dep.Pkg)
		}
	}
	return linked
}

func isNormal(kinds []cargoDepKind) bool {
	for _, kind := range kinds {
		if kind.Kind == nil {
			return true
		}
	}
	return false
}

func isProcMacro(pkg cargoPackage) bool {
	for _, target := range pkg.Targets {
		if slices.Contains(target.Kind, "proc-macro") {
			return true
		}
	}
	return false
}

// stdCrates is curated/rust/crates.json: the crates Rust's standard
// library is built from, with each distinct notice text stored once, since
// most crates quote word-for-word the same Apache and MIT texts.
type stdCrates struct {
	Texts  []string   `json:"texts"`
	Crates []stdCrate `json:"crates"`
}

// stdCrate is one of the crates Rust's standard library is built from.
type stdCrate struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	License string   `json:"license"`
	Authors []string `json:"authors,omitempty"`
	URL     string   `json:"url"`
	// Notices are the files the Rust project quotes for the crate. A few
	// it lists by license alone, which is then printed from curated/spdx.
	Notices []stdNotice `json:"notices,omitempty"`
}

// stdNotice is one quoted file, its text an index into stdCrates.Texts.
type stdNotice struct {
	File string `json:"file"`
	Text int    `json:"text"`
}

// rustStdComponents is Rust's standard library, which is linked into every
// Rust program, and the crates it is built from, with the copyright
// notices the Rust project publishes for them (curated/rust). They are
// refused once the toolchain desktop/src-tauri/rust-toolchain.toml pins is
// not the one they were taken from.
func rustStdComponents(curated fs.FS) ([]component, error) {
	pinned, err := pinnedRustToolchain()
	if err != nil {
		return nil, err
	}
	recorded, err := curatedVersion(curated, "rust")
	if err != nil {
		return nil, err
	}
	if recorded != pinned {
		return nil, fmt.Errorf("rust-toolchain.toml pins Rust %s, and curated/rust holds the standard library's notices for %s; "+
			"run `make notices REFRESH_RUST_STD=1` with the new toolchain installed", pinned, recorded)
	}
	std, err := fs.ReadFile(curated, "rust/std.md")
	if err != nil {
		return nil, err
	}
	components := []component{{
		Group:   groupRustStd,
		Name:    "Rust standard library",
		Version: pinned,
		License: "Apache-2.0 OR MIT",
		URL:     "https://github.com/rust-lang/rust",
		Texts:   []text{{Name: "COPYRIGHT-library", Body: normalize(string(std))}},
	}}

	body, err := fs.ReadFile(curated, "rust/crates.json")
	if err != nil {
		return nil, err
	}
	var listed stdCrates
	if err := json.Unmarshal(body, &listed); err != nil {
		return nil, fmt.Errorf("reading curated/rust/crates.json: %w", err)
	}
	for _, crate := range listed.Crates {
		var texts []text
		for _, notice := range crate.Notices {
			if notice.Text < 0 || notice.Text >= len(listed.Texts) {
				return nil, fmt.Errorf("curated/rust/crates.json: %s's %s names text %d of %d", crate.Name, notice.File, notice.Text, len(listed.Texts))
			}
			texts = append(texts, text{Name: notice.File, Body: listed.Texts[notice.Text]})
		}
		if len(texts) == 0 {
			texts, err = declaredLicense(curated, crate.License, crate.Name, crate.Authors)
			if err != nil {
				return nil, err
			}
		}
		components = append(components, component{
			Group:   groupRustStd,
			Name:    crate.Name,
			Version: crate.Version,
			License: crate.License,
			URL:     crate.URL,
			Texts:   texts,
		})
	}
	return components, nil
}

var toolchainChannel = regexp.MustCompile(`(?m)^channel\s*=\s*"([^"]+)"`)

// pinnedRustToolchain is the Rust version the desktop is built with.
func pinnedRustToolchain() (string, error) {
	body, err := os.ReadFile(filepath.Join(desktopCrate, "rust-toolchain.toml"))
	if err != nil {
		return "", err
	}
	match := toolchainChannel.FindSubmatch(body)
	if match == nil {
		return "", fmt.Errorf("%s/rust-toolchain.toml pins no channel", desktopCrate)
	}
	return string(match[1]), nil
}

// refreshRustStd writes curated/rust from the pinned toolchain's own copy
// of the standard library's copyright notices, which its rust-docs
// component installs as HTML: the library's own notice as std.md, and each
// crate it is built from, with its notices, in crates.json. Kept apart,
// the crates' license texts are printed once each in the notices, as every
// other component's are, rather than once per crate.
func refreshRustStd(curatedDir string) error {
	pinned, err := pinnedRustToolchain()
	if err != nil {
		return err
	}
	cmd := exec.Command("rustc", "--print", "sysroot")
	cmd.Dir = desktopCrate
	sysroot, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("finding the pinned toolchain: %w", err)
	}
	page := filepath.Join(strings.TrimSpace(string(sysroot)), "share", "doc", "rust", "COPYRIGHT-library.html")
	html, err := os.ReadFile(page)
	if err != nil {
		return fmt.Errorf("%w; install the toolchain's documentation with `rustup component add rust-docs`", err)
	}
	markdown, err := htmltomarkdown.ConvertString(string(html))
	if err != nil {
		return fmt.Errorf("reading %s: %w", page, err)
	}
	std, crates, err := splitStdNotices(markdown)
	if err != nil {
		return fmt.Errorf("reading %s: %w", page, err)
	}
	encoded, err := json.MarshalIndent(crates, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Join(curatedDir, "rust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, content := range map[string]string{
		"std.md":      normalize(std) + "\n",
		"crates.json": string(encoded) + "\n",
		versionFile:   pinned + "\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// outOfTree begins the section of COPYRIGHT-library listing the crates
// the standard library is built from.
const outOfTree = "## Out-of-tree dependencies"

var (
	stdCrateHeading = regexp.MustCompile(`(?m)^### .*$`)
	stdCrateURL     = regexp.MustCompile(`\*\*URL:\*\* \[(https://crates\.io/crates/([^/\]]+)/([^\]]+))\]`)
	stdCrateLicense = regexp.MustCompile(`(?m)^\*\*License:\*\* (.+)$`)
	stdCrateAuthors = regexp.MustCompile(`(?m)^\*\*Authors:\*\* (.+)$`)
	stdCrateNotice  = regexp.MustCompile("(?s)`([^`\n]+)`\n\n```\n(.*?)\n```")
)

// splitStdNotices reads COPYRIGHT-library, converted to Markdown, into the
// standard library's own notice and the crates it lists, each with every
// notice file it quotes.
func splitStdNotices(markdown string) (string, stdCrates, error) {
	std, rest, ok := strings.Cut(markdown, outOfTree)
	if !ok {
		return "", stdCrates{}, fmt.Errorf("no %q section", outOfTree)
	}
	sections := stdCrateHeading.Split(rest, -1)[1:]
	var listed stdCrates
	index := map[string]int{}
	for _, section := range sections {
		url := stdCrateURL.FindStringSubmatch(section)
		license := stdCrateLicense.FindStringSubmatch(section)
		if url == nil || license == nil {
			return "", stdCrates{}, fmt.Errorf("a crate's section has no crates.io URL or license: %.80q", section)
		}
		// Markdown escapes an underscore, which crate names are full of.
		unescape := strings.NewReplacer(`\_`, "_").Replace
		crate := stdCrate{
			Name:    unescape(url[2]),
			Version: url[3],
			License: strings.TrimSpace(license[1]),
			URL:     unescape(url[1]),
		}
		if authors := stdCrateAuthors.FindStringSubmatch(section); authors != nil {
			for _, author := range strings.Split(authors[1], ",") {
				if author = strings.TrimSpace(author); author != "" {
					crate.Authors = append(crate.Authors, author)
				}
			}
		}
		for _, notice := range stdCrateNotice.FindAllStringSubmatch(section, -1) {
			body := normalize(notice[2])
			at, ok := index[body]
			if !ok {
				at = len(listed.Texts)
				index[body] = at
				listed.Texts = append(listed.Texts, body)
			}
			crate.Notices = append(crate.Notices, stdNotice{File: notice[1], Text: at})
		}
		listed.Crates = append(listed.Crates, crate)
	}
	if len(listed.Crates) == 0 {
		return "", stdCrates{}, fmt.Errorf("no crates under %q", outOfTree)
	}
	return strings.TrimSpace(std), listed, nil
}
