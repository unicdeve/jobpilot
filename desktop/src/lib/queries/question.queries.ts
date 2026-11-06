import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { ApplicationQuestion } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useQuestions loads the questions the candidate noted on an application,
/// each with its suggestion and its answer's draft flags.
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
}

export function useAddQuestion(opts?: MutationOpts<ApplicationQuestion, AddQuestionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, label }: AddQuestionVars) => api.addQuestion(applicationId, label),
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

export function useDeleteQuestion(opts?: MutationOpts<void, QuestionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ questionId }: QuestionVars) => api.deleteQuestion(questionId),
    ...opts,
    onSuccess: (result, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.questions(variables.applicationId) });
      opts?.onSuccess?.(result, variables, ...rest);
    },
  });
}
