import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { ApplicationAnswer, ApplicationQuestion, PersonalAnswer, QuestionType } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useQuestions loads the questions an application form asks.
export function useQuestions(applicationId: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationQuestion[]>({
    queryKey: keys.applications.questions(applicationId ?? ""),
    queryFn: applicationId ? () => api.listQuestions(applicationId) : skipToken,
  });
}

interface AddQuestionVars {
  applicationId: string;
  label: string;
  type: QuestionType;
  required: boolean;
}

export function useAddQuestion(opts?: MutationOpts<ApplicationQuestion, AddQuestionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, label, type, required }: AddQuestionVars) =>
      api.addQuestion(applicationId, label, type, required),
    ...opts,
    onSuccess: (question, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.questions(variables.applicationId) });
      opts?.onSuccess?.(question, variables, ...rest);
    },
  });
}

interface QuestionVars {
  questionId: string;
  applicationId: string;
}

export function useResolveQuestion(opts?: MutationOpts<ApplicationQuestion, QuestionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ questionId }: QuestionVars) => api.resolveQuestion(questionId),
    ...opts,
    onSuccess: (question, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.questions(variables.applicationId) });
      opts?.onSuccess?.(question, variables, ...rest);
    },
  });
}

type AnswerVars = QuestionVars & { answer: string };

export function useSetQuestionAnswer(opts?: MutationOpts<ApplicationQuestion, AnswerVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ questionId, answer }: AnswerVars) => api.setQuestionAnswer(questionId, answer),
    ...opts,
    onSuccess: (question, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.questions(variables.applicationId) });
      opts?.onSuccess?.(question, variables, ...rest);
    },
  });
}

/// usePersonalQuestions reads the questions asked before a letter for one
/// application, without asking a model; usePreparePersonalQuestions adds
/// the ones the model proposes.
export function usePersonalQuestions(applicationId: string, enabled: boolean) {
  const api = useApi();
  return useQuery<PersonalAnswer[]>({
    queryKey: keys.applications.personalQuestions(applicationId),
    queryFn: () => api.listPersonalQuestions(applicationId),
    enabled,
  });
}

export function usePreparePersonalQuestions(opts?: MutationOpts<PersonalAnswer[], string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (applicationId: string) => api.preparePersonalQuestions(applicationId),
    ...opts,
    onSuccess: (questions, applicationId, ...rest) => {
      queryClient.setQueryData(keys.applications.personalQuestions(applicationId), questions);
      opts?.onSuccess?.(questions, applicationId, ...rest);
    },
  });
}

interface SavePersonalAnswersVars {
  applicationId: string;
  answers: ApplicationAnswer[];
}

export function useSavePersonalAnswers(opts?: MutationOpts<PersonalAnswer[], SavePersonalAnswersVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, answers }: SavePersonalAnswersVars) =>
      api.savePersonalAnswers(applicationId, answers),
    ...opts,
    onSuccess: (questions, variables, ...rest) => {
      queryClient.setQueryData(keys.applications.personalQuestions(variables.applicationId), questions);
      opts?.onSuccess?.(questions, variables, ...rest);
    },
  });
}
