package domain

import "fmt"

// A SearchTemplate is a ready-made saved search: criteria that are known to
// work, under a name the user recognizes.
//
// Templates exist because SearchCriteria.Matches is unforgiving in ways that
// are invisible from the form. Every term must be present, terms are matched
// as substrings, and Keywords is matched against the whole job description
// rather than the title. A reasonable-looking search can therefore return
// almost nothing, or almost everything, and the user has no way to tell
// which from looking at it.
//
// Importing a template creates an ordinary saved search. Nothing links the
// two afterwards: the copy is the user's, to edit or delete as they like,
// and a later release changing a template never rewrites a search they have
// already adopted.
type SearchTemplate struct {
	// ID is stable across releases. It is what a client sends to import a
	// template, so renaming one for clarity must not change it.
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Summary  string         `json:"summary"`
	Criteria SearchCriteria `json:"criteria"`
}

// A SearchTemplateGroup is the set of templates for one kind of work.
//
// A group is a menu, not a bundle. Roles that read as one job to a hiring
// manager are separate searches here because Matches cannot express "or":
// "Backend Engineer" and "Backend Developer" is two searches, and adopting
// only one silently discards half the market.
type SearchTemplateGroup struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Summary   string           `json:"summary"`
	Templates []SearchTemplate `json:"templates"`
}

