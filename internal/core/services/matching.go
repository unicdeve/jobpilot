package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unicdev/job-agent/internal/core/domain"
	"github.com/unicdev/job-agent/internal/core/ports"
)

// MatchingService evaluates the fit between a candidate and a job.
type MatchingService struct {
	matchRepo    ports.MatchRepository
	jobRepo      ports.JobRepository
	candRepo     ports.CandidateRepository
	resumeRepo   ports.ResumeRepository
	llmProvider  ports.LLMProvider
}

func NewMatchingService(
	matchRepo ports.MatchRepository,
	jobRepo ports.JobRepository,
	candRepo ports.CandidateRepository,
	resumeRepo ports.ResumeRepository,
	llmProvider ports.LLMProvider,
) *MatchingService {
	return &MatchingService{
		matchRepo:    matchRepo,
		jobRepo:      jobRepo,
		candRepo:     candRepo,
		resumeRepo:   resumeRepo,
		llmProvider:  llmProvider,
	}
}

// AnalyzeJobForCandidate performs a full match analysis for a specific job.
func (s *MatchingService) AnalyzeJobForCandidate(ctx context.Context, candidateID, jobID string) (*domain.JobMatch, error) {
	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch job: %w", err)
	}

	candidate, err := s.candRepo.GetByID(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch candidate: %w", err)
	}

	prompt := s.buildPrompt(candidate, job)

	req := ports.Request{
		SystemPrompt: "You are an expert technical recruiter. Analyze the fit between a candidate and a job posting. Output ONLY valid JSON.",
		UserPrompt:   prompt,
		Temperature:  0.1,
	}

	resp, err := s.llmProvider.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	match, err := s.parseMatchResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse match response: %w", err)
	}

	match.JobID = jobID
	match.CandidateID = candidateID
	match.ModelProvider = "ollama"
	match.ModelName = "llama3"

	if err := s.matchRepo.Save(ctx, match); err != nil {
		return nil, fmt.Errorf("failed to save match: %w", err)
	}

	return match, nil
}

// ListMatches retrieves all match analyses for a candidate.
func (s *MatchingService) ListMatches(ctx context.Context, candidateID string) ([]*domain.JobMatch, error) {
	return s.matchRepo.ListByCandidate(ctx, candidateID)
}

func (s *MatchingService) buildPrompt(c *domain.Candidate, j *domain.Job) string {
	var sb strings.Builder
	sb.WriteString("Analyze the following job and candidate. Output the result in JSON format with these keys: score (0-100), confidence (0.0-1.0), strengths (list), gaps (list), concerns (list), matched_skills (list), missing_skills (list), rationale (string).\n\n")

	sb.WriteString("### Job Posting\n")
	sb.WriteString(fmt.Sprintf("Title: %s\nCompany: %s\nDescription: %s\n", j.Title, j.CompanyName, j.DescriptionMarkdown))

	sb.WriteString("\n### Candidate Profile\n")
	sb.WriteString(fmt.Sprintf("Summary: %s\nSkills: %s\nExperience: %s\n", c.Summary, c.Skills, c.Experiences))

	return sb.String()
}

func (s *MatchingService) parseMatchResponse(content string) (*domain.JobMatch, error) {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var match domain.JobMatch
	if err := json.Unmarshal([]byte(content), &match); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w", err)
	}

	if match.Score < 0 || match.Score > 100 {
		return nil, fmt.Errorf("invalid match score: %d", match.Score)
	}

	return &match, nil
}
