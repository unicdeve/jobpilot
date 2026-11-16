// Agent runs and their events (ADR-0037), ported from internal/core/domain/agent_run.go. Pure: times are epoch ms.
export type AgentRunStatus = 'running' | 'completed' | 'failed';
export type RunTrigger = 'manual' | 'schedule';
export const agentTaskScoutRun = 'scout_run';
export const policyAllowed = 'allowed';
export const policyDenied = 'denied';

export interface AgentRun {
  id: string; scoutId?: string; trigger?: RunTrigger; taskType: string; status: AgentRunStatus;
  startedAt: number; completedAt?: number; error?: string;
}
export interface AgentEvent {
  id: string; runId: string; action: string; tool: string; status: string;
  inputSummary?: string; outputSummary?: string; policyResult?: string; durationMs: number; createdAt: number;
}
export interface AgentRunFilter { taskType?: string; status?: AgentRunStatus; limit?: number; offset?: number }
export interface AgentRunSummary extends AgentRun { scoutName?: string; eventCount: number }
export interface AgentRunPage { runs: AgentRunSummary[]; total: number }
export interface AgentRunDetail { run: AgentRunSummary; events: AgentEvent[] }

export const inProgress = (s: AgentRunStatus): boolean => s === 'running';
export const validAgentRunStatus = (s: string): s is AgentRunStatus => s === 'running' || s === 'completed' || s === 'failed';
export const validAgentTaskType = (t: string): boolean => t === agentTaskScoutRun;
