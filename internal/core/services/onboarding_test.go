package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// newTestOnboardingService constructs an OnboardingService with fakes.
func newTestOnboardingService(candRepo *fakeCandidateRepository, llm *fakeLLMProvider) *OnboardingService {
	resumeSvc := NewResumeService(newFakeResumeRepository(), llm, &fakeDocumentConverter{})
	return NewOnboardingService(candRepo, resumeSvc, NewSavedSearchService(newFakeSavedSearchRepository()), llm, &fakeLocalStorage{dir: "/data"})
}

type fakeLocalStorage struct {
	dir string
	err error
}

func (f *fakeLocalStorage) Prepare(context.Context) (string, error) { return f.dir, f.err }

func TestOnboardingService_SetupEnvironment(t *testing.T) {
	t.Parallel()
	svc := newTestOnboardingService(newFakeCandidateRepository(), &fakeLLMProvider{})

	dir, err := svc.SetupEnvironment(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "/data", dir)
}

func TestOnboardingService_ConfigureLLM(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		svc := newTestOnboardingService(newFakeCandidateRepository(), &fakeLLMProvider{})
		assert.NoError(t, svc.ConfigureLLM(context.Background()))
	})

	t.Run("connectivity error is wrapped", func(t *testing.T) {
		t.Parallel()
		llm := &fakeLLMProvider{connectivityErr: errors.New("connection refused")}
		svc := newTestOnboardingService(newFakeCandidateRepository(), llm)

		err := svc.ConfigureLLM(context.Background())

		assert.ErrorContains(t, err, "LLM connectivity test failed")
	})
}

func TestOnboardingService_InitializeProfile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		candRepo := newFakeCandidateRepository()
		svc := newTestOnboardingService(candRepo, &fakeLLMProvider{})
		profile := &domain.Candidate{FirstName: "Taiwo"}

		err := svc.InitializeProfile(context.Background(), profile)

		require.NoError(t, err)
		assert.NotEmpty(t, profile.ID, "the generated ID should propagate back to the caller's struct")
	})

	t.Run("save error is wrapped", func(t *testing.T) {
		t.Parallel()
		candRepo := newFakeCandidateRepository()
		candRepo.saveErr = errors.New("disk full")
		svc := newTestOnboardingService(candRepo, &fakeLLMProvider{})

		err := svc.InitializeProfile(context.Background(), &domain.Candidate{FirstName: "Taiwo"})

		assert.ErrorContains(t, err, "failed to save initial profile")
	})
}

func TestOnboardingService_InitializeProfile_RefusesAnUnknownTimeZone(t *testing.T) {
	t.Parallel()
	svc := NewOnboardingService(newFakeCandidateRepository(), nil, nil, nil, nil)

	err := svc.InitializeProfile(context.Background(), &domain.Candidate{FirstName: "Ada", TimeZone: "Lagos"})
	assert.ErrorIs(t, err, ErrInvalidTimeZone)

	profile := &domain.Candidate{FirstName: "Ada", TimeZone: "africa/lagos"}
	require.NoError(t, svc.InitializeProfile(context.Background(), profile))
	assert.Equal(t, "Africa/Lagos", profile.TimeZone)
}
