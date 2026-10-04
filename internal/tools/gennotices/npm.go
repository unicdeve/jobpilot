package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// bundledPackagesFile is what the desktop's Vite build writes: every npm
// package whose code it put into the app (desktop/bundled-packages.ts).
const bundledPackagesFile = "desktop/bundled-packages.json"

// nodeModules is where `bun install` puts the desktop's packages.
const nodeModules = "desktop/node_modules"

type npmManifest struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	License json.RawMessage `json:"license"`
	Author  json.RawMessage `json:"author"`
}

// npmComponents is every npm package the desktop's build bundled.
func npmComponents(curated fs.FS) ([]component, error) {
	body, err := os.ReadFile(bundledPackagesFile)
	if err != nil {
		return nil, fmt.Errorf("%w; it is written by the desktop's build (`bun run build`)", err)
	}
	var bundled []string
	if err := json.Unmarshal(body, &bundled); err != nil {
		return nil, fmt.Errorf("reading %s: %w", bundledPackagesFile, err)
	}

	var components []component
	for _, entry := range bundled {
		name, version, err := splitPackage(entry)
		if err != nil {
			return nil, err
		}
		dir, manifest, err := findPackage(name, version)
		if err != nil {
			return nil, err
		}
		texts, err := licenseTexts(dir)
		if err != nil {
			return nil, fmt.Errorf("reading %s's license: %w", entry, err)
		}
		license := manifestLicense(manifest)
		if len(texts) == 0 {
			var authors []string
			if author := manifestAuthor(manifest); author != "" {
				authors = []string{author}
			}
			texts, err = declaredLicense(curated, license, name, authors)
			if err != nil {
				return nil, err
			}
		}
		components = append(components, component{
			Group:   groupNPM,
			Name:    name,
			Version: version,
			License: license,
			URL:     "https://www.npmjs.com/package/" + name + "/v/" + version,
			Texts:   texts,
		})
	}
	return components, nil
}

// splitPackage reads "name@version", where a scoped name begins with "@".
func splitPackage(entry string) (name, version string, err error) {
	at := strings.LastIndex(entry, "@")
	if at <= 0 {
		return "", "", fmt.Errorf("%q in %s is not name@version", entry, bundledPackagesFile)
	}
	return entry[:at], entry[at+1:], nil
}

// findPackage is the directory a package of this version is installed in:
// the top of node_modules, or a node_modules nested in another package
// where two versions of it are installed.
func findPackage(name, version string) (string, npmManifest, error) {
	if manifest, ok := readManifest(filepath.Join(nodeModules, name), version); ok {
		return filepath.Join(nodeModules, name), manifest, nil
	}
	var found string
	var foundManifest npmManifest
	suffix := string(filepath.Separator) + filepath.Join("node_modules", name)
	err := filepath.WalkDir(nodeModules, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return err
		}
		if entry.IsDir() && strings.HasSuffix(path, suffix) {
			if manifest, ok := readManifest(path, version); ok {
				found, foundManifest = path, manifest
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return "", npmManifest{}, err
	}
	if found == "" {
		return "", npmManifest{}, fmt.Errorf("%s@%s, which the desktop's build bundled, is not installed in %s; run `bun install`", name, version, nodeModules)
	}
	return found, foundManifest, nil
}

func readManifest(dir, version string) (npmManifest, bool) {
	body, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return npmManifest{}, false
	}
	var manifest npmManifest
	if json.Unmarshal(body, &manifest) != nil || manifest.Version != version {
		return npmManifest{}, false
	}
	return manifest, true
}

// manifestLicense is a package's declared license, written either as an
// SPDX expression or, in older packages, as {"type": "MIT"}.
func manifestLicense(manifest npmManifest) string {
	var expression string
	if json.Unmarshal(manifest.License, &expression) == nil {
		return expression
	}
	var legacy struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(manifest.License, &legacy) == nil {
		return legacy.Type
	}
	return ""
}

// manifestAuthor is a package's author, written either as "Name <email>"
// or as {"name": "Name"}.
func manifestAuthor(manifest npmManifest) string {
	var author string
	if json.Unmarshal(manifest.Author, &author) == nil {
		return author
	}
	var person struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(manifest.Author, &person) == nil {
		return person.Name
	}
	return ""
}
