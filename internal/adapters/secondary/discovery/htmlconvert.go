package discovery

import (
	"context"
	"fmt"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/documents"
)

// htmlConverter is shared by the connectors in this package. Conversion is
// stateless for HTML, and holding one keeps a single implementation of
// "what markdown does a job description become" across the product
// (ADR-0016).
var htmlConverter = documents.NewConverter()

// htmlToMarkdown converts an HTML job description, as returned by a job
// board's API, to the markdown stored on the job.
func htmlToMarkdown(html string) (string, error) {
	doc, err := htmlConverter.ConvertHTML(context.Background(), html)
	if err != nil {
		return "", fmt.Errorf("failed to convert job description to markdown: %w", err)
	}
	return doc.Markdown, nil
}
