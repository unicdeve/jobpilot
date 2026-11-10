package ports

import "context"

// DocumentConverter turns a document into the markdown the rest of the
// product reads: the resume a profile draft is extracted from, and the job
// description a match is scored against.
//
// It is a port because core must not choose a conversion library
// (ADR-0016). It is also the reason ResumeService can be tested without
// running a PDF engine.
type DocumentConverter interface {
	// ConvertFile reads a document from disk. The format is determined
	// from the file itself, not from the caller.
	ConvertFile(ctx context.Context, path string) (Document, error)
	// ConvertBytes converts a document handed over as its content, such
	// as a file a browser uploaded, which has a name and no path. The
	// format is determined from the content where it can be, and
	// otherwise from the extension of name.
	ConvertBytes(ctx context.Context, name string, data []byte) (Document, error)
	// ConvertHTML converts an HTML fragment, as returned by a job board's
	// API or scraped from a posting.
	ConvertHTML(ctx context.Context, html string) (Document, error)
}

// Document is a converted document.
//
// It is a struct rather than a bare string so that what a converter can
// recover from a source — today the markdown, including any hyperlinks
// that were annotations rather than text — can grow without changing every
// caller.
type Document struct {
	Markdown string
}
