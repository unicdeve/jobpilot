package domain

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/unicdev/jobpilot/internal/core/domain/geo"
)

//go:generate go run ../../tools/genstarter -seed ../../tools/genstarter/seed.txt -out starter_companies_gen.go

// BoardProvider is an applicant-tracking system whose hiring pages JobPilot
// reads (docs/02_features/17_company_watchlists.md D6).
type BoardProvider string

// The providers JobPilot reads.
const (
	ProviderGreenhouse      BoardProvider = "greenhouse"
	ProviderLever           BoardProvider = "lever"
	ProviderAshby           BoardProvider = "ashby"
	ProviderWorkable        BoardProvider = "workable"
	ProviderSmartRecruiters BoardProvider = "smartrecruiters"
)

// ProviderName is the provider as the product writes it.
func (p BoardProvider) ProviderName() string {
	switch p {
	case ProviderGreenhouse:
		return "Greenhouse"
	case ProviderLever:
		return "Lever"
	case ProviderAshby:
		return "Ashby"
	case ProviderWorkable:
		return "Workable"
	case ProviderSmartRecruiters:
		return "SmartRecruiters"
	}
	return string(p)
}

// RegionEU is the region of a board hosted on a provider's EU servers.
const RegionEU = "eu"

// Board is one company's hiring page on a provider (ADR-0024 §1).
type Board struct {
	Provider BoardProvider `json:"provider"`
	// Region is empty, or RegionEU for a board on the provider's EU
	// servers, which is a different account from one of the same name.
	Region string `json:"region,omitempty"`
	// Name is Greenhouse's board token, Lever's site, Ashby's job board
	// name, Workable's account or SmartRecruiters' company identifier: the
	// first segment of the page's path, or Workable's subdomain.
	Name string `json:"name"`
}

// Same reports whether two boards are one: a provider's names are not case
// sensitive.
func (b Board) Same(other Board) bool {
	return b.Provider == other.Provider && b.Region == other.Region && strings.EqualFold(b.Name, other.Name)
}

// URL is the board's hiring page.
func (b Board) URL() string {
	name := url.PathEscape(b.Name)
	eu := ""
	if b.Region == RegionEU {
		eu = "eu."
	}
	switch b.Provider {
	case ProviderGreenhouse:
		return "https://job-boards." + eu + "greenhouse.io/" + name
	case ProviderLever:
		return "https://jobs." + eu + "lever.co/" + name
	case ProviderAshby:
		return "https://jobs.ashbyhq.com/" + name
	case ProviderWorkable:
		return "https://apply.workable.com/" + name + "/"
	case ProviderSmartRecruiters:
		return "https://careers.smartrecruiters.com/" + name
	}
	return ""
}

