package documents

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/conductor-oss/markitdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata goldens from the current converter output")

const (
	fixturePDF    = "testdata/resume.pdf"
	fixtureGolden = "testdata/resume.golden.md"
)

// TestPDFFixtureStillProvokesTheDefect is the reason the fixture exists.
//
// A regression test for ADR-0016 is worth nothing if the sample PDF happens
// not to trigger the bug, because then the test passes whether the fix is
// present or not. So this asserts the opposite of everything else here: that
// Go MarkItDown's rectangle-based extraction really does mangle this file.
//
// If it ever stops failing — upstream fixed it, or the fixture was
// regenerated into something harmless — the guarantee has quietly
// evaporated and this test says so.
//
// It does not run in parallel, and neither do the other tests here that
// convert a PDF: this one starts Go MarkItDown's WebAssembly engine and the
// rest start ours, and two runtimes coming up at once race on a package
// global inside wazero. Nothing is lost by serializing them, since the
// engine pool holds one instance anyway.
func TestPDFFixtureStillProvokesTheDefect(t *testing.T) {
	result, err := markitdown.New().Convert(fixturePDF)
	require.NoError(t, err)

	assert.NotContains(t, result.Markdown, "Ada Lovelace",
		"the rectangle path is expected to break the candidate's name")
	assert.NotContains(t, result.Markdown, "ada.lovelace.dev@example.com",
		"the rectangle path is expected to break the candidate's email address")
	assert.Contains(t, result.Markdown, "SUMMARYRY",
		"duplication from overlapping rectangles is the signature of the defect")
}

func TestConverter_ConvertFile_PDF(t *testing.T) {
	doc, err := NewConverter().ConvertFile(context.Background(), fixturePDF)
	require.NoError(t, err)

	t.Run("the facts a form would ask for survive intact", func(t *testing.T) {
		// Each of these is wrong in the rectangle path above. A mangled
		// email is the dangerous case: it is plausible, so no downstream
		// guard can catch it, and it would be typed into an employer's
		// form as the candidate's own.
		for _, want := range []string{
			"Ada Lovelace",
			"ada.lovelace.dev@example.com",
			"Berlin, Germany",
			"PROFESSIONAL SUMMARY",
			"end-to-end",
			"efficiency",
			"Staff Engineer",
			"Jul 2022",
			"4.1s to 0.8s",
		} {
			assert.Contains(t, doc.Markdown, want)
		}
	})

	t.Run("hyperlinks are recovered with the text they sit on", func(t *testing.T) {
		// The URLs are annotations, not text: the content stream holds only
		// the word "Github". Without this the model has nothing to extract
		// but the label, which is how "ub" was once proposed as a
		// candidate's GitHub URL.
		assert.Contains(t, doc.Markdown, "Github -> https://github.com/adalovelace")
		assert.Contains(t, doc.Markdown, "LinkedIn -> https://www.linkedin.com/in/ada-lovelace")
		assert.Contains(t, doc.Markdown, "Blog -> https://ada.example.dev/")
		assert.Contains(t, doc.Markdown, "mailto:ada.lovelace.dev@example.com")
	})

	t.Run("a link that is not the candidate's own is distinguishable", func(t *testing.T) {
		// A resume links to employers, projects and certificates. Pairing
		// each URL with its anchor is what stops the open-source repo below
		// being read as this candidate's GitHub profile — a plausible,
		// wrong, and entirely fabricated fact.
		assert.Contains(t, doc.Markdown, "Acme Analytics -> https://acme.example.com/")
		assert.Contains(t, doc.Markdown,
			"github.com/someone-else/fast-parser -> https://github.com/someone-else/fast-parser")
	})

	t.Run("matches the golden", func(t *testing.T) {
		if *updateGolden {
			require.NoError(t, os.WriteFile(fixtureGolden, []byte(doc.Markdown), 0o644))
			t.Log("golden rewritten")
			return
		}
		want, err := os.ReadFile(fixtureGolden)
		require.NoError(t, err, "run: go test ./internal/adapters/secondary/documents -update-golden")
		assert.Equal(t, string(want), doc.Markdown)
	})
}

