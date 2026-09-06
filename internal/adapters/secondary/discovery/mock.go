// Package discovery provides ports.JobSource implementations used to find
// job postings, including a MockJobSource for development and testing.
package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// MockJobSource is a fake job source for development and testing.
type MockJobSource struct{}

// NewMockJobSource constructs a MockJobSource.
func NewMockJobSource() *MockJobSource {
	return &MockJobSource{}
}

// ID returns the mock source's unique identifier.
func (m *MockJobSource) ID() string {
	return "mock-source"
}

// Name returns the mock source's human-readable name.
func (m *MockJobSource) Name() string {
	return "Mock Job Board"
}

// Capabilities reports which features the mock source supports.
func (m *MockJobSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{
		CanSearch:       true,
		CanGetJob:       true,
		CanAuthenticate: false,
	}
}

// Search returns fake jobs, optionally filtered by criteria.Keywords.
func (m *MockJobSource) Search(_ context.Context, criteria domain.SearchCriteria) ([]domain.Job, error) {
	// Return a few fake jobs based on keywords
	jobs := []domain.Job{
		{
			ID:                  "mock-1",
			CanonicalURL:        "https://example.com/jobs/1",
			SourceID:            m.ID(),
			SourceJobID:         "1",
			Title:               "Software Engineer (Mock)",
			CompanyName:         "MockCorp",
			CompanyURL:          "https://mockcorp.com",
			DescriptionMarkdown: "A great software engineering role at MockCorp. We use Go and SQLite!",
			Locations:           []string{"Remote", "Lagos"},
			RemoteType:          "remote",
			EmploymentType:      "full-time",
			SalaryMin:           100000,
			SalaryMax:           150000,
			SalaryCurrency:      "USD",
			SalaryPeriod:        "yearly",
			Skills:              []string{"Go", "SQLite", "Docker"},
			PostedAt:            time.Now().AddDate(0, 0, -2),
			DiscoveredAt:        time.Now(),
		},
		{
			ID:                  "mock-2",
			CanonicalURL:        "https://example.com/jobs/2",
			SourceID:            m.ID(),
			SourceJobID:         "2",
			Title:               "Backend Developer (Mock)",
			CompanyName:         "FakeTech",
			CompanyURL:          "https://faketech.com",
			DescriptionMarkdown: "Looking for a backend pro. Must love Go.",
			Locations:           []string{"New York"},
			RemoteType:          "onsite",
			EmploymentType:      "full-time",
			SalaryMin:           120000,
			SalaryMax:           180000,
			SalaryCurrency:      "USD",
			SalaryPeriod:        "yearly",
			Skills:              []string{"Go", "PostgreSQL"},
			PostedAt:            time.Now().AddDate(0, 0, -5),
			DiscoveredAt:        time.Now(),
		},
	}

	// Simple keyword filter
	if criteria.Keywords == "" {
		return jobs, nil
	}

	var filtered []domain.Job
	for _, j := range jobs {
		if containsKeyword(j.Title, criteria.Keywords) || containsKeyword(j.DescriptionMarkdown, criteria.Keywords) {
			filtered = append(filtered, j)
		}
	}
	return filtered, nil
}

// GetJob returns the fake job whose canonical URL matches url.
func (m *MockJobSource) GetJob(ctx context.Context, url string) (*domain.Job, error) {
	// For simplicity, return mock-1 for any URL containing "1" and mock-2 for "2"
	jobs, err := m.Search(ctx, domain.SearchCriteria{})
	if err != nil {
		return nil, err
	}

	if url == "https://example.com/jobs/1" && len(jobs) > 0 {
		return &jobs[0], nil
	}
	if url == "https://example.com/jobs/2" && len(jobs) > 1 {
		return &jobs[1], nil
	}
	return nil, fmt.Errorf("job not found in mock source: %s", url)
}

func containsKeyword(text, keyword string) bool {
	// In a real app, use a more robust search. For mock, simple contains is fine.
	return len(keyword) == 0 || strings.Contains(strings.ToLower(text), strings.ToLower(keyword))
}
