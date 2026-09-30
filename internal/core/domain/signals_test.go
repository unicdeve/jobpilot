package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The locations and sentences below are from real postings, measured on
// 2026-09-30 (ADR-0023).

func TestReadWorkable_Locations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		remote    string
		locations []string
		scope     WorkableScope
		countries []string
	}{
		{"worldwide", RemoteTypeRemote, []string{"Worldwide"}, WorkableAnywhere, nil},
		{"anywhere in the world", RemoteTypeRemote, []string{"Anywhere in the World"}, WorkableAnywhere, nil},
		{"remote global", RemoteTypeRemote, []string{"Remote (Global)"}, WorkableAnywhere, nil},
		{"remote everywhere", RemoteTypeRemote, []string{"Remote(Everywhere)"}, WorkableAnywhere, nil},
		{"anywhere with hours", RemoteTypeRemote, []string{"REMOTE (anywhere, EST hours)"}, WorkableAnywhere, nil},
		{"remote us", RemoteTypeRemote, []string{"Remote (US)"}, WorkableListed, []string{"US"}},
		{"us fully remote", RemoteTypeRemote, []string{"US (Fully Remote)"}, WorkableListed, []string{"US"}},
		{"remote dash us", RemoteTypeRemote, []string{"Remote - US"}, WorkableListed, []string{"US"}},
		{"us citizens", RemoteTypeRemote, []string{"U.S. Citizens"}, WorkableListed, []string{"US"}},
		{"us only with cities", RemoteTypeRemote, []string{"REMOTE (US ONLY, LA/SF preferred)"}, WorkableListed, []string{"US"}},
		{"country list", RemoteTypeRemote, []string{"USA, Canada, Argentina, Mexico, Peru"}, WorkableListed,
			[]string{"AR", "CA", "MX", "PE", "US"}},
		{"us time zones only", RemoteTypeRemote, []string{"USA, Canada, USA timezones"}, WorkableListed, []string{"CA", "US"}},
		{"himalayas countries", RemoteTypeRemote, []string{"United States", "Poland"}, WorkableListed, []string{"PL", "US"}},
		{"bare remote", RemoteTypeRemote, []string{"Remote"}, WorkableUnknown, nil},
		{"no location", RemoteTypeRemote, nil, WorkableUnknown, nil},
		{"time zone only", RemoteTypeRemote, []string{"REMOTE (US time zones)"}, WorkableUnknown, nil},
		{"exclusion list", RemoteTypeRemote, []string{"Internationally located (not in the US, CA, UK, NZ, or AU)"}, WorkableUnknown, nil},
		{"not a place", RemoteTypeRemote, []string{"Full-time*"}, WorkableUnknown, nil},
		{"onsite city", RemoteTypeOnsite, []string{"München"}, WorkableListed, []string{"DE"}},
		{"onsite city and country", RemoteTypeOnsite, []string{"London, England, United Kingdom"}, WorkableListed, []string{"GB"}},
		{"onsite state", RemoteTypeOnsite, []string{"Hamburg, Hamburg, Deutschland"}, WorkableListed, []string{"DE"}},
		{"onsite either city", RemoteTypeHybrid, []string{"London, UK or NYC"}, WorkableListed, []string{"GB", "US"}},
		{"onsite parenthesised", RemoteTypeOnsite, []string{"ONSITE (Zürich, Switzerland)"}, WorkableListed, []string{"CH"}},
		{"hybrid with terms", RemoteTypeHybrid, []string{"Hybrid (Belgium, 3 days/week)"}, WorkableListed, []string{"BE"}},
		{"onsite anywhere is still its office", RemoteTypeOnsite, []string{"Berlin", "Worldwide"}, WorkableListed, []string{"DE"}},
		{"onsite unplaced town", RemoteTypeOnsite, []string{"Grünwald"}, WorkableUnknown, nil},
		{"city named by its state", RemoteTypeOnsite, []string{"On-Site (Cambridge, MA)"}, WorkableListed, []string{"US"}},
		{"city named by its province", RemoteTypeOnsite, []string{"London, ON"}, WorkableListed, []string{"CA"}},
		{"most populous city", RemoteTypeOnsite, []string{"Cambridge"}, WorkableListed, []string{"GB"}},
		{"country code is not a state", RemoteTypeOnsite, []string{"Hamburg, DE"}, WorkableListed, []string{"DE"}},
		{"city outside its country", RemoteTypeOnsite, []string{"Paris, Texas"}, WorkableListed, []string{"US"}},
		{"compound country names", RemoteTypeRemote,
			[]string{"🇧🇦 Bosnia and Herzegovina, 🇹🇼 Taiwan, Province of China, 🇰🇷 Korea (Republic of), and 🇬🇧 United Kingdom of Great Britain and Northern Ireland"},
			WorkableListed, []string{"BA", "GB", "KR", "TW"}},
		{"regions", RemoteTypeRemote, []string{"Northern America, LATAM, Europe, APAC"}, WorkableListed, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			job := Job{RemoteType: tc.remote, Locations: tc.locations}
			ReadSignals(&job)
			assert.Equal(t, tc.scope, job.Workable.Scope)
			if tc.countries != nil {
				assert.Equal(t, tc.countries, job.Workable.Countries)
			}
		})
	}
}

