package discovery

import (
	"bytes"
	"fmt"

	"github.com/conductor-oss/markitdown"
)

// htmlToMarkdown converts an HTML job description (as returned by
// job-board APIs) to markdown using the same MarkItDown engine
// internal/core/services/resume.go uses for resume documents, per
// docs/09_decisions/0006-go-markitdown.md.
func htmlToMarkdown(html string) (string, error) {
	md := markitdown.New()
	result, err := md.ConvertReader(bytes.NewReader([]byte(html)), markitdown.StreamInfo{MIMEType: "text/html"})
	if err != nil {
		return "", fmt.Errorf("failed to convert job description to markdown: %w", err)
	}
	return result.Markdown, nil
}
