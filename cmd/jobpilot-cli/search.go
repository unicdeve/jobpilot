package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/domain/geo"
)

func newSearchCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "search", Short: "Discover jobs"}

	runCmd := &cobra.Command{
		Use:   "run [keywords]",
		Short: "Discover jobs, from a saved search or ad-hoc keywords",
		Long: "Runs discovery against every configured job source.\n\n" +
			"With no arguments it runs your default saved search. Pass --saved to run a\n" +
			"different one, or give keywords for a one-off search. Discovery always needs\n" +
			"criteria: the public job boards do no filtering of their own, so a search with\n" +
			"nothing set would import every posting on every board.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			saved, err := cmd.Flags().GetString("saved")
			if err != nil {
				return err
			}

			var result *domain.DiscoveryResult
			switch {
			case saved != "" && len(args) > 0:
				return fmt.Errorf("give either --saved or keywords, not both")

			case len(args) > 0:
				result, err = client.SearchJobs(ctx, domain.SearchCriteria{Keywords: strings.Join(args, " ")})

			default:
				search, resolveErr := resolveSavedSearch(ctx, client, saved)
				if resolveErr != nil {
					return resolveErr
				}
				fmt.Printf("Running saved search %q…\n", search.Name)
				result, err = client.RunSavedSearch(ctx, search.ID)
			}
			if err != nil {
				return fmt.Errorf("searching for jobs: %w", err)
			}

			if unanswered := result.Unanswered(); len(unanswered) > 0 {
				fmt.Printf("⚠️  %s — results may be incomplete.\n", describeUnansweredSources(unanswered))
			}
			if failing := failingParts(result.Sources); len(failing) > 0 {
				fmt.Printf("⚠️  Couldn't read the hiring page of %s. See 'company list'.\n", joinNames(failing))
			}

			if len(result.Jobs) == 0 {
				fmt.Println("No jobs found matching your criteria.")
				return nil
			}

			fmt.Printf("Found %d jobs (%d new):\n", len(result.Jobs), result.New)
			for i, j := range result.Jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", i+1, j.Title, j.CompanyName, j.ID)
			}
			return nil
		},
	}
	runCmd.Flags().String("saved", "", "Name or ID of a saved search to run (default: your default saved search)")
	cmd.AddCommand(runCmd)

	cmd.AddCommand(newSavedSearchCmd(client))
	cmd.AddCommand(newSearchTemplateCmd(client))

	// "list" reads stored discovery history and contacts no job source, so
	// it stays usable offline. It previously ran an empty live search, which
	// re-queried every source and could not work without internet access.
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List previously discovered jobs from local history",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()

			keywords, err := cmd.Flags().GetString("keywords")
			if err != nil {
				return err
			}
			limit, err := cmd.Flags().GetInt("limit")
			if err != nil {
				return err
			}
			offset, err := cmd.Flags().GetInt("offset")
			if err != nil {
				return err
			}

			source, err := cmd.Flags().GetString("source")
			if err != nil {
				return err
			}
			remote, err := cmd.Flags().GetString("remote")
			if err != nil {
				return err
			}
			postedWithin, err := cmd.Flags().GetInt("posted-within-days")
			if err != nil {
				return err
			}
			if postedWithin < 0 {
				return fmt.Errorf("--posted-within-days must not be negative")
			}

			notInterested, err := cmd.Flags().GetBool("not-interested")
			if err != nil {
				return err
			}

			workFrom, relocation, err := workFromFromFlags(cmd)
			if err != nil {
				return err
			}

			filter := domain.JobFilter{
				Keywords: keywords, SourceID: source, RemoteType: remote,
				WorkFrom: workFrom, IncludeRelocation: relocation,
			}
			if notInterested {
				filter.Interest = domain.JobsNotInterested
			}
			if postedWithin > 0 {
				filter.PostedSince = time.Now().AddDate(0, 0, -postedWithin)
			}

			page, err := client.ListJobs(ctx, filter, limit, offset)
			if err != nil {
				return fmt.Errorf("listing jobs: %w", err)
			}

			if len(page.Jobs) == 0 {
				switch {
				case offset > 0:
					fmt.Printf("No jobs at offset %d (%d stored in total).\n", offset, page.Total)
				case notInterested:
					fmt.Println("No jobs are marked not interested.")
				default:
					// Jobs ruled out are stored but not listed, so an empty list
					// is not the same as nothing discovered.
					ruledOut, err := client.ListJobs(ctx, domain.JobFilter{Interest: domain.JobsNotInterested}, 1, 0)
					if err != nil {
						return fmt.Errorf("listing jobs: %w", err)
					}
					if ruledOut.Total > 0 {
						fmt.Printf("No jobs to list. %d marked not interested; --not-interested lists them.\n", ruledOut.Total)
						return nil
					}
					fmt.Println("No jobs discovered yet. Run 'search run <keywords>' to discover some.")
				}
				return nil
			}

			heading := "Discovered jobs"
			if notInterested {
				heading = "Jobs marked not interested"
			}
			fmt.Printf("%s %d-%d of %d:\n", heading, offset+1, offset+len(page.Jobs), page.Total)
			for i, j := range page.Jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", offset+i+1, j.Title, j.CompanyName, j.ID)
			}

			if shown := offset + len(page.Jobs); shown < page.Total {
				fmt.Printf("\n%d more — use --offset %d to see the next page.\n", page.Total-shown, shown)
			}
			return nil
		},
	}
	listCmd.Flags().String("keywords", "", "Only list jobs whose title or company matches these keywords")
	listCmd.Flags().String("source", "", "Only list jobs from this source, e.g. arbeitnow")
	listCmd.Flags().String("remote", "", "Only list remote, hybrid or onsite jobs")
	listCmd.Flags().Int("posted-within-days", 0, "Only list jobs posted in the last this many days")
	listCmd.Flags().Bool("not-interested", false, "List only the jobs marked not interested, most recently first")
	workFromFlags(listCmd)
	listCmd.Flags().Int("limit", 0, "Maximum jobs to list (default 50, maximum 200)")
	listCmd.Flags().Int("offset", 0, "Skip this many jobs before listing")
	cmd.AddCommand(listCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "show [id]",
		Short: "Show a stored job in full",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			job, err := client.GetJob(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("getting job: %w", err)
			}

			fmt.Printf("%s at %s\n", job.Title, job.CompanyName)
			fmt.Printf("ID:       %s\n", job.ID)
			fmt.Printf("URL:      %s\n", job.CanonicalURL)
			fmt.Printf("Location: %s\n", strings.Join(job.Locations, ", "))
			fmt.Printf("Open to:  %s\n", describeWorkable(job.Workable))
			if job.Workable.Note != "" {
				fmt.Printf("          The posting says: %q\n", job.Workable.Note)
			}
			if line := describeOffer("Relocate:", job.Relocation); line != "" {
				fmt.Println(line)
			}
			if line := describeOffer("Visa:    ", job.VisaSponsorship); line != "" {
				fmt.Println(line)
			}
			if line := describeHours(job.Hours); line != "" {
				fmt.Println(line)
			}
			fmt.Printf("Type:     %s · %s\n", job.RemoteType, job.EmploymentType)
			if len(job.Skills) > 0 {
				fmt.Printf("Skills:   %s\n", strings.Join(job.Skills, ", "))
			}
			if job.DescriptionMarkdown != "" {
				fmt.Printf("\n%s\n", job.DescriptionMarkdown)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "import [url]",
		Short: "Import a job from a specific URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			url := args[0]

			job, err := client.ImportJob(ctx, url)
			if err != nil {
				return fmt.Errorf("importing job: %w", err)
			}
			fmt.Printf("✅ Job imported: %s at %s (ID: %s)\n", job.Title, job.CompanyName, job.ID)
			return nil
		},
	})

	return cmd
}