func TestReadWorkable_RegionsExpand(t *testing.T) {
	t.Parallel()

	job := Job{RemoteType: RemoteTypeRemote, Locations: []string{"EMEA"}}
	ReadSignals(&job)
	assert.Equal(t, []string{"EMEA"}, job.Workable.Places)
	assert.True(t, job.Workable.OpenTo("NG"), "Nigeria is in EMEA")
	assert.True(t, job.Workable.OpenTo("de"))
	assert.False(t, job.Workable.OpenTo("US"))
}

func TestReadWorkable_DescriptionContradictsTheLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		locations   []string
		description string
		contradicts bool
	}{
		{"must reside", []string{"Anywhere in the World"},
			"Candidates must reside in or be willing to relocate to the San Francisco Bay Area (Alameda, Contra Costa).", true},
		{"legally within the us", []string{"Anywhere in the World"},
			"Applicants must be able to work legally within the United States. Please, no 3rd party inquiries.", false},
		{"authorized in the us", []string{"Anywhere in the World"},
			"We're only hiring those authorized to work in the United States.", true},
		{"us based only", []string{"Anywhere in the World"},
			"US based applicants only. $114,297 — $235,319 USD", true},
		{"located in several", []string{"Worldwide"},
			"This is a remote position. However, candidates must be located in the United States, Canada (Ontario or British Columbia), Netherlands, Poland.", true},
		{"consistent with the label", []string{"Remote (US)"},
			"Candidates must be authorized to work in the United States without sponsorship.", false},
		{"narrower than the label", []string{"Europe"},
			"You must be based in Germany to be considered.", false},
		{"elsewhere than the label", []string{"Europe"},
			"You must be authorized to work in the United States.", true},
		{"time zone", []string{"Worldwide"},
			"You must be located within three hours of CET time zone.", false},
		{"no place named", []string{"Worldwide"},
			"You must be eligible to work in the country you live in.", false},
		{"us the pronoun", []string{"Worldwide"},
			"Remote from anywhere; let us know where you are based and we will work with you.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			job := Job{RemoteType: RemoteTypeRemote, Locations: tc.locations, DescriptionMarkdown: tc.description}
			ReadSignals(&job)
			if tc.contradicts {
				assert.Equal(t, WorkableUnknown, job.Workable.Scope)
				assert.NotEmpty(t, job.Workable.Note, "the sentence is kept, so the candidate sees why")
			} else {
				assert.True(t, job.Workable.Known(), "scope %s, note %q", job.Workable.Scope, job.Workable.Note)
			}
		})
	}
}

func TestReadWorkable_TitleContradictsTheLabel(t *testing.T) {
	t.Parallel()

	for title, known := range map[string]bool{
		"Staff Backend Engineer - Core Product (Europe)":                false,
		"Senior Backend Engineer, AI Platform - 100% Remote - EMEA":     true,
		"Backend Engineer - Wallets (100% Remote Worldwide)":            true,
		"Backend Software Engineer - Python/Postgres [Remote / Global]": true,
		"Go - Senior Backend Engineer, AI Coding Agent Quality":         true,
		"Senior Engineer (US)": false,
		"Senior Backend Engineer, Integrations - 100% Remote - EMEA": true,
	} {
		job := Job{Title: title, RemoteType: RemoteTypeRemote, Locations: []string{"Worldwide"}}
		if strings.Contains(title, "EMEA") {
			// Himalayas' own list of EMEA, which differs from JobPilot's
			// at its edges.
			job.Locations = []string{"Afghanistan", "Albania", "Germany", "Nigeria", "Kazakhstan"}
		}
		ReadSignals(&job)
		assert.Equal(t, known, job.Workable.Known(), "%q read as %s", title, job.Workable.Scope)
		if !known {
			assert.Equal(t, title, job.Workable.Note)
		}
	}
}