// searchTemplateGroups is the catalog.
//
// Three rules govern what may be added here, each learned by measuring
// candidate criteria against 654 live postings from all five sources. See
// docs/02_features/11_search_templates.md for the measurements.
//
// Put the distinguishing word in Title, not the role word. "Backend" in a
// title is what makes the job a backend job; "Engineer" only decides which
// half of the market you see, because a board writes either Engineer or
// Developer and Matches requires every term. Leaving the role word out
// returned the same postings as pinning it, plus the ones titled Developer.
//
// Split compound words rather than guessing a spelling. Title "full stack"
// found 26 postings; "full-stack" found 15 and "fullstack" 9. Terms are
// matched as substrings, so "full" and "stack" each occur inside all three
// spellings, while no single spelling occurs inside the others.
//
// Never use a term shorter than four characters, and never put a common word
// in Keywords. Keywords is matched against the whole description, where
// ordinary prose supplies almost anything: keywords "go" matched 426 of 654
// postings, on "go-to", "goals" and "Django". Substring matching costs the
// abbreviations too — title "ui" matches "Cruise" and "Recruiter", "ux"
// matches "Luxury", "ml" matches the German "Teamleiter". So no template
// searches for UI, UX, ML, SRE, iOS or Go by those names.
var searchTemplateGroups = []SearchTemplateGroup{
	{
		ID:      "software-engineering",
		Name:    "Software engineering",
		Summary: "Engineering roles by specialty. Adopt several: boards title the same job Engineer or Developer, and one search cannot cover both.",
		Templates: []SearchTemplate{
			{
				ID:       "backend-engineer",
				Name:     "Backend Engineer",
				Summary:  "Titles containing Backend or Back End, whether Engineer or Developer.",
				Criteria: SearchCriteria{Title: "back end"},
			},
			{
				ID:       "frontend-engineer",
				Name:     "Frontend Engineer",
				Summary:  "Titles containing Frontend or Front End.",
				Criteria: SearchCriteria{Title: "front end"},
			},
			{
				ID:       "fullstack-engineer",
				Name:     "Full-stack Engineer",
				Summary:  "All three spellings: Full-Stack, Full Stack and Fullstack.",
				Criteria: SearchCriteria{Title: "full stack"},
			},
			{
				ID:       "software-engineer",
				Name:     "Software Engineer",
				Summary:  "The broad one. Use it to see the market, then narrow with a salary floor or a location.",
				Criteria: SearchCriteria{Title: "software engineer"},
			},
			{
				ID:       "software-developer",
				Name:     "Software Developer",
				Summary:  "The same roles at boards that say Developer. Worth adopting alongside Software Engineer.",
				Criteria: SearchCriteria{Title: "software developer"},
			},
			{
				ID:       "platform-engineer",
				Name:     "Platform Engineer",
				Summary:  "Internal platform and developer-experience teams.",
				Criteria: SearchCriteria{Title: "platform engineer"},
			},
		},
	},
	{
		ID:      "engineering-by-stack",
		Name:    "Engineering by stack",
		Summary: "For a specific technology. These read the job description, not the title, because boards rarely put a stack in the title — so expect some postings that only mention the technology in passing.",
		Templates: []SearchTemplate{
			{
				ID:      "go-engineer",
				Name:    "Go Engineer",
				Summary: "Searches for Golang. Plain \"Go\" is unusable: it also matches go-to, goals and Django.",
				Criteria: SearchCriteria{
					Keywords: "golang",
				},
			},
			{
				ID:      "react-engineer",
				Name:    "React Engineer",
				Summary: "React in the description, Frontend or Front End in the title.",
				Criteria: SearchCriteria{
					Keywords: "react",
					Title:    "front end",
				},
			},
			{
				ID:      "react-fullstack",
				Name:    "React Full-stack Engineer",
				Summary: "React in the description, Full-stack in the title.",
				Criteria: SearchCriteria{
					Keywords: "react",
					Title:    "full stack",
				},
			},
			{
				ID:      "python-backend-engineer",
				Name:    "Python Backend Engineer",
				Summary: "Python in the description, Backend in the title.",
				Criteria: SearchCriteria{
					Keywords: "python",
					Title:    "back end",
				},
			},
			{
				ID:      "typescript-frontend-engineer",
				Name:    "TypeScript Frontend Engineer",
				Summary: "TypeScript in the description, Frontend in the title.",
				Criteria: SearchCriteria{
					Keywords: "typescript",
					Title:    "front end",
				},
			},
		},
	},
	{
		ID:      "devops-infrastructure",
		Name:    "DevOps and infrastructure",
		Summary: "Running the systems rather than building the product. The titles vary more than in any other discipline, so adopt all of these.",
		Templates: []SearchTemplate{
			{
				ID:       "devops-engineer",
				Name:     "DevOps Engineer",
				Summary:  "Titles containing DevOps.",
				Criteria: SearchCriteria{Title: "devops"},
			},
			{
				ID:       "site-reliability-engineer",
				Name:     "Site Reliability Engineer",
				Summary:  "Spelled out. SRE alone is too short to search on safely.",
				Criteria: SearchCriteria{Title: "site reliability"},
			},
			{
				ID:       "infrastructure-engineer",
				Name:     "Infrastructure Engineer",
				Summary:  "Titles containing Infrastructure.",
				Criteria: SearchCriteria{Title: "infrastructure"},
			},
			{
				ID:       "cloud-engineer",
				Name:     "Cloud Engineer",
				Summary:  "Titles containing Cloud and Engineer.",
				Criteria: SearchCriteria{Title: "cloud engineer"},
			},
			{
				ID:      "terraform-engineer",
				Name:    "Terraform / IaC Engineer",
				Summary: "Terraform in the description, Engineer in the title. Includes product engineering roles that ask for Terraform.",
				Criteria: SearchCriteria{
					Keywords: "terraform",
					Title:    "engineer",
				},
			},
			{
				ID:      "kubernetes-platform-engineer",
				Name:    "Kubernetes Platform Engineer",
				Summary: "Kubernetes in the description, Platform in the title.",
				Criteria: SearchCriteria{
					Keywords: "kubernetes",
					Title:    "platform",
				},
			},
		},
	},
	{
		ID:      "data-and-ml",
		Name:    "Data and machine learning",
		Summary: "Data work, from pipelines through analysis to models.",
		Templates: []SearchTemplate{
			{
				ID:       "data-engineer",
				Name:     "Data Engineer",
				Summary:  "Pipelines and warehouses.",
				Criteria: SearchCriteria{Title: "data engineer"},
			},
			{
				ID:       "data-scientist",
				Name:     "Data Scientist",
				Summary:  "Titles containing Data Scientist.",
				Criteria: SearchCriteria{Title: "data scientist"},
			},
			{
				ID:       "data-analyst",
				Name:     "Data Analyst",
				Summary:  "Titles containing Data Analyst.",
				Criteria: SearchCriteria{Title: "data analyst"},
			},
			{
				ID:       "machine-learning-engineer",
				Name:     "Machine Learning Engineer",
				Summary:  "Spelled out. ML alone matches the German Teamleiter.",
				Criteria: SearchCriteria{Title: "machine learning"},
			},
		},
	},
	{
		ID:      "product",
		Name:    "Product management",
		Summary: "Owning what gets built.",
		Templates: []SearchTemplate{
			{
				ID:       "product-manager",
				Name:     "Product Manager",
				Summary:  "Also matches Senior and Principal Product Manager.",
				Criteria: SearchCriteria{Title: "product manager"},
			},
			{
				ID:       "product-owner",
				Name:     "Product Owner",
				Summary:  "The title European boards prefer for the same work.",
				Criteria: SearchCriteria{Title: "product owner"},
			},
			{
				ID:       "technical-program-manager",
				Name:     "Technical Program Manager",
				Summary:  "Titles containing Program Manager.",
				Criteria: SearchCriteria{Title: "program manager"},
			},
			{
				ID:       "product-marketing-manager",
				Name:     "Product Marketing Manager",
				Summary:  "Titles containing Product Marketing.",
				Criteria: SearchCriteria{Title: "product marketing"},
			},
		},
	},
	{
		ID:      "design",
		Name:    "Design",
		Summary: "Product and interface design. UX and UI are not searched on directly: two letters match Luxury and Cruise.",
		Templates: []SearchTemplate{
			{
				ID:       "product-designer",
				Name:     "Product Designer",
				Summary:  "Titles containing Product Designer.",
				Criteria: SearchCriteria{Title: "product designer"},
			},
			{
				ID:      "ux-designer",
				Name:    "UX Designer",
				Summary: "Designer in the title, usability in the description. Narrower than Product Designer, and more reliably about software.",
				Criteria: SearchCriteria{
					Keywords: "usability",
					Title:    "designer",
				},
			},
			{
				ID:       "design-lead",
				Name:     "Design Lead",
				Summary:  "Titles containing Design and Lead.",
				Criteria: SearchCriteria{Title: "design lead"},
			},
			{
				ID:       "designer-any",
				Name:     "Designer (any)",
				Summary:  "The broad one. It also finds graphic and fashion design, which share the word.",
				Criteria: SearchCriteria{Title: "designer"},
			},
			{
				ID:      "figma-designer",
				Name:    "Designer (Figma)",
				Summary: "Designer in the title, Figma in the description — a reliable sign of product rather than graphic design.",
				Criteria: SearchCriteria{
					Keywords: "figma",
					Title:    "designer",
				},
			},
		},
	},
	{
		ID:      "quality-and-security",
		Name:    "Quality and security",
		Summary: "Testing and security engineering.",
		Templates: []SearchTemplate{
			{
				ID:       "qa-engineer",
				Name:     "QA Engineer",
				Summary:  "Titles containing QA.",
				Criteria: SearchCriteria{Title: "qa"},
			},
			{
				ID:       "test-engineer",
				Name:     "Test Engineer",
				Summary:  "Also matches Software Engineer in Test.",
				Criteria: SearchCriteria{Title: "engineer test"},
			},
			{
				ID:       "security-engineer",
				Name:     "Security Engineer",
				Summary:  "Also matches Cybersecurity Engineer.",
				Criteria: SearchCriteria{Title: "security engineer"},
			},
			{
				ID:       "cybersecurity",
				Name:     "Cybersecurity",
				Summary:  "Titles containing Cyber. Broader than Security Engineer, and it also finds sales roles at security companies.",
				Criteria: SearchCriteria{Title: "cyber"},
			},
		},
	},
}

// SearchTemplateGroups returns the catalog of ready-made searches.
//
// It is static Go data rather than seeded rows, so that a release can
// correct a template without a migration, and so the CLI and the desktop
// application cannot disagree about what a template contains.
func SearchTemplateGroups() []SearchTemplateGroup {
	groups := make([]SearchTemplateGroup, len(searchTemplateGroups))
	for i, group := range searchTemplateGroups {
		groups[i] = group
		groups[i].Templates = append([]SearchTemplate(nil), group.Templates...)
	}
	return groups
}

// FindSearchTemplate returns the template with the given ID.
func FindSearchTemplate(id string) (SearchTemplate, error) {
	for _, group := range searchTemplateGroups {
		for _, template := range group.Templates {
			if template.ID == id {
				return template, nil
			}
		}
	}
	return SearchTemplate{}, fmt.Errorf("no search template with ID %q", id)
}
