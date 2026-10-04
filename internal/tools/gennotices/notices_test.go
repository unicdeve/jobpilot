package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLicenseTexts(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"LICENSE-MIT":    "MIT terms\r\n",
		"LICENSE-APACHE": "Apache terms  \n\n",
		"COPYING.txt":    "copying",
		"NOTICE":         "a notice",
		"PATENTS":        "a grant",
		"license.go":     "package license",
		"README.md":      "readme",
		"Cargo.toml":     "[package]",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "licenses"), 0o755))

	texts, err := licenseTexts(dir)

	require.NoError(t, err)
	assert.Equal(t, []text{
		{Name: "COPYING.txt", Body: "copying"},
		{Name: "LICENSE-APACHE", Body: "Apache terms"},
		{Name: "LICENSE-MIT", Body: "MIT terms"},
		{Name: "NOTICE", Body: "a notice"},
		{Name: "PATENTS", Body: "a grant"},
	}, texts, "license, copying, notice and patent files, normalized; never code, a directory or a readme")
}

func TestDeclaredLicense(t *testing.T) {
	curated := os.DirFS("curated")

	for expression, want := range map[string]string{
		"MIT":                       "MIT",
		"Zlib OR Apache-2.0 OR MIT": "MIT",
		"MIT/Apache-2.0":            "MIT",
		"Apache-2.0 WITH LLVM-exception OR Apache-2.0 OR MIT": "MIT",
		"BSD-3-Clause":        "BSD-3-Clause",
		"MPL-2.0":             "MPL-2.0",
		"(Apache-2.0 OR MIT)": "MIT",
	} {
		texts, err := declaredLicense(curated, expression, "widget", []string{"Ada Lovelace <ada@example.com>"})
		require.NoError(t, err, expression)
		require.Len(t, texts, 1, expression)
		assert.True(t, strings.HasPrefix(texts[0].Name, want+", which widget declares"), "%s: %s", expression, texts[0].Name)
	}

	mit, err := declaredLicense(curated, "MIT", "widget", []string{"Ada Lovelace <ada@example.com>", "Charles Babbage"})
	require.NoError(t, err)
	assert.Contains(t, mit[0].Body, "Copyright (c) Ada Lovelace <ada@example.com>, Charles Babbage\n",
		"the canonical text's placeholder names the authors")
	assert.NotContains(t, mit[0].Body, "<year>")

	anonymous, err := declaredLicense(curated, "BSD-3-Clause", "widget", nil)
	require.NoError(t, err)
	assert.Contains(t, anonymous[0].Body, "Copyright (c) the widget authors")

	for _, refused := range []string{"MIT AND Apache-2.0", "GPL-3.0-only", "Apache-2.0 WITH LLVM-exception"} {
		_, err := declaredLicense(curated, refused, "widget", nil)
		assert.Error(t, err, "%s is refused rather than guessed at", refused)
	}
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "Copyright © 2001 Ada\nline", normalize("\n\nCopyright \xa9 2001 Ada  \r\nline\t\n\n"),
		"Latin-1 is read as such, and line endings and trailing space do not matter")
	assert.Equal(t, "Copyright © 2001", normalize("Copyright © 2001"), "UTF-8 is left alone")
}

func TestRender(t *testing.T) {
	apache := "Apache License\n\n  Version 2.0"
	components := []component{
		{Group: groupNPM, Name: "zeta", Version: "1.0.0", License: "MIT", URL: "https://npm/zeta", Texts: []text{{Name: "LICENSE", Body: "MIT for zeta"}}},
		{Group: groupRustCrates, Name: "beta", Version: "0.2.0", URL: "https://crates/beta", Texts: []text{{Name: "LICENSE-APACHE", Body: apache}}},
		{Group: groupGoModules, Name: "example.com/alpha", Version: "v1.0.0", URL: "https://pkg/alpha", Texts: []text{{Name: "LICENSE", Body: "  Apache License\n\nVersion   2.0"}}},
	}

	out := render(components)

	assert.Equal(t, 2, strings.Count(out, "The license of:"), "the two Apache copies differ only in spacing, and share one")
	assert.Less(t, strings.Index(out, "example.com/alpha"), strings.Index(out, "beta 0.2.0"), "groups keep their order")
	assert.Less(t, strings.Index(out, "beta 0.2.0"), strings.Index(out, "zeta 1.0.0"))
	assert.Contains(t, out, "  example.com/alpha v1.0.0, license [1]\n    https://pkg/alpha\n")
	assert.Contains(t, out, "  beta 0.2.0, license [1]\n")
	assert.Contains(t, out, "  zeta 1.0.0, MIT, license [2]\n")
	assert.Contains(t, out, "[1] The license of:\n      example.com/alpha v1.0.0 (LICENSE)\n      beta 0.2.0 (LICENSE-APACHE)\n")

	reversed := []component{components[2], components[1], components[0]}
	assert.Equal(t, out, render(reversed), "the same components render the same whatever order they arrive in")
}

