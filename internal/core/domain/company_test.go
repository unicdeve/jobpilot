package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBoard(t *testing.T) {
	t.Parallel()

	greenhouse := func(name string) Board { return Board{Provider: ProviderGreenhouse, Name: name} }
	for address, want := range map[string]Board{
		"https://boards.greenhouse.io/gitlab":                                      greenhouse("gitlab"),
		"https://job-boards.greenhouse.io/gitlab/jobs/8556658002":                  greenhouse("gitlab"),
		"job-boards.greenhouse.io/gitlab":                                          greenhouse("gitlab"),
		"  https://www.boards.greenhouse.io/gitlab?gh_src=abc  ":                   greenhouse("gitlab"),
		"https://boards.greenhouse.io/embed/job_board?for=grafanalabs":             greenhouse("grafanalabs"),
		"https://boards.greenhouse.io/embed/job_app?for=gitlab&token=8556658002":   greenhouse("gitlab"),
		"https://job-boards.eu.greenhouse.io/n26/jobs/1":                           {Provider: ProviderGreenhouse, Region: RegionEU, Name: "n26"},
		"https://jobs.lever.co/spotify/2193db3f-77c5-43b8-b030-8f92c9882bf1/apply": {Provider: ProviderLever, Name: "spotify"},
		"https://jobs.eu.lever.co/acme":                                            {Provider: ProviderLever, Region: RegionEU, Name: "acme"},
		"https://jobs.ashbyhq.com/clickup/1e8c7f3d-3cd6-4393-b17f-6a858f480ba9":    {Provider: ProviderAshby, Name: "clickup"},
		"HTTPS://JOBS.ASHBYHQ.COM/ClickUp":                                         {Provider: ProviderAshby, Name: "ClickUp"},
		"https://jobs.ashbyhq.com/The%20Browser%20Company":                         {Provider: ProviderAshby, Name: "The Browser Company"},
	} {
		got, err := ParseBoard(address)
		require.NoError(t, err, address)
		assert.Equal(t, want, got, address)
	}

	for _, address := range []string{
		"", "gitlab", "https://gitlab.com/jobs", "https://www.linkedin.com/company/gitlab",
		"https://boards.greenhouse.io/", "https://boards.greenhouse.io/embed/job_board",
		"https://jobs.lever.co", "https://jobs.ashbyhq.com/..", "https://greenhouse.io.evil.example/gitlab",
		"https://jobs.ashbyhq.com/%3Cscript%3E",
	} {
		_, err := ParseBoard(address)
		assert.ErrorIs(t, err, ErrUnsupportedAddress, "%q", address)
	}
}

func TestBoard_URL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://job-boards.greenhouse.io/gitlab", Board{Provider: ProviderGreenhouse, Name: "gitlab"}.URL())
	assert.Equal(t, "https://job-boards.eu.greenhouse.io/n26", Board{Provider: ProviderGreenhouse, Region: RegionEU, Name: "n26"}.URL())
	assert.Equal(t, "https://jobs.lever.co/spotify", Board{Provider: ProviderLever, Name: "spotify"}.URL())
	assert.Equal(t, "https://jobs.eu.lever.co/acme", Board{Provider: ProviderLever, Region: RegionEU, Name: "acme"}.URL())
	assert.Equal(t, "https://jobs.ashbyhq.com/The%20Browser%20Company", Board{Provider: ProviderAshby, Name: "The Browser Company"}.URL())

	for _, board := range []Board{
		{Provider: ProviderGreenhouse, Region: RegionEU, Name: "n26"},
		{Provider: ProviderAshby, Name: "The Browser Company"},
	} {
		parsed, err := ParseBoard(board.URL())
		require.NoError(t, err)
		assert.Equal(t, board, parsed, "a board's address names the board")
	}
}

func TestBoard_Same(t *testing.T) {
	t.Parallel()

	gitlab := Board{Provider: ProviderGreenhouse, Name: "gitlab"}
	assert.True(t, gitlab.Same(Board{Provider: ProviderGreenhouse, Name: "GitLab"}))
	assert.False(t, gitlab.Same(Board{Provider: ProviderGreenhouse, Region: RegionEU, Name: "gitlab"}))
	assert.False(t, gitlab.Same(Board{Provider: ProviderLever, Name: "gitlab"}))
}

func TestBoardOf(t *testing.T) {
	t.Parallel()

	own, ok := BoardOf(Job{CanonicalURL: "https://jobs.lever.co/spotify/2193db3f"})
	require.True(t, ok)
	assert.Equal(t, "spotify", own.Name)

	linked, ok := BoardOf(Job{
		CanonicalURL:        "https://news.ycombinator.com/item?id=1",
		DescriptionMarkdown: "Acme | Remote\n\nSee https://acme.example/careers or apply at [Ashby](https://jobs.ashbyhq.com/acme/123/application).",
	})
	require.True(t, ok, "a Hacker News posting links to the company's page")
	assert.Equal(t, Board{Provider: ProviderAshby, Name: "acme"}, linked)

	_, ok = BoardOf(Job{CanonicalURL: "https://remoteok.com/remote-jobs/1", DescriptionMarkdown: "Apply by email."})
	assert.False(t, ok)
}

func TestCleanCompanyName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "GitLab Inc.", CleanCompanyName("  GitLab\n\tInc. "))
	long := CleanCompanyName(stringOf('é', 150))
	assert.Len(t, []rune(long), maxCompanyName)
}

func stringOf(r rune, n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = r
	}
	return string(runes)
}

func TestBoard_DefaultName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Grafanalabs", Board{Name: "grafanalabs"}.DefaultName())
	assert.Equal(t, "Cockroach labs", Board{Name: "cockroach-labs"}.DefaultName())
}

func TestOpenAcrossContinents(t *testing.T) {
	t.Parallel()

	read := func(locations ...string) Workable {
		job := Job{Locations: locations, RemoteType: RemoteTypeRemote}
		ReadSignals(&job)
		return job.Workable
	}
	assert.True(t, OpenAcrossContinents(read("Remote, Global")))
	assert.True(t, OpenAcrossContinents(read("Home based - EMEA")), "EMEA spans Europe, Africa and Asia")
	assert.True(t, OpenAcrossContinents(read("Remote, Canada", "Remote, United Kingdom")))
	assert.True(t, OpenAcrossContinents(read("AMER")), "the Americas are two continents")
	assert.False(t, OpenAcrossContinents(read("Remote (United States | Canada)")), "one continent")
	assert.False(t, OpenAcrossContinents(read("Remote France; Remote Germany; Remote UK")))
	assert.False(t, OpenAcrossContinents(read("HQ")), "unknown")
}
