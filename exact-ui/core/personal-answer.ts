// A port of the Go core's personal-answer rules (internal/core/domain/personal_answer.go), ADR-0037.
// Only the candidate writes an answer (ADR-0034 section 5): nothing here writes one.
// Pure: no Exact, network or storage dependency, no clock.

export type PersonalAnswerKey = 'looking_for' | 'values' | 'proudest_work' | 'known_for' | 'career_story' | 'kept';

export interface PersonalAnswer {
  id: string;
  candidateId?: string;
  key?: PersonalAnswerKey;
  question: string;
  answer: string;
  // Epoch milliseconds; only its order is used.
  updatedAt: number;
}

export interface StandingQuestion {
  key: PersonalAnswerKey;
  question: string;
  // What a good answer covers, for whoever is asking.
  hint: string;
}

// The About you questions, in the order they are asked.
export const standingQuestions: readonly StandingQuestion[] = [
  { key: 'looking_for', question: 'What are you looking for in your next role?', hint: 'The work you want to do, the problems you want to be near, what would make you glad you moved.' },
  { key: 'values', question: 'What do you care about in your work, and in the people you do it with?', hint: 'What you hold yourself to, and what makes a team one you want to be part of.' },
  { key: 'proudest_work', question: 'Tell us about the work you are proudest of.', hint: 'What the problem was, what you did yourself, and what changed because of it.' },
  { key: 'known_for', question: 'What do colleagues come to you for?', hint: 'The things people bring to you, in your own words.' },
  { key: 'career_story', question: 'How did you get into this work, and where are you taking it?', hint: 'Include any change of direction or gap you would like explained your way.' },
];

export const isStandingKey = (key: string): boolean => standingQuestions.some((q) => q.key === key);

export const standingQuestionFor = (key: string): StandingQuestion | undefined => standingQuestions.find((q) => q.key === key);

const maxKeptAnswersUsed = 3;

const stopWords = new Set([
  'a', 'an', 'and', 'are', 'about', 'as', 'at', 'be', 'by', 'can', 'describe', 'do', 'does',
  'for', 'from', 'have', 'how', 'i', 'if', 'in', 'is', 'it', 'me', 'of', 'on', 'or', 'our',
  'please', 'share', 'tell', 'that', 'the', 'this', 'to', 'us', 'was', 'we', 'were', 'what', 'when',
  'where', 'which', 'who', 'why', 'with', 'would', 'you', 'your',
]);

// Go's len(word): UTF-8 bytes, not UTF-16 units.
const utf8Length = (s: string): number => {
  let n = 0;
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4;
  }
  return n;
};

// strings.FieldsFunc(strings.ToLower(text), not letter and not digit)
const significantWords = (text: string): Set<string> => {
  const words = new Set<string>();
  for (const word of text.toLowerCase().split(/[^\p{L}\p{Nd}]+/u)) {
    if (word !== '' && utf8Length(word) > 2 && !stopWords.has(word)) words.add(word);
  }
  return words;
};

// The kept answers closest to a question: those whose question shares the most words with it,
// newest first on a tie, none that share no word, never an empty answer, at most three.
export function relevantKeptAnswers(kept: PersonalAnswer[], label: string): PersonalAnswer[] {
  const want = significantWords(label);
  const candidates: { answer: PersonalAnswer; shared: number; index: number }[] = [];
  kept.forEach((answer, index) => {
    let shared = 0;
    for (const word of significantWords(answer.question)) if (want.has(word)) shared++;
    if (shared > 0 && answer.answer.trim() !== '') candidates.push({ answer, shared, index });
  });
  // The index keeps the sort stable on every engine.
  candidates.sort((a, b) => b.shared - a.shared || b.answer.updatedAt - a.answer.updatedAt || a.index - b.index);
  return candidates.slice(0, maxKeptAnswersUsed).map((c) => c.answer);
}