// criteriaFlags registers the search criteria flags shared by the commands
// that build a SearchCriteria, so every surface offers the same set.
func criteriaFlags(cmd *cobra.Command) {
	cmd.Flags().String("keywords", "", "Words that must all appear in the title or description")
	cmd.Flags().String("title", "", "Words that must all appear in the job title")
	cmd.Flags().String("location", "", "Words that must all appear in the job's locations")
	cmd.Flags().String("remote", "", "Remote type to require, e.g. remote, hybrid, onsite")
	cmd.Flags().String("employment", "", "Employment type to require, e.g. full_time, contract")
	cmd.Flags().Float64("salary-min", 0, "Lowest acceptable advertised salary (jobs without a salary are kept)")
	cmd.Flags().Float64("salary-max", 0, "Highest acceptable advertised salary (jobs without a salary are kept)")
	cmd.Flags().StringSlice("exclude-company", nil, "Company to exclude; repeatable")
	workFromFlags(cmd)
	cmd.Flags().Bool("fit-hours", false, "Leave out jobs whose required working hours your profile's time zone cannot work")
	cmd.Flags().Int("hours-shift", 0, fmt.Sprintf("With --fit-hours, how many hours you would move your working day for a job (0 to %d)", domain.MaxHoursShift))
}

// workFromFlags registers where the candidate works from, for the commands
// that search and the one that lists stored jobs (ADR-0023 §4).
func workFromFlags(cmd *cobra.Command) {
	cmd.Flags().String("work-from", "", "The country you work from, e.g. Nigeria or NG: keeps jobs open there or anywhere, and jobs that do not say")
	cmd.Flags().Bool("include-relocation", false, "With --work-from, also keep jobs elsewhere that offer or mention relocation or visa sponsorship")
}