func TestReadOffer_Relocation(t *testing.T) {
	t.Parallel()

	for sentence, want := range map[string]OfferStatus{
		"We welcome applications from talent worldwide and provide relocation support to Hamburg, Germany.":                         OfferOffered,
		"Full relocation to SF covered. Onsite in SF.":                                                                              OfferOffered,
		"- Relocation support (up to €10k) for roles based in France":                                                               OfferOffered,
		"Do you want to move to Germany? No problem – we offer you an attractive relocation package.":                               OfferOffered,
		"We are open to support with relocation efforts.":                                                                           OfferOffered,
		"We are currently unable to offer fully remote work, visa sponsorship, or relocation support for this opportunity.":         OfferDeclined,
		"Relocation assistance will not be provided for this role.":                                                                 OfferDeclined,
		"This position is not eligible for relocation assistance.":                                                                  OfferDeclined,
		"Please note: For this role we are not offering relocation support or visa sponsorship.":                                    OfferDeclined,
		"Visa sponsorship may be available for eligible candidates already located in a UK/EU country; relocation is not provided.": OfferDeclined,
		"Based in or willing to relocate to San Francisco, CA or Austin, TX.":                                                       OfferMentioned,
		"Relocation assistance (position dependent).":                                                                               OfferMentioned,
		"Benefits vary by country and may include healthcare coverage, parental leave and relocation support.":                      OfferMentioned,
		"We are not a typical company, and we offer relocation support to everyone who joins.":                                      OfferOffered,
		"Our platform processes 25 trillion data points.":                                                                           "",
	} {
		assert.Equal(t, want, readOffer(Sentences(sentence), relocationTopic).Status, "%q", sentence)
	}
}

func TestReadOffer_Visa(t *testing.T) {
	t.Parallel()

	for sentence, want := range map[string]OfferStatus{
		"$250k+ base + seed equity | Visa sponsorship available":                                                     OfferOffered,
		"Visa sponsorship: yes.":                                                                                     OfferOffered,
		"Why join us: €10k signing bonus - Visa sponsorship + relocation support":                                    OfferOffered,
		"Applicants must be based in the US, we are unable to sponsor visas.":                                        OfferDeclined,
		"Software Engineering Intern | ONSITE (Zürich, Switzerland) | Internship | No visa sponsorship":              OfferDeclined,
		"The role touches ITAR-controlled data, so US persons only, no visa sponsorship.":                            OfferDeclined,
		"Candidates must be authorized to work in the United States without current or future employer sponsorship.": OfferDeclined,
		"Unfortunately, we do not offer sponsorship and you do need to be living in the UK or Ireland to apply.":     OfferDeclined,
		"We do not provide visa assistance, and our cooperation model does not include benefits.":                    OfferDeclined,
		"$180-200K + equity | VISA (transfer only)":                                                                  OfferMentioned,
		"Proactively mentor, sponsor and develop senior engineers.":                                                  "",
		"Company-sponsored dinners and social gatherings.":                                                           "",
		"An employer-sponsored 401(k) retirement plan.":                                                              "",
	} {
		assert.Equal(t, want, readOffer(Sentences(sentence), visaTopic).Status, "%q", sentence)
	}
}

func TestReadOffer_RefusalWinsAndKeepsItsSentence(t *testing.T) {
	t.Parallel()

	description := "Benefits may include relocation support.\n\nThis position is not eligible for relocation assistance."
	offer := readOffer(Sentences(description), relocationTopic)
	assert.Equal(t, OfferDeclined, offer.Status)
	assert.Equal(t, "This position is not eligible for relocation assistance", offer.Evidence)
}

func TestSentences_PlainText(t *testing.T) {
	t.Parallel()

	sentences := Sentences("&lt;p&gt;We are **unable** to sponsor U.S. visas.&lt;/p&gt;\n- Relocation support | [https://acme.example](https://acme.example/jobs)")
	assert.Equal(t, []string{"We are unable to sponsor US visas", "Relocation support", "https://acme.example"}, sentences)
}
