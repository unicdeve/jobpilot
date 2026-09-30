//go:build integration

package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// weWorkRemotelyFeedXML is shaped like the feed measured on 2026-09-30:
// the country list, when present, is narrower than the region.
func weWorkRemotelyFeedXML(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>We Work Remotely</title>` + strings.Join(items, "") + `</channel></rss>`
}

const (
	wwrWorldwide = `<item>
		<title>Twikey: Senior Java Developer</title>
		<region>Anywhere in the World</region>
		<skills>Java, PostgreSQL, and Spring</skills>
		<type>Full-Time</type>
		<description>&lt;p&gt;Build &lt;strong&gt;payments&lt;/strong&gt;.&lt;/p&gt;</description>
		<pubDate>Thu, 17 Sep 2026 10:51:23 +0000</pubDate>
		<expires_at>Sat, 17 Oct 2026 10:51:23 +0000</expires_at>
		<guid>https://weworkremotely.com/remote-jobs/twikey-senior-java-developer</guid>
		<link>https://weworkremotely.com/remote-jobs/twikey-senior-java-developer</link>
	</item>`
	wwrUSOnly = `<item>
		<title>Glean: Application Security Engineer</title>
		<region>Anywhere in the World</region>
		<country>🇺🇸 United States of America</country>
		<type>Contract</type>
		<description>Remote.</description>
		<guid>https://weworkremotely.com/remote-jobs/glean-application-security-engineer</guid>
		<link>https://weworkremotely.com/remote-jobs/glean-application-security-engineer</link>
	</item>`
)

func TestWeWorkRemotelySource_Search(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// Every feed carries the worldwide posting, as the categories
		// overlap; one also carries the US-only posting.
		items := []string{wwrWorldwide}
		if r.URL.Path == "/remote-back-end-programming-jobs.rss" {
			items = append(items, wwrUSOnly)
		}
		_, _ = w.Write([]byte(weWorkRemotelyFeedXML(items...)))
	}))
	t.Cleanup(server.Close)

	source := NewWeWorkRemotelySource(server.URL)
	jobs, err := source.Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err)
	require.Len(t, jobs, 2, "a posting in several category feeds is one job")
	assert.Equal(t, int32(len(weWorkRemotelyCategories)), requests.Load())

	byTitle := map[string]domain.Job{jobs[0].Title: jobs[0], jobs[1].Title: jobs[1]}
	java := byTitle["Senior Java Developer"]
	assert.Equal(t, "Twikey", java.CompanyName)
	assert.Equal(t, "weworkremotely", java.SourceID)
	assert.Equal(t, "https://weworkremotely.com/remote-jobs/twikey-senior-java-developer", java.CanonicalURL,
		"the job keeps its We Work Remotely URL, which the terms ask for")
	assert.Equal(t, []string{"Anywhere in the World"}, java.Locations)
	assert.Equal(t, domain.RemoteTypeRemote, java.RemoteType)
	assert.Equal(t, "full-time", java.EmploymentType)
	assert.Equal(t, []string{"Java", "PostgreSQL", "Spring"}, java.Skills)
	assert.False(t, java.PostedAt.IsZero())
	assert.False(t, java.ExpiresAt.IsZero())
	assert.Contains(t, java.DescriptionMarkdown, "payments")
	assert.NotContains(t, java.DescriptionMarkdown, "<strong>")

	security := byTitle["Application Security Engineer"]
	assert.Equal(t, []string{"🇺🇸 United States of America"}, security.Locations,
		"the employer's countries are narrower than the board's region, and truer")
	domain.ReadSignals(&security)
	assert.Equal(t, []string{"US"}, security.Workable.Countries)

	_, err = source.Search(context.Background(), domain.SearchCriteria{Title: "other"})
	require.NoError(t, err)
	assert.Equal(t, int32(len(weWorkRemotelyCategories)), requests.Load(), "within the hour, the feeds are not asked again")
}

func TestWeWorkRemotelySource_OneFeedFailing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/remote-programming-jobs.rss" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(weWorkRemotelyFeedXML(wwrWorldwide)))
	}))
	t.Cleanup(server.Close)

	jobs, err := NewWeWorkRemotelySource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	require.NoError(t, err, "the other feeds still answered")
	assert.Len(t, jobs, 1)
}

func TestWeWorkRemotelySource_NoFeedAnswering(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	_, err := NewWeWorkRemotelySource(server.URL).Search(context.Background(), domain.SearchCriteria{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "403")
	assert.False(t, errors.Is(err, ports.ErrSourceUnreachable), "a feed that answered with an error was reached")
}

func TestNoFeedAnswered(t *testing.T) {
	t.Parallel()

	unreachable := fmt.Errorf("offline: %w", ports.ErrSourceUnreachable)
	failed := errors.New("status 500")

	assert.NoError(t, noFeedAnswered([]error{unreachable, nil}))
	assert.ErrorIs(t, noFeedAnswered([]error{unreachable, unreachable}), ports.ErrSourceUnreachable)
	err := noFeedAnswered([]error{unreachable, failed})
	assert.Equal(t, failed, err, "one feed reached means the connection is not the reason")
}
