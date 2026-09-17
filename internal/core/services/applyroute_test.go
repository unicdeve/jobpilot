package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

func field(label string) ports.FormField {
	return ports.FormField{Label: label, Kind: ports.FormFieldKindText}
}

func TestLooksLikeApplicationForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields []ports.FormField
		want   bool
	}{
		{
			// The defect this gate exists for: the board's newsletter box
			// was the only field on the posting, and an earlier version
			// filled it with the candidate's address and called the
			// application prepared.
			name:   "a newsletter signup is not an application form",
			fields: []ports.FormField{field("Enter your email")},
			want:   false,
		},
		{
			name:   "a login is not an application form",
			fields: []ports.FormField{field("Email"), field("Password")},
			want:   false,
		},
		{
			name:   "a search box is not an application form",
			fields: []ports.FormField{field("Search jobs")},
			want:   false,
		},
		{
			// Measured on the real Ashby form this route leads to.
			name: "a real ATS application form is",
			fields: []ports.FormField{
				field("Name"), field("Email"), field("Current company"),
				field("Current location"), field("Phone"), field("Additional information"),
				field("LinkedIn"), field("What are your salary expectations annually ?"),
			},
			want: true,
		},
		{
			name:   "three distinct things asked about a person is enough",
			fields: []ports.FormField{field("Full name"), field("Email"), field("Phone number")},
			want:   true,
		},
		{
			// Several spellings of one question are still one question, so
			// a form asking only for a name cannot reach the threshold by
			// splitting it in two.
			name:   "first and last name count once",
			fields: []ports.FormField{field("First name"), field("Last name"), field("Email")},
			want:   false,
		},
		{
			name:   "nothing at all is not an application form",
			fields: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, LooksLikeApplicationForm(tt.fields))
		})
	}
}

func TestPickApplyControl(t *testing.T) {
	t.Parallel()

	t.Run("prefers a link to a known ATS over any wording", func(t *testing.T) {
		t.Parallel()
		controls := []ports.ApplyControl{
			{Ref: "0", Text: "Apply now", URL: "https://board.example/jobs/1/apply"},
			{Ref: "1", Text: "See role", URL: "https://jobs.ashbyhq.com/acme/1234/application"},
		}

		got := PickApplyControl(controls)

		require.NotNil(t, got)
		assert.Equal(t, "1", got.Ref, "the destination itself beats a guess about wording")
	})

	t.Run("finds the control on the route this was built for", func(t *testing.T) {
		t.Parallel()
		// As measured on the live Arbeitnow posting.
		controls := []ports.ApplyControl{
			{Ref: "0", Text: "Jobs in Berlin", URL: "https://www.arbeitnow.fr/jobs/berlin"},
			{Ref: "1", Text: "Apply Now", URL: "https://www.arbeitnow.fr/jobs/companies/lemlist/x-387941/apply"},
			{Ref: "2", Text: "Apply Now", URL: ""},
		}

		got := PickApplyControl(controls)

		require.NotNil(t, got)
		assert.Equal(t, "1", got.Ref)
	})

	t.Run("a button with no URL still counts on its wording", func(t *testing.T) {
		t.Parallel()
		controls := []ports.ApplyControl{
			{Ref: "0", Text: "Save job", URL: ""},
			{Ref: "1", Text: "Apply for this job", URL: ""},
		}

		got := PickApplyControl(controls)

		require.NotNil(t, got)
		assert.Equal(t, "1", got.Ref)
	})

	t.Run("nothing that looks like a way in returns nothing", func(t *testing.T) {
		t.Parallel()
		controls := []ports.ApplyControl{
			{Ref: "0", Text: "Home", URL: "https://board.example/"},
			{Ref: "1", Text: "Privacy policy", URL: "https://board.example/privacy"},
			{Ref: "2", Text: "Applying the filters below narrows your search", URL: "https://board.example/help"},
		}

		assert.Nil(t, PickApplyControl(controls), "the LLM fallback exists for exactly this case")
	})

	t.Run("ignores words inside other words and inside query strings", func(t *testing.T) {
		t.Parallel()
		controls := []ports.ApplyControl{
			{Ref: "0", Text: "Reapply later", URL: "https://board.example/x"},
			{Ref: "1", Text: "Applied Materials", URL: "https://board.example/company/applied-materials"},
			{Ref: "2", Text: "Share", URL: "https://board.example/x?utm_campaign=apply"},
		}

		assert.Nil(t, PickApplyControl(controls))
	})

	t.Run("no controls at all", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, PickApplyControl(nil))
	})
}
