package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// licenseFile matches the files a component keeps its license, copyright
// and notices in, at the top of its source: LICENSE, LICENSE-MIT,
// COPYING.txt, NOTICE, PATENTS and the like.
var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|copyright|notice|patents|unlicense)([._-][a-z0-9._-]*)?$`)

// codeExtensions are files a license name can begin and that are not a
// license: license.go, notice.rs.
var codeExtensions = map[string]bool{
	".go": true, ".rs": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true,
	".json": true, ".html": true, ".sh": true, ".py": true, ".toml": true,
}

// licenseTexts reads every license file at the top of dir, in name order.
func licenseTexts(dir string) ([]text, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var texts []text
	for _, entry := range entries {
		name := entry.Name()
		if !licenseFile.MatchString(name) || codeExtensions[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if t := (text{Name: name, Body: normalize(string(body))}); t.Body != "" {
			texts = append(texts, t)
		}
	}
	sort.Slice(texts, func(i, j int) bool { return texts[i].Name < texts[j].Name })
	return texts, nil
}

// curatedTexts reads every file in dir, which holds licenses no package
// manager carries: Go's own, Rust's, and PDFium's.
func curatedTexts(curated fs.FS, dir string) ([]text, error) {
	entries, err := fs.ReadDir(curated, dir)
	if err != nil {
		return nil, err
	}
	var texts []text
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == versionFile {
			continue
		}
		body, err := fs.ReadFile(curated, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		texts = append(texts, text{Name: entry.Name(), Body: normalize(string(body))})
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("%s holds no license", dir)
	}
	return texts, nil
}

// preferred is the order a license is chosen in from an expression that
// offers several, which needs only the one: the shortest to carry first.
var preferred = []string{"MIT", "BSD-3-Clause", "Apache-2.0", "MPL-2.0"}

// alternatives splits a license expression where it offers a choice.
var alternatives = regexp.MustCompile(`\s+OR\s+|\s*/\s*`)

// copyrightPlaceholder is the line in the canonical MIT and BSD texts that
// names whoever holds the copyright.
var copyrightPlaceholder = regexp.MustCompile(`(?m)^Copyright \(c\) <year> <[^>]*>\.?$`)

// declaredLicense is the text of the license a component declares but does
// not ship a file for: the canonical text, from the curated spdx
// directory, with its authors as the holders where the text names them.
//
// An expression offering a choice ("MIT OR Apache-2.0", or the older
// "MIT/Apache-2.0") is read as the first of preferred it offers; an
// alternative with an exception ("Apache-2.0 WITH LLVM-exception") is
// never the one chosen. One that needs several licenses at once ("AND")
// is refused rather than guessed at: no component without a license file
// declares one yet.
func declaredLicense(curated fs.FS, expression, component string, authors []string) ([]text, error) {
	if strings.Contains(expression, " AND ") {
		return nil, fmt.Errorf("%s declares %q and ships no license file; gennotices does not yet print licenses that apply together", component, expression)
	}
	offered := map[string]bool{}
	for _, alternative := range alternatives.Split(expression, -1) {
		offered[strings.Trim(alternative, "() ")] = true
	}
	for _, id := range preferred {
		if !offered[id] {
			continue
		}
		body, err := fs.ReadFile(curated, path.Join("spdx", id+".txt"))
		if err != nil {
			return nil, err
		}
		holders := "the " + component + " authors"
		if len(authors) > 0 {
			holders = strings.Join(authors, ", ")
		}
		license := copyrightPlaceholder.ReplaceAllLiteralString(normalize(string(body)), "Copyright (c) "+holders)
		return []text{{
			Name: id + ", which " + component + " declares without shipping a license file",
			Body: license,
		}}, nil
	}
	return nil, fmt.Errorf("%s declares %q and ships no license file, and gennotices holds none of its licenses' texts", component, expression)
}

// normalize makes two copies of one license text the same string however
// their line endings and trailing spaces differ, so they share one copy in
// the notices. A text that is not UTF-8 is an older file's Latin-1, its
// "©" a single byte, and is read as such.
func normalize(s string) string {
	if !utf8.ValidString(s) {
		latin1 := make([]rune, len(s))
		for i := 0; i < len(s); i++ {
			latin1[i] = rune(s[i])
		}
		s = string(latin1)
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}
