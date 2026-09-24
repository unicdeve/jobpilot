package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchTemplateCatalogIsWellFormed(t *testing.T) {
	groups := SearchTemplateGroups()
	require.NotEmpty(t, groups)

	groupIDs := map[string]bool{}
	templateIDs := map[string]bool{}

	for _, group := range groups {
		assert.NotEmpty(t, group.Name, "group %q needs a name", group.ID)
		assert.NotEmpty(t, group.Summary, "group %q needs a summary", group.ID)
		assert.False(t, groupIDs[group.ID], "duplicate group ID %q", group.ID)
		groupIDs[group.ID] = true

		assert.GreaterOrEqual(t, len(group.Templates), 2,
			"group %q holds one template: a group exists to offer alternatives", group.ID)

		for _, template := range group.Templates {
			assert.NotEmpty(t, template.Name, "template %q needs a name", template.ID)
			assert.NotEmpty(t, template.Summary, "template %q needs a summary", template.ID)
			assert.False(t, templateIDs[template.ID], "duplicate template ID %q", template.ID)
			templateIDs[template.ID] = true

			assert.False(t, template.Criteria.IsEmpty(),
				"template %q sets no criterion, so adopting it would import every posting on every board", template.ID)
		}
	}
}

func TestEverySearchTemplateCanBeFoundByID(t *testing.T) {
	for _, group := range SearchTemplateGroups() {
		for _, want := range group.Templates {
			got, err := FindSearchTemplate(want.ID)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		}
	}

	_, err := FindSearchTemplate("no-such-template")
	assert.Error(t, err)
}

// TestNoSearchTemplateSearchesOnATinyKeyword guards the rule that cost the
// most to learn: Keywords is matched against the whole job description, as a
// substring, so a short word is supplied by ordinary prose. Measured over 654
// live postings, keywords "go" matched 426 of them — on "go-to", "goals" and
// "Django" — and "rust" matched 151, on "trust".
//
// Title terms are exempt. A title is a few words long, and a short term there
// is ANDed with a longer one: "back end" is safe because "back" carries it.
func TestNoSearchTemplateSearchesOnATinyKeyword(t *testing.T) {
	const shortest = 5

	for _, group := range SearchTemplateGroups() {
		for _, template := range group.Templates {
			for _, term := range SearchTerms(template.Criteria.Keywords) {
				assert.GreaterOrEqual(t, len(term), shortest,
					"template %q searches descriptions for %q, which is short enough that ordinary prose supplies it",
					template.ID, term)
			}
		}
	}
}

// decoys are real job titles paired with the kind of description prose that
// made a candidate criterion misfire while the catalog was being measured.
// None of them is the sort of vacancy any template claims to find, so no
// template may match one.
var decoys = []Job{
	{
		Title:               "Senior Sales Engineer, Enterprise",
		DescriptionMarkdown: "Demo our platform to enterprise buyers and close the technical evaluation.",
	},
	{
		Title:               "Senior Contract Recruiter - Go to Market (GTM)",
		DescriptionMarkdown: "Help us go to market. You will own hiring goals across the go-to-market organisation.",
	},
	{
		Title:               "Teamleiter (m/w/d) im Finanzvertrieb",
		DescriptionMarkdown: "Sie leiten ein Team im Finanzvertrieb.",
	},
	{
		Title:               "(Junior) Design Manager (all genders) – Luxury Retail",
		DescriptionMarkdown: "Support the luxury retail design team with seasonal collections.",
	},
	{
		Title:               "Chemical Laboratory Assistant / Lab Technician (f/m/d) Analytics & Synthesis",
		DescriptionMarkdown: "You will run each reaction in the synthesis lab and record the analytics.",
	},
	{
		Title:               "Senior Account Executive, Enterprise",
		DescriptionMarkdown: "Build trust with enterprise buyers and hit your goals. Our customers are engineering teams.",
	},
	{
		Title:               "Quality Dispense Technician North",
		DescriptionMarkdown: "Maintain dispense quality across venues in the north.",
	},
}

func TestNoSearchTemplateMatchesADecoy(t *testing.T) {
	for _, group := range SearchTemplateGroups() {
		for _, template := range group.Templates {
			for _, decoy := range decoys {
				assert.False(t, template.Criteria.Matches(decoy),
					"template %q matches %q, which is not the kind of job it offers",
					template.ID, decoy.Title)
			}
		}
	}
}

// TestATitleTemplateReadsTheTitleNotTheProse pins the reason the catalog
// prefers Title. "Senior Backend Engineer - Cruise" is a genuine backend
// vacancy whose title hides "ui" inside "Cruise" and whose description says
// almost nothing: the backend template finds it anyway, and no design
// template is fooled by the spelling.
func TestATitleTemplateReadsTheTitleNotTheProse(t *testing.T) {
	job := Job{
		Title:               "Senior Backend Engineer - Cruise",
		DescriptionMarkdown: "Join the cruise booking team.",
	}

	backend, err := FindSearchTemplate("backend-engineer")
	require.NoError(t, err)
	assert.True(t, backend.Criteria.Matches(job))

	for _, group := range SearchTemplateGroups() {
		if group.ID != "design" {
			continue
		}
		for _, template := range group.Templates {
			assert.False(t, template.Criteria.Matches(job),
				"design template %q matched a backend vacancy", template.ID)
		}
	}
}

