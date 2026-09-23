import { open } from "@tauri-apps/plugin-dialog";

/**
 * The formats the document converter can actually read.
 *
 * PDF is extracted by JobPilot itself; the rest go through Go MarkItDown
 * (ADR-0016). Anything else fails at conversion, so offering it in the
 * picker only lets someone choose a file that cannot work — which is what
 * happened before this list existed.
 *
 * Legacy `.doc` is deliberately absent: it is a different format from
 * `.docx` and nothing here reads it.
 */
export const RESUME_EXTENSIONS = ["pdf", "docx", "txt", "md", "markdown", "html", "htm"];

/**
 * Asks for a resume file, offering only the formats that convert.
 *
 * Returns null when the user cancels.
 */
export async function pickResumeFile(): Promise<string | null> {
  const path = await open({
    multiple: false,
    title: "Select a resume file",
    filters: [{ name: "Documents", extensions: RESUME_EXTENSIONS }],
  });
  if (!path || Array.isArray(path)) return null;
  return path;
}
