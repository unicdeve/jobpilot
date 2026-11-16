import { expect, test } from 'bun:test';
import {
  isStandingKey,
  relevantKeptAnswers,
  standingQuestionFor,
  standingQuestions,
  type PersonalAnswer,
} from './personal-answer.ts';

// The case is the Go test's (internal/core/domain/personal_answer_test.go).
test('relevant kept answers', () => {
  const at = (minutes: number) => Date.UTC(2026, 8, 29, 9, minutes);
  const kept: PersonalAnswer[] = [
    {
      id: 'why-old',
      question: 'Why do you want to work at Initech?',
      answer: 'Old',
      updatedAt: at(1),
    },
    {
      id: 'why-new',
      question: 'Why do you want to work at Globex?',
      answer: 'New',
      updatedAt: at(5),
    },
    {
      id: 'platform',
      question: 'Why do you want to join our platform team?',
      answer: 'Platform',
      updatedAt: at(2),
    },
    {
      id: 'review',
      question: 'How do you approach code review?',
      answer: 'Reviews',
      updatedAt: at(3),
    },
    {
      id: 'blank',
      question: 'Why do you want to work on our platform?',
      answer: '  ',
      updatedAt: at(9),
    },
  ];
  const chosen = relevantKeptAnswers(kept, 'Why do you want to work on our platform team?');
  expect(chosen.map((a) => a.id)).toEqual(['platform', 'why-new', 'why-old']);
  expect(relevantKeptAnswers(kept, 'What is your notice period?')).toEqual([]);
  expect(relevantKeptAnswers(kept, 'Why do you?')).toEqual([]);
});

test('standing questions', () => {
  expect(standingQuestions.length).toBe(5);
  expect(isStandingKey('values')).toBe(true);
  expect(isStandingKey('kept')).toBe(false);
  expect(standingQuestionFor('known_for')?.question).toBe('What do colleagues come to you for?');
  expect(standingQuestionFor('nope')).toBeUndefined();
});
