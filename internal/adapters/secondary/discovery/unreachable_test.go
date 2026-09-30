//go:build integration

package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// Every connector tells a source it could not reach apart from one that
// answered badly, because discovery reports the first as the user being
// offline and the second as the source being broken.
func TestSources_ReportUnreachableOnlyWhenNothingAnswered(t *testing.T) {
	t.Parallel()

	connectors := map[string]func(baseURL string) ports.JobSource{
		"arbeitnow":      func(u string) ports.JobSource { return NewArbeitnowSource(u) },
		"remotive":       func(u string) ports.JobSource { return NewRemotiveSource(u) },
		"remoteok":       func(u string) ports.JobSource { return NewRemoteOKSource(u) },
		"himalayas":      func(u string) ports.JobSource { return NewHimalayasSource(u) },
		"hackernews":     func(u string) ports.JobSource { return NewHackerNewsSource(u) },
		"weworkremotely": func(u string) ports.JobSource { return NewWeWorkRemotelySource(u) },
		"jobicy":         func(u string) ports.JobSource { return NewJobicySource(u) },
	}
	// A country, so Jobicy reads its locations first, as a real search does.
	criteria := domain.SearchCriteria{Keywords: "engineer", WorkFrom: "NG"}

	for name, newSource := range connectors {
		t.Run(name+" with nothing listening is unreachable", func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.NotFoundHandler())
			baseURL := server.URL
			server.Close() // the port now refuses connections

			_, err := newSource(baseURL).Search(context.Background(), criteria)

			require.Error(t, err)
			assert.ErrorIs(t, err, ports.ErrSourceUnreachable)
		})

		t.Run(name+" answering with an error status is not unreachable", func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			t.Cleanup(server.Close)

			_, err := newSource(server.URL).Search(context.Background(), criteria)

			require.Error(t, err)
			assert.NotErrorIs(t, err, ports.ErrSourceUnreachable)
		})
	}
}