// DefaultName is what a company is called when its board does not say:
// its board's name, with its first letter capitalised.
func (b Board) DefaultName() string {
	name := strings.NewReplacer("-", " ", "_", " ").Replace(b.Name)
	runes := []rune(name)
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

// ErrUnsupportedAddress is returned for an address that is not a hiring page
// JobPilot reads. It lists what is.
var ErrUnsupportedAddress = errors.New("not a hiring page JobPilot can read; " +
	"use a Greenhouse (boards.greenhouse.io/…, job-boards.greenhouse.io/…), " +
	"Lever (jobs.lever.co/…), Ashby (jobs.ashbyhq.com/…), Workable (apply.workable.com/…) " +
	"or SmartRecruiters (careers.smartrecruiters.com/…, jobs.smartrecruiters.com/…) address")

// ErrWorkablePostingAddress is returned for a Workable posting's short
// address, apply.workable.com/j/{code}, which does not name the account the
// posting is on. Workable redirects it to the account's own address, so it
// is named by following that redirect (ADR-0026 §3).
var ErrWorkablePostingAddress = errors.New("a Workable posting's short address does not name its company")

// boardName is what a board's name may hold. A name is one path segment, so
// this refuses nothing a provider issues and anything that is not one.
var boardName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} ._-]*$`)

// ParseBoard reads the board an address names: the address of a hiring
// page, or of any posting on it (ADR-0024 §1). It fetches nothing.
func ParseBoard(address string) (Board, error) {
	address = strings.TrimSpace(address)
	if address != "" && !strings.Contains(address, "://") {
		address = "https://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return Board{}, fmt.Errorf("%q is %w", address, ErrUnsupportedAddress)
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })

	if host == "apply.workable.com" && len(segments) > 0 && segments[0] == "j" {
		return Board{}, fmt.Errorf("%q: %w", address, ErrWorkablePostingAddress)
	}
	board, ok := boardAt(host, segments, parsed.Query())
	if !ok || !boardName.MatchString(board.Name) {
		return Board{}, fmt.Errorf("%q is %w", address, ErrUnsupportedAddress)
	}
	return board, nil
}

// boardAt is the board a supported host and path name.
func boardAt(host string, segments []string, query url.Values) (Board, bool) {
	first := ""
	if len(segments) > 0 {
		first = segments[0]
	}
	switch host {
	case "boards.greenhouse.io", "job-boards.greenhouse.io":
		return greenhouseBoard("", first, query)
	case "boards.eu.greenhouse.io", "job-boards.eu.greenhouse.io":
		return greenhouseBoard(RegionEU, first, query)
	case "jobs.lever.co":
		return Board{Provider: ProviderLever, Name: first}, first != ""
	case "jobs.eu.lever.co":
		return Board{Provider: ProviderLever, Region: RegionEU, Name: first}, first != ""
	case "jobs.ashbyhq.com":
		return Board{Provider: ProviderAshby, Name: first}, first != ""
	case "apply.workable.com":
		return Board{Provider: ProviderWorkable, Name: first}, first != ""
	case "careers.smartrecruiters.com", "jobs.smartrecruiters.com":
		return Board{Provider: ProviderSmartRecruiters, Name: first}, first != "" && !smartRecruitersPaths[strings.ToLower(first)]
	}
	// A Workable account's older address is its own subdomain:
	// sumble-inc.workable.com, which Workable redirects to
	// apply.workable.com/sumble-inc/.
	if account, ok := strings.CutSuffix(host, ".workable.com"); ok && !strings.Contains(account, ".") && !workableHosts[account] {
		return Board{Provider: ProviderWorkable, Name: account}, true
	}
	return Board{}, false
}

// workableHosts are Workable's own subdomains, which are not accounts:
// jobs.workable.com is Workable's search across every account.
var workableHosts = map[string]bool{
	"apply": true, "jobs": true, "www": true, "help": true, "resources": true,
	"api": true, "app": true, "careers": true, "status": true, "developers": true,
}

// smartRecruitersPaths are the first segments of SmartRecruiters' own pages
// on its job hosts, which are not companies: a posting's referral link is
// jobs.smartrecruiters.com/external-referrals/company/{Company}/….
var smartRecruitersPaths = map[string]bool{
	"external-referrals": true, "oneclick-ui": true, "sr-jobs": true, "ni": true,
	"search": true, "public": true, "embed": true,
}

// greenhouseBoard reads a Greenhouse path, whose embedded forms name the
// board in their query: /embed/job_board?for=gitlab.
func greenhouseBoard(region, first string, query url.Values) (Board, bool) {
	name := first
	if first == "embed" {
		name = query.Get("for")
	}
	return Board{Provider: ProviderGreenhouse, Region: region, Name: name}, name != ""
}

// addressInText finds the addresses in a description that could be a
// supported board's.
var addressInText = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:(?:job-)?boards(?:\.eu)?\.greenhouse\.io|jobs(?:\.eu)?\.lever\.co|jobs\.ashbyhq\.com|[a-z0-9-]+\.workable\.com|(?:careers|jobs)\.smartrecruiters\.com)/[^\s)\]>"'<]+`)