func TestConverter_ConvertFile_NonPDF(t *testing.T) {
	t.Run("other formats still go through Go MarkItDown", func(t *testing.T) {
		doc, err := NewConverter().ConvertFile(context.Background(), "testdata/resume.html")
		require.NoError(t, err)
		assert.Contains(t, doc.Markdown, "Ada Lovelace")
		assert.Contains(t, doc.Markdown, "PROFESSIONAL SUMMARY")
	})

	t.Run("a missing file names itself", func(t *testing.T) {
		_, err := NewConverter().ConvertFile(context.Background(), "testdata/nothing-here.pdf")
		assert.ErrorContains(t, err, "testdata/nothing-here.pdf")
	})

	t.Run("a PDF is routed by its bytes, not by its name", func(t *testing.T) {
		// The desktop's picker accepts any file and people rename things.
		// Sending a PDF to the markdown engine because it was called .txt
		// is the silent corruption this whole change is about.
		pdf, err := os.ReadFile(fixturePDF)
		require.NoError(t, err)
		misnamed := filepath.Join(t.TempDir(), "resume.txt")
		require.NoError(t, os.WriteFile(misnamed, pdf, 0o644))

		doc, err := NewConverter().ConvertFile(context.Background(), misnamed)

		require.NoError(t, err)
		assert.Contains(t, doc.Markdown, "Ada Lovelace")
	})

	t.Run("a file that is not a PDF is not treated as one", func(t *testing.T) {
		plain := filepath.Join(t.TempDir(), "notes.txt")
		require.NoError(t, os.WriteFile(plain, []byte("Ada Lovelace\nEngineer\n"), 0o644))

		doc, err := NewConverter().ConvertFile(context.Background(), plain)

		require.NoError(t, err)
		assert.Contains(t, doc.Markdown, "Ada Lovelace")
	})
}

func TestConverter_ConvertHTML(t *testing.T) {
	t.Parallel()

	doc, err := NewConverter().ConvertHTML(context.Background(),
		"<div><h2>About the role</h2><p>We need a <strong>Go</strong> engineer.</p></div>")

	require.NoError(t, err)
	assert.Contains(t, doc.Markdown, "About the role")
	assert.Contains(t, doc.Markdown, "Go")
	assert.NotContains(t, doc.Markdown, "<strong>", "markup should not reach a prompt")
}

func TestLinksSection(t *testing.T) {
	t.Parallel()

	t.Run("no links means no section", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, linksSection(nil))
	})

	t.Run("a link without an anchor still lists its URL", func(t *testing.T) {
		t.Parallel()
		got := linksSection([]recoveredLink{{URL: "https://example.com/"}})
		assert.Contains(t, got, "- https://example.com/")
		assert.NotContains(t, got, "->")
	})

	t.Run("the section says these are not all the candidate's", func(t *testing.T) {
		t.Parallel()
		got := linksSection([]recoveredLink{{Anchor: "Github", URL: "https://github.com/ada"}})
		assert.Contains(t, got, "Github -> https://github.com/ada")
		assert.True(t, strings.Contains(got, "rather than to the candidate"),
			"the model is told the list is raw, because most links on a resume are not the candidate's")
	})
}

func TestIsPDF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"a PDF", []byte("%PDF-1.7\n..."), true},
		{"junk before the header, as readers tolerate", append(make([]byte, 200), []byte("%PDF-1.4")...), true},
		{"plain text", []byte("Ada Lovelace, engineer"), false},
		{"empty", nil, false},
		{"the word PDF in prose is not a signature", []byte("I have attached my PDF resume"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isPDF(tt.in))
		})
	}
}
