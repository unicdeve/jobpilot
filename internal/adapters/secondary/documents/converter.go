// Package documents converts resumes and job descriptions to markdown.
//
// Go MarkItDown does the work for every format except PDF, whose converter
// reads text out of PDFium text rectangles and both duplicates and deletes
// characters as a result. PDF is handled here instead, on the same PDFium
// build MarkItDown already depends on. See ADR-0016.
package documents

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/conductor-oss/markitdown"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// Converter implements ports.DocumentConverter.
type Converter struct {
	pdf *pdfExtractor
}

// NewConverter constructs a Converter. The PDF engine is not started until
// a PDF is actually converted, so a process that never reads one — the CLI
// listing jobs, a test — pays nothing for it.
func NewConverter() *Converter {
	return &Converter{pdf: newPDFExtractor()}
}

// ConvertFile reads a document from disk.
func (c *Converter) ConvertFile(ctx context.Context, path string) (ports.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ports.Document{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	if isPDF(data) {
		return c.convertPDF(ctx, data)
	}
	return convertWithMarkItDown(path)
}

// ConvertBytes converts a document handed over as its content, such as a
// file a browser uploaded (ADR-0035 §4).
//
// A PDF is recognised by its content. Anything else goes to MarkItDown,
// which reads a file and goes by its extension, so the content is written
// to a temporary file named with name's extension, and removed afterwards.
func (c *Converter) ConvertBytes(ctx context.Context, name string, data []byte) (ports.Document, error) {
	if isPDF(data) {
		return c.convertPDF(ctx, data)
	}

	ext := strings.ToLower(filepath.Ext(filepath.Base(name)))
	if !documentExtension.MatchString(ext) {
		return ports.Document{}, fmt.Errorf("%q has no extension that says what kind of document it is", name)
	}
	dir, err := os.MkdirTemp("", "jobpilot-document-")
	if err != nil {
		return ports.Document{}, fmt.Errorf("failed to make room to convert %s: %w", name, err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "document"+ext)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return ports.Document{}, fmt.Errorf("failed to make room to convert %s: %w", name, err)
	}
	doc, err := convertWithMarkItDown(path)
	if err != nil {
		return ports.Document{}, fmt.Errorf("failed to convert %s to markdown: %w", name, err)
	}
	return doc, nil
}

// documentExtension is an extension ConvertBytes will name a temporary
// file with: a dot and a few letters or digits, and nothing a name sent by
// a client could use to reach outside the temporary directory.
var documentExtension = regexp.MustCompile(`^\.[a-z0-9]{1,10}$`)

func (c *Converter) convertPDF(ctx context.Context, data []byte) (ports.Document, error) {
	markdown, err := c.pdf.extract(ctx, data)
	if err != nil {
		return ports.Document{}, err
	}
	return ports.Document{Markdown: markdown}, nil
}

func convertWithMarkItDown(path string) (ports.Document, error) {
	md := markitdown.New()
	result, err := md.Convert(path)
	if err != nil {
		return ports.Document{}, fmt.Errorf("failed to convert %s to markdown: %w", path, err)
	}
	return ports.Document{Markdown: result.Markdown}, nil
}

// ConvertHTML converts an HTML fragment.
func (c *Converter) ConvertHTML(_ context.Context, html string) (ports.Document, error) {
	md := markitdown.New()
	result, err := md.ConvertReader(bytes.NewReader([]byte(html)), markitdown.StreamInfo{MIMEType: "text/html"})
	if err != nil {
		return ports.Document{}, fmt.Errorf("failed to convert HTML to markdown: %w", err)
	}
	return ports.Document{Markdown: result.Markdown}, nil
}

// isPDF reports whether the bytes are a PDF, by signature rather than by
// file extension.
//
// The desktop's file picker accepts any file and a resume is routinely
// saved under the wrong name, so the extension is a claim about the
// content, not evidence of it. Handing a PDF to MarkItDown because it was
// named ".doc" is exactly the silent corruption ADR-0016 is about.
func isPDF(data []byte) bool {
	// The header is allowed a little junk in front of it; readers in
	// practice scan the first kilobyte, and so does PDFium.
	limit := min(len(data), 1024)
	return bytes.Contains(data[:limit], []byte("%PDF-"))
}

// linksSection renders recovered hyperlinks for the model to read.
//
// A PDF hyperlink is an annotation with a rectangle, not text: the content
// stream holds the word "Github" and nothing else, which is why extracting
// one used to propose "ub" as a candidate's GitHub URL. These are the real
// targets, paired with the text they sit on so that a link belonging to an
// employer or a project is not read as the candidate's own. They remain
// untrusted candidate-supplied values.
func linksSection(links []recoveredLink) string {
	if len(links) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Hyperlinks in this document\n\n")
	b.WriteString("Each line is the text the reader sees, then the address it links to. ")
	b.WriteString("Some of these belong to employers, projects or certificates rather than to the candidate.\n\n")
	for _, link := range links {
		b.WriteString("- ")
		if link.Anchor != "" {
			b.WriteString(link.Anchor)
			b.WriteString(" -> ")
		}
		b.WriteString(link.URL)
		b.WriteString("\n")
	}
	return b.String()
}