// BoardOf is the board a job's posting is on: its own address's, or else
// the first supported address in its description, which is where a Hacker
// News posting links to the company's page.
func BoardOf(job Job) (Board, bool) {
	if board, err := ParseBoard(job.CanonicalURL); err == nil {
		return board, true
	}
	for _, address := range addressInText.FindAllString(job.DescriptionMarkdown, -1) {
		if board, err := ParseBoard(address); err == nil {
			return board, true
		}
	}
	return Board{}, false
}

// BoardCheck is how reading a watched company's board last went.
type BoardCheck string

// The outcomes of a check. A company not yet checked has none.
const (
	BoardCheckOK BoardCheck = "ok"
	// BoardCheckGone is a board the provider says does not exist: the
	// company moved provider, or closed its page.
	BoardCheckGone        BoardCheck = "gone"
	BoardCheckUnreachable BoardCheck = "unreachable"
	BoardCheckFailed      BoardCheck = "failed"
)

// maxCompanyName bounds a company's name, which comes from a page JobPilot
// does not control, or from the candidate.
const maxCompanyName = 100

// CleanCompanyName is a name as it is stored: one line, trimmed, and at most
// maxCompanyName characters.
func CleanCompanyName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if runes := []rune(name); len(runes) > maxCompanyName {
		name = strings.TrimSpace(string(runes[:maxCompanyName]))
	}
	return name
}

// WatchedCompany is a company whose hiring page the candidate asked JobPilot
// to read (ADR-0024 §2).
type WatchedCompany struct {
	ID    string `json:"id"`
	Board Board  `json:"board"`
	Name  string `json:"name"`
	// URL is the board's hiring page.
	URL string `json:"url"`
	// AddedFrom is the address the candidate gave.
	AddedFrom string    `json:"added_from"`
	AddedAt   time.Time `json:"added_at"`
	// CheckedAt is when the board was last read, and Check how that went.
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Check     BoardCheck `json:"check"`
	// OpenRoles is how many postings the board held when last read, before
	// any search's criteria.
	OpenRoles int `json:"open_roles"`
}

// BoardContents is what reading a board found.
type BoardContents struct {
	Jobs []Job
	// FetchedAt is when the provider answered: earlier than now for an
	// answer kept from an earlier read.
	FetchedAt time.Time
	// Undescribed is a board whose provider lists its postings without
	// their descriptions, which ports.BoardReader.DescribeJobs reads one
	// posting at a time: SmartRecruiters' (ADR-0026 §2).
	Undescribed bool
}

// StarterCompany is a suggestion from the starter list, with what was
// measured to put it there (docs/02_features/17_company_watchlists.md D2).
type StarterCompany struct {
	Board Board  `json:"board"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	// MeasuredOn is the day the board was read, as YYYY-MM-DD.
	MeasuredOn string `json:"measured_on"`
	// OpenRoles is how many postings the board held then, and Qualifying
	// how many of them could be done from anywhere or from countries on
	// more than one continent.
	OpenRoles  int `json:"open_roles"`
	Qualifying int `json:"qualifying_roles"`
}

// StarterCompanies is the starter list, generated by internal/tools/genstarter.
func StarterCompanies() []StarterCompany {
	companies := make([]StarterCompany, len(starterCompanies))
	for i, company := range starterCompanies {
		company.URL = company.Board.URL()
		companies[i] = company
	}
	return companies
}

// OpenAcrossContinents reports whether a posting earns its company a place
// on the starter list: it can be done from anywhere, or from countries on
// more than one continent (docs/02_features/17_company_watchlists.md).
func OpenAcrossContinents(workable Workable) bool {
	switch workable.Scope {
	case WorkableAnywhere:
		return true
	case WorkableListed:
		continents := make(map[string]bool)
		for _, code := range workable.Countries {
			if country, ok := geo.CountryByCode(code); ok {
				continents[country.Continent] = true
			}
		}
		return len(continents) > 1
	}
	return false
}
