package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func searchableSource(id string, jobs []domain.Job, err error) *fakeJobSource {
	return &fakeJobSource{
		id:           id,
		capabilities: ports.SourceCapabilities{CanSearch: true},
		searchResult: jobs,
		searchErr:    err,
	}
}

func TestJobDiscoveryService_Search(t *testing.T) {
	t.Parallel()

	t.Run("new job is saved", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		source := searchableSource("mock", []domain.Job{{CanonicalURL: "https://acme.example/job-1", Title: "Backend Engineer"}}, nil)
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		jobs, err := svc.Search(context.Background(), domain.SearchCriteria{Keywords: "backend"})

		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "Backend Engineer", jobs[0].Title)
		assert.Equal(t, 1, jobRepo.saveCalls)
	})

	t.Run("existing job by canonical URL is returned, not re-saved", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		existing := &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1", Title: "Existing title"}
		require.NoError(t, jobRepo.Save(context.Background(), existing))

		source := searchableSource("mock", []domain.Job{{CanonicalURL: "https://acme.example/job-1", Title: "New title"}}, nil)
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		jobs, err := svc.Search(context.Background(), domain.SearchCriteria{})

		require.NoError(t, err)
		require.Len(t, jobs, 1)
		assert.Equal(t, "job-1", jobs[0].ID)
		assert.Equal(t, "Existing title", jobs[0].Title)
		assert.Equal(t, 1, jobRepo.saveCalls, "the duplicate should not trigger a second save")
	})

	t.Run("a failing source is skipped, others still contribute results", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		failing := searchableSource("broken", nil, errors.New("source down"))
		working := searchableSource("mock", []domain.Job{{CanonicalURL: "https://acme.example/job-1"}}, nil)
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{failing, working})

		jobs, err := svc.Search(context.Background(), domain.SearchCriteria{})

		require.NoError(t, err)
		assert.Len(t, jobs, 1)
	})

	t.Run("a source without CanSearch is not queried", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		noSearch := &fakeJobSource{id: "no-search", capabilities: ports.SourceCapabilities{CanSearch: false}}
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{noSearch})

		jobs, err := svc.Search(context.Background(), domain.SearchCriteria{})

		require.NoError(t, err)
		assert.Empty(t, jobs)
	})

	t.Run("save error propagates", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		jobRepo.saveErr = errors.New("disk full")
		source := searchableSource("mock", []domain.Job{{CanonicalURL: "https://acme.example/job-1"}}, nil)
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		_, err := svc.Search(context.Background(), domain.SearchCriteria{})

		assert.ErrorContains(t, err, "failed to save discovered job")
	})
}

func TestJobDiscoveryService_ImportJob(t *testing.T) {
	t.Parallel()

	t.Run("uses a CanGetJob-capable source and saves the job", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		source := &fakeJobSource{
			id:           "mock",
			capabilities: ports.SourceCapabilities{CanGetJob: true},
			getJobResult: &domain.Job{CanonicalURL: "https://acme.example/job-1", Title: "Backend Engineer"},
		}
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		job, err := svc.ImportJob(context.Background(), "https://acme.example/job-1")

		require.NoError(t, err)
		assert.Equal(t, "Backend Engineer", job.Title)
		assert.Equal(t, 1, jobRepo.saveCalls)
	})

	t.Run("dedups against an existing job", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		existing := &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1"}
		require.NoError(t, jobRepo.Save(context.Background(), existing))
		source := &fakeJobSource{
			id:           "mock",
			capabilities: ports.SourceCapabilities{CanGetJob: true},
			getJobResult: &domain.Job{CanonicalURL: "https://acme.example/job-1"},
		}
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		job, err := svc.ImportJob(context.Background(), "https://acme.example/job-1")

		require.NoError(t, err)
		assert.Equal(t, "job-1", job.ID)
		assert.Equal(t, 1, jobRepo.saveCalls, "the duplicate should not trigger a second save")
	})

	t.Run("no capable source errors", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		source := &fakeJobSource{id: "mock", capabilities: ports.SourceCapabilities{CanGetJob: false}}
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		_, err := svc.ImportJob(context.Background(), "https://acme.example/job-1")

		assert.ErrorContains(t, err, "no available source supports importing jobs by URL")
	})

	t.Run("source error propagates", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		source := &fakeJobSource{
			id:           "mock",
			capabilities: ports.SourceCapabilities{CanGetJob: true},
			getJobErr:    errors.New("not found upstream"),
		}
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{source})

		_, err := svc.ImportJob(context.Background(), "https://acme.example/job-1")

		assert.ErrorContains(t, err, "failed to import job from source")
	})

	t.Run("a failing capable source doesn't block a later capable source from resolving the URL", func(t *testing.T) {
		t.Parallel()
		jobRepo := newFakeJobRepository()
		failing := &fakeJobSource{
			id:           "arbeitnow",
			capabilities: ports.SourceCapabilities{CanGetJob: true},
			getJobErr:    errors.New("not this source's job"),
		}
		working := &fakeJobSource{
			id:           "remotive",
			capabilities: ports.SourceCapabilities{CanGetJob: true},
			getJobResult: &domain.Job{CanonicalURL: "https://acme.example/job-1", Title: "Backend Engineer"},
		}
		svc := NewJobDiscoveryService(jobRepo, []ports.JobSource{failing, working})

		job, err := svc.ImportJob(context.Background(), "https://acme.example/job-1")

		require.NoError(t, err)
		assert.Equal(t, "Backend Engineer", job.Title)
	})
}
