package documents

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/structs"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// instanceTimeout bounds the wait for the single PDFium instance. Two
// imports at once queue rather than fail; a wait longer than this means
// something is wedged.
const instanceTimeout = 30 * time.Second

// maxRecoveredLinks caps the hyperlinks appended to a converted document.
// A resume has a handful. A document with thousands is either generated or
// hostile, and either way its annotation list is not worth pasting into a
// model's context.
const maxRecoveredLinks = 64

// pdfExtractor reads text out of PDFs with PDFium.
//
// It deliberately uses PDFium's whole-page text rather than its text
// rectangles. The rectangles carry font geometry, which is why Go
// MarkItDown uses them to infer heading levels, but bounded text from
// overlapping rectangles emits a character twice and a narrow glyph whose
// box falls between two rectangles is emitted by neither. Measured on a
// real resume that costs about a third of the document, including the
// candidate's name and the dots in their email address (ADR-0016).
type pdfExtractor struct{}

func newPDFExtractor() *pdfExtractor {
	return &pdfExtractor{}
}

// The PDF engine is process-wide, not per-Converter.
//
// It is a WebAssembly runtime, and starting a second one is both wasteful
// and unsafe: two concurrent webassembly.Init calls race on a package
// global inside wazero, which the race detector catches. Constructing more
// than one Converter is entirely reasonable — the daemon has one and the
// job-board connectors have another — so the engine cannot belong to any
// of them.
var (
	pdfPoolOnce sync.Once
	pdfPool     pdfium.Pool
	pdfPoolErr  error
)

// instance borrows a PDFium instance, starting the engine on first use.
//
// The pool holds a single instance, so concurrent conversions queue rather
// than run at once. That is the engine's own constraint, not a choice.
func (e *pdfExtractor) instance() (pdfium.Pdfium, error) {
	pdfPoolOnce.Do(func() {
		pdfPool, pdfPoolErr = webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1})
	})
	if pdfPoolErr != nil {
		return nil, fmt.Errorf("failed to start the PDF engine: %w", pdfPoolErr)
	}
	instance, err := pdfPool.GetInstance(instanceTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a PDF engine instance: %w", err)
	}
	return instance, nil
}

// extract converts a PDF's bytes to markdown.
func (e *pdfExtractor) extract(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	instance, err := e.instance()
	if err != nil {
		return "", err
	}
	defer func() { _ = instance.Close() }()

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		// A password-protected or malformed file lands here. Say so
		// plainly: the user chose this file and can choose another.
		return "", fmt.Errorf("the PDF could not be opened: %w", err)
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	}()

	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return "", fmt.Errorf("the PDF could not be read: %w", err)
	}

	var (
		body  strings.Builder
		links []recoveredLink
		seen  = map[string]bool{}
	)
	for index := 0; index < pageCount.PageCount; index++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: index}}

		text, err := instance.GetPageText(&requests.GetPageText{Page: page})
		if err != nil {
			return "", fmt.Errorf("page %d of the PDF could not be read: %w", index+1, err)
		}
		if trimmed := strings.TrimSpace(text.Text); trimmed != "" {
			body.WriteString(trimmed)
			body.WriteString("\n\n")
		}

		for _, link := range e.pageLinks(instance, doc.Document, page) {
			if seen[link.URL] || len(links) >= maxRecoveredLinks {
				continue
			}
			seen[link.URL] = true
			links = append(links, link)
		}
	}

	markdown := strings.TrimSpace(body.String())
	if markdown == "" && len(links) == 0 {
		// A scanned resume is an image with no text layer. Reporting this
		// as an error is better than storing an empty document that every
		// later step then silently reads as "the candidate said nothing".
		return "", fmt.Errorf("no text could be read from the PDF; it may be a scan or an image")
	}

	return markdown + linksSection(links), nil
}

// recoveredLink is a hyperlink annotation: the text the reader sees and
// the URL it actually points at.
type recoveredLink struct {
	Anchor string
	URL    string
}

// pageLinks returns a page's link annotations, in page order, each paired
// with the text it sits on top of.
//
// The anchor matters as much as the URL. A resume's hyperlinks are mostly
// not the candidate's own: employers, projects they contributed to,
// certificates. A bare list of URLs invites the model to read
// "github.com/ThallesP/nestjs-better-auth" as the candidate's GitHub
// profile — a plausible, wrong, and entirely fabricated fact, which is the
// failure ADR-0016 exists to stop rather than relocate. "Github" next to
// the URL says whose it is.
//
// Anchors come from PDFium's per-character positions, not from its text
// rectangles. Characters are one-to-one with the text page, so the
// duplication and deletion that made the rectangle path unusable cannot
// occur here.
//
// Failures are not errors: an annotation pointing inside the document
// rather than at a URL is normal, and a document whose annotations cannot
// be walked still has perfectly good text.
func (e *pdfExtractor) pageLinks(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, page requests.Page) []recoveredLink {
	chars := e.pageChars(instance, page)

	var (
		links []recoveredLink
		start int
	)
	for {
		enumerated, err := instance.FPDFLink_Enumerate(&requests.FPDFLink_Enumerate{Page: page, StartPos: start})
		if err != nil || enumerated.Link == nil || enumerated.NextStartPos == nil {
			return links
		}
		start = *enumerated.NextStartPos

		action, err := instance.FPDFLink_GetAction(&requests.FPDFLink_GetAction{Link: *enumerated.Link})
		if err != nil || action.Action == nil {
			continue
		}
		uri, err := instance.FPDFAction_GetURIPath(&requests.FPDFAction_GetURIPath{
			Document: document,
			Action:   *action.Action,
		})
		if err != nil || uri.URIPath == nil {
			continue
		}
		url := strings.TrimSpace(*uri.URIPath)
		if url == "" {
			continue
		}

		link := recoveredLink{URL: url}
		if rect, err := instance.FPDFLink_GetAnnotRect(&requests.FPDFLink_GetAnnotRect{
			Link: *enumerated.Link,
		}); err == nil && rect.Rect != nil {
			link.Anchor = anchorWithin(chars, *rect.Rect)
		}
		links = append(links, link)
	}
}

// pageChars returns a page's characters with their positions, or nil when
// the page has no readable text layer.
func (e *pdfExtractor) pageChars(instance pdfium.Pdfium, page requests.Page) []*responses.GetPageTextStructuredChar {
	structured, err := instance.GetPageTextStructured(&requests.GetPageTextStructured{
		Page: page,
		Mode: requests.GetPageTextStructuredModeChars,
	})
	if err != nil {
		return nil
	}
	return structured.Chars
}

// anchorWithin returns the text whose characters fall inside a rectangle.
//
// A character counts when its centre is inside, so a glyph that pokes a
// fraction past the annotation's edge — which is usual, since the
// rectangle is drawn around the visible run rather than around the glyph
// boxes — still belongs to it.
func anchorWithin(chars []*responses.GetPageTextStructuredChar, rect structs.FPDF_FS_RECTF) string {
	left, right := math.Min(float64(rect.Left), float64(rect.Right)), math.Max(float64(rect.Left), float64(rect.Right))
	bottom, top := math.Min(float64(rect.Bottom), float64(rect.Top)), math.Max(float64(rect.Bottom), float64(rect.Top))

	var b strings.Builder
	for _, char := range chars {
		if char == nil {
			continue
		}
		x := (char.PointPosition.Left + char.PointPosition.Right) / 2
		y := (char.PointPosition.Top + char.PointPosition.Bottom) / 2
		if x >= left && x <= right && y >= bottom && y <= top {
			b.WriteString(char.Text)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