func TestLinkedCrates(t *testing.T) {
	dev := "dev"
	normal := []cargoDepKind{{}}
	metadata := cargoMetadata{Packages: []cargoPackage{
		{ID: "app", Name: "app"},
		{ID: "lib", Name: "lib"},
		{ID: "leaf", Name: "leaf"},
		{ID: "macro", Name: "macro", Targets: []cargoTarget{{Kind: []string{"proc-macro"}}}},
		{ID: "macrodep", Name: "macrodep"},
		{ID: "testonly", Name: "testonly"},
	}}
	metadata.Resolve.Root = "app"
	metadata.Resolve.Nodes = []cargoNode{
		{ID: "app", Deps: []cargoDep{
			{Pkg: "lib", DepKinds: normal},
			{Pkg: "macro", DepKinds: normal},
			{Pkg: "testonly", DepKinds: []cargoDepKind{{Kind: &dev}}},
		}},
		{ID: "lib", Deps: []cargoDep{{Pkg: "leaf", DepKinds: normal}}},
		{ID: "macro", Deps: []cargoDep{{Pkg: "macrodep", DepKinds: normal}}},
	}

	var names []string
	for _, pkg := range linkedCrates(metadata) {
		names = append(names, pkg.Name)
	}

	assert.ElementsMatch(t, []string{"lib", "leaf"}, names,
		"normal dependencies, transitively; not a dev dependency, a procedural macro, or what only a macro uses")
}

func TestDecodeLinkedModules(t *testing.T) {
	stream := `{"ImportPath":"fmt"}
{"Module":{"Path":"github.com/unicdev/jobpilot","Main":true}}
{"Module":{"Path":"example.com/a","Version":"v1.0.0","Dir":"/mod/a"}}
{"Module":{"Path":"example.com/a","Version":"v1.0.0","Dir":"/mod/a"}}
{"Module":{"Path":"example.com/b","Version":"v2.0.0","Dir":"/mod/b-orig","Replace":{"Path":"../b","Dir":"/src/b"}}}
`
	modules, err := decodeLinkedModules(strings.NewReader(stream))

	require.NoError(t, err)
	require.Len(t, modules, 2, "the standard library and JobPilot itself are not modules it credits, and each module is listed once")
	assert.Equal(t, "/mod/a", modules[0].Dir)
	assert.Equal(t, "/src/b", modules[1].Dir, "a replaced module is read where it is replaced from")
}

func TestSplitStdNotices(t *testing.T) {
	markdown := "# Copyright notices for The Rust Standard Library\n\nDual-licensed.\n\n" + outOfTree + "\n\nThe crates:\n\n" +
		"### 📦 hash\\_brown-0.1.0\n\n**URL:** [https://crates.io/crates/hash\\_brown/0.1.0](https://crates.io/crates/hash\\_brown/0.1.0)\n\n" +
		"**Authors:** Ada, Charles\n\n**License:** MIT OR Apache-2.0\n\n**Notices:**\n\n`LICENSE-MIT`\n\n```\nMIT text\n```\n\n`LICENSE-APACHE`\n\n```\nApache text\n```\n\n" +
		"### 📦 sgx-abi-0.6.1\n\n**URL:** [https://crates.io/crates/sgx-abi/0.6.1](https://crates.io/crates/sgx-abi/0.6.1)\n\n**License:** MPL-2.0\n\n" +
		"### 📦 other-1.0.0\n\n**URL:** [https://crates.io/crates/other/1.0.0](https://crates.io/crates/other/1.0.0)\n\n**License:** MIT\n\n`LICENSE`\n\n```\nMIT text\n```\n"

	std, listed, err := splitStdNotices(markdown)

	require.NoError(t, err)
	assert.Equal(t, "# Copyright notices for The Rust Standard Library\n\nDual-licensed.", std)
	require.Len(t, listed.Crates, 3)
	assert.Equal(t, []string{"MIT text", "Apache text"}, listed.Texts, "a text quoted twice is stored once")
	assert.Equal(t, stdCrate{
		Name: "hash_brown", Version: "0.1.0", License: "MIT OR Apache-2.0", Authors: []string{"Ada", "Charles"},
		URL:     "https://crates.io/crates/hash_brown/0.1.0",
		Notices: []stdNotice{{File: "LICENSE-MIT", Text: 0}, {File: "LICENSE-APACHE", Text: 1}},
	}, listed.Crates[0])
	assert.Empty(t, listed.Crates[1].Notices, "a crate listed by license alone")
	assert.Equal(t, []stdNotice{{File: "LICENSE", Text: 0}}, listed.Crates[2].Notices)
}

func TestSplitPackage(t *testing.T) {
	for entry, want := range map[string][2]string{
		"react@19.1.0":          {"react", "19.1.0"},
		"@base-ui/react@1.8.0":  {"@base-ui/react", "1.8.0"},
		"@scope/pkg@1.0.0-rc.1": {"@scope/pkg", "1.0.0-rc.1"},
	} {
		name, version, err := splitPackage(entry)
		require.NoError(t, err, entry)
		assert.Equal(t, want, [2]string{name, version}, entry)
	}
	_, _, err := splitPackage("@scope/pkg")
	assert.Error(t, err)
}