// workFromFromFlags reads the flags workFromFlags registers, resolving a
// country as a person writes it to its ISO code.
func workFromFromFlags(cmd *cobra.Command) (string, bool, error) {
	workFrom, err := cmd.Flags().GetString("work-from")
	if err != nil {
		return "", false, err
	}
	relocation, err := cmd.Flags().GetBool("include-relocation")
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(workFrom) == "" {
		if relocation {
			return "", false, fmt.Errorf("--include-relocation needs --work-from: relocation is from somewhere")
		}
		return "", false, nil
	}
	code, ok := geo.ResolveCountry(workFrom)
	if !ok {
		return "", false, fmt.Errorf("--work-from %q is not a country JobPilot knows; try its name or its two-letter code", workFrom)
	}
	return code, relocation, nil
}

// criteriaFromFlags reads the flags registered by criteriaFlags.
func criteriaFromFlags(cmd *cobra.Command) (domain.SearchCriteria, error) {
	var c domain.SearchCriteria
	var err error

	if c.Keywords, err = cmd.Flags().GetString("keywords"); err != nil {
		return c, err
	}
	if c.Title, err = cmd.Flags().GetString("title"); err != nil {
		return c, err
	}
	if c.Location, err = cmd.Flags().GetString("location"); err != nil {
		return c, err
	}
	if c.RemotePreference, err = cmd.Flags().GetString("remote"); err != nil {
		return c, err
	}
	if c.EmploymentType, err = cmd.Flags().GetString("employment"); err != nil {
		return c, err
	}
	if c.SalaryMin, err = cmd.Flags().GetFloat64("salary-min"); err != nil {
		return c, err
	}
	if c.SalaryMax, err = cmd.Flags().GetFloat64("salary-max"); err != nil {
		return c, err
	}
	if c.ExcludedCompanies, err = cmd.Flags().GetStringSlice("exclude-company"); err != nil {
		return c, err
	}
	if c.WorkFrom, c.IncludeRelocation, err = workFromFromFlags(cmd); err != nil {
		return c, err
	}
	if c.FitHours, err = cmd.Flags().GetBool("fit-hours"); err != nil {
		return c, err
	}
	if c.HoursShift, err = cmd.Flags().GetInt("hours-shift"); err != nil {
		return c, err
	}
	if c.HoursShift != 0 && !c.FitHours {
		return c, fmt.Errorf("--hours-shift needs --fit-hours: it is how far you would move your day for a job's hours")
	}
	return c, nil
}