// TestAStackTemplateAlsoMatchesAPassingMention records what the templates in
// the "engineering by stack" group cost, so that it is a known price rather
// than a surprise.
//
// Those templates search the job description, because boards almost never
// put a stack in the title — measured over 654 live postings, "typescript"
// appeared in no title at all and "golang" in three. A description, though,
// mentions a technology for many reasons, and a sales role at an
// infrastructure company lists the whole stack. Matches cannot tell "this
// job is Go" from "this page says Go", and no wording of the criteria makes
// it able to.
//
// The group says so in its summary. If this test ever fails, matching has
// become more precise and that summary should be revisited.
func TestAStackTemplateAlsoMatchesAPassingMention(t *testing.T) {
	salesRole := Job{
		Title:               "Senior Sales Engineer, Enterprise",
		DescriptionMarkdown: "Demo our platform to customers running Kubernetes, and speak credibly about Golang and Terraform.",
	}

	goTemplate, err := FindSearchTemplate("go-engineer")
	require.NoError(t, err)
	assert.True(t, goTemplate.Criteria.Matches(salesRole),
		"the Go template no longer reaches a sales posting that merely lists Golang")

	// The title-driven templates are not fooled by the same posting.
	for _, id := range []string{"backend-engineer", "frontend-engineer", "fullstack-engineer"} {
		template, err := FindSearchTemplate(id)
		require.NoError(t, err)
		assert.False(t, template.Criteria.Matches(salesRole),
			"template %q matched a sales posting", id)
	}
}

// realPostings are titles taken verbatim from a live run across all five job
// sources, with a minimal description supplying only the technology a
// template looks for. Every template must find at least one: a template that
// matches nothing is a menu entry that quietly wastes the user's run.
var realPostings = []Job{
	{Title: "Backend Engineer - Agentic AI (m/w/d)"},
	{Title: "Senior Backend Engineer, Python/Django (f/d/m): Path to CTO", DescriptionMarkdown: "Python and Django."},
	{Title: "Senior Front end Engineer I"},
	{Title: "Frontend Engineer", DescriptionMarkdown: "We build with React and TypeScript."},
	{Title: "Frontend Web Application Developer", DescriptionMarkdown: "TypeScript throughout."},
	{Title: "Full-Stack Engineer (f/m/d)"},
	{Title: "Senior Fullstack Developer (m/w/d)"},
	{Title: "Junior Full Stack Developer (All Genders)", DescriptionMarkdown: "React on the front."},
	{Title: "Principal Software Engineer - Data Lakes"},
	{Title: "Senior Independent Software Developer"},
	{Title: "Platform Engineer", DescriptionMarkdown: "We run Kubernetes."},
	{Title: "Cloud / Platform Engineer"},
	{Title: "Senior/Staff Golang Engineer (EMEA)", DescriptionMarkdown: "Golang services."},
	{Title: "Senior DevOps Engineer", DescriptionMarkdown: "Terraform and Kubernetes."},
	{Title: "Site Reliability Engineer"},
	{Title: "ML Infrastructure Engineer"},
	{Title: "Staff Data Engineer"},
	{Title: "Senior Data Scientist"},
	{Title: "Data Analyst"},
	{Title: "Machine Learning Engineer"},
	{Title: "Senior Product Manager, Orchestration"},
	{Title: "Product owner - Politiques sociales H/F"},
	{Title: "Senior Program Manager, Community Investments"},
	{Title: "Senior Manager, Product Marketing"},
	{Title: "Lead Product Designer"},
	{Title: "Senior Product Designer (M/F/*)", DescriptionMarkdown: "You will run usability sessions and own the work in Figma."},
	{Title: "QA Engineer"},
	{Title: "Senior Software Development Engineer in Test (SDET)"},
	{Title: "Security Engineer"},
	{Title: "Cybersecurity Principal"},
}

func TestEverySearchTemplateFindsARealPosting(t *testing.T) {
	for _, group := range SearchTemplateGroups() {
		for _, template := range group.Templates {
			matched := false
			for _, job := range realPostings {
				if template.Criteria.Matches(job) {
					matched = true
					break
				}
			}
			assert.True(t, matched,
				"template %q (%s) matches none of the real postings",
				template.ID, describeTemplateCriteria(template.Criteria))
		}
	}
}

func describeTemplateCriteria(c SearchCriteria) string {
	var parts []string
	if c.Keywords != "" {
		parts = append(parts, "keywords="+c.Keywords)
	}
	if c.Title != "" {
		parts = append(parts, "title="+c.Title)
	}
	return strings.Join(parts, " ")
}
