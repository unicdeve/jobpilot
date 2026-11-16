// A resume on the device (ADR-0037), from the Go core's domain.Resume and ResumeService's import checks.
// It holds the resume's Markdown text and metadata only. Turning a file into that text (PDF extraction, ADR-0016;
// MarkItDown) happens before this module and reaches core through a converter port, never a library.
export interface Resume {
  id: string; candidateId: string; name: string;
  // Where the file was imported from, or '' when the client never had a path.
  originalFilePath: string;
  markdownContent: string;
  // JSON text a reader of the resume recorded; '' when none.
  parsedMetadata: string;
  createdAt: number; updatedAt: number;
}

export type NewResume = { ok: true; resume: Resume } | { ok: false; error: string };

// A client may send a path from any platform; the resume is named after the file either way.
export function resumeName(fileName: string): string {
  const parts = fileName.replace(/\\/g, '/').split('/').filter((p) => p !== '');
  return (parts[parts.length - 1] ?? '').trim();
}

export function newResume(input: { id: string; candidateId: string; fileName: string; markdown: string; originalFilePath?: string; parsedMetadata?: string }, now: number): NewResume {
  const name = resumeName(input.fileName);
  if (name === '' || name === '.') return { ok: false, error: 'The file has no name.' };
  if (input.markdown.trim() === '') return { ok: false, error: `${name} has no text.` };
  return {
    ok: true,
    resume: {
      id: input.id, candidateId: input.candidateId, name, originalFilePath: input.originalFilePath ?? '',
      markdownContent: input.markdown, parsedMetadata: input.parsedMetadata ?? '', createdAt: now, updatedAt: now,
    },
  };
}