// describeHours is one line on the working hours a job asks for, with the
// sentence it was read from, or empty when it asks for none (ADR-0025).
func describeHours(hours domain.WorkingHours) string {
	if !hours.Known() {
		return ""
	}
	verb := "required"
	if hours.Status == domain.HoursPreferred {
		verb = "preferred"
	}
	return fmt.Sprintf("Hours:    %s, %s — %q", hours.Window(), verb, hours.Evidence)
}

// describeWorkable says where a job can be done from, as ADR-0023 reads it.
func describeWorkable(w domain.Workable) string {
	switch w.Scope {
	case domain.WorkableAnywhere:
		return "anywhere"
	case domain.WorkableListed:
		return strings.Join(w.Places, ", ")
	}
	return "not stated"
}

// describeOffer is one line on relocation or visa sponsorship, with the
// sentence it was read from, or empty when the posting does not mention it.
func describeOffer(label string, offer domain.Offer) string {
	var status string
	switch offer.Status {
	case domain.OfferOffered:
		status = "offered"
	case domain.OfferMentioned:
		status = "mentioned"
	case domain.OfferDeclined:
		status = "not offered"
	default:
		return ""
	}
	return fmt.Sprintf("%s %s — %q", label, status, offer.Evidence)
}

// describeCriteria renders the criteria that are set, for listing commands.
func describeCriteria(c domain.SearchCriteria) string {
	parts := make([]string, 0, 8)
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, label+"="+value)
		}
	}

	add("keywords", c.Keywords)
	add("title", c.Title)
	add("location", c.Location)
	add("remote", c.RemotePreference)
	add("employment", c.EmploymentType)
	if c.SalaryMin > 0 {
		parts = append(parts, fmt.Sprintf("salary>=%.0f", c.SalaryMin))
	}
	if c.SalaryMax > 0 {
		parts = append(parts, fmt.Sprintf("salary<=%.0f", c.SalaryMax))
	}
	if len(c.ExcludedCompanies) > 0 {
		parts = append(parts, "excluding "+strings.Join(c.ExcludedCompanies, ", "))
	}
	if c.WorkFrom != "" {
		from := "work from " + geo.CountryName(c.WorkFrom)
		if c.IncludeRelocation {
			from += ", or relocate"
		}
		parts = append(parts, from)
	}
	if c.FitHours {
		hours := "fit my hours"
		if c.HoursShift > 0 {
			hours += fmt.Sprintf(", shifting up to %dh", c.HoursShift)
		}
		parts = append(parts, hours)
	}
	return strings.Join(parts, "  ")
}

// describeUnansweredSources names the sources a search did not hear from,
// grouped by why: "Couldn't reach Remotive and RemoteOK; Himalayas answered
// with an error".
func describeUnansweredSources(sources []domain.SourceOutcome) string {
	var unreachable, failed []string
	for _, source := range sources {
		if source.Status == domain.SourceStatusUnreachable {
			unreachable = append(unreachable, source.SourceName)
		} else {
			failed = append(failed, source.SourceName)
		}
	}

	var parts []string
	if len(unreachable) > 0 {
		parts = append(parts, "Couldn't reach "+joinNames(unreachable))
	}
	if len(failed) > 0 {
		parts = append(parts, joinNames(failed)+" answered with an error")
	}
	return strings.Join(parts, "; ")
}

// failingParts names the parts of the sources that answered that did not:
// the watched companies whose pages failed (ADR-0024 §5).
func failingParts(sources []domain.SourceOutcome) []string {
	var failing []string
	for _, source := range sources {
		failing = append(failing, source.Failing...)
	}
	return failing
}
