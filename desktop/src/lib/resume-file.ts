import { type ChosenFile, chooseFile } from "./platform";

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
const RESUME_EXTENSIONS = ["pdf", "docx", "txt", "md", "markdown", "html", "htm"];

/**
 * Asks for a resume file, offering only the formats that convert.
 *
 * Returns null when the user cancels.
 */
export function pickResumeFile(): Promise<ChosenFile | null> {
  return chooseFile("Select a resume file", RESUME_EXTENSIONS);
}
