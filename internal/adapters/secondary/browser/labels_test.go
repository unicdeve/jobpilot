package browser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsReadableLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		label string
		want  bool
	}{
		// Measured on the real Ashby form: radio groups with no
		// <fieldset><legend> and UUID name attributes.
		{"ce2a003d 1cd3 4cab 82ae 298b5f8df5f3 7f7d551e 04d5 4fb2 9384 d0392b01dfc4", false},
		{"c72e127d-1d9e-47e7-9013-81068949bd6c", false},
		{"deadbeefcafe", false},

		// Measured on the real Lever form: custom questions whose only
		// label is the form-encoded name attribute. Every token here reads
		// as a word, so a label being entirely an identifier cannot be the
		// test — it is the identifier inside it that gives it away.
		{"cards[e27c9c9a-4ad6-4889-b355-8862299c8274][field0]", false},
		{"surveysResponses[a025f000 ffd1 400a bdf2 be50f57f6908][responses][field0]", false},
		{"", false},
		{"   ", false},

		{"Name", true},
		{"Email", true},
		{"Where are you based?", true},
		{"What are your salary expectations annually ?", true},
		{"CV", true},
		{"Téléphone", true},

		// Short all-hex strings are words, not identifiers.
		{"Fee", true},
		{"Added", true},
		{"Face", true},

		// A real question is never rejected for the digits in it, however
		// many of them there are.
		{"How many years of experience do you have with Go, as of 2026?", true},
		{"Notice period in days (e.g. 30, 60, 90)", true},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isReadableLabel(tt.label))
		})
	}
}

func TestCleanLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		label string
		want  string
	}{
		// Measured on the real Greenhouse and Lever forms, which mark a
		// required field with an asterisk inside the label text.
		{"greenhouse required marker", "First Name*", "First Name"},
		{"lever required marker", "Phone ✱", "Phone"},
		{"no marker to remove", "Current company", "Current company"},

		// Punctuation that is part of the question survives: only the
		// marker goes.
		{"a question mark is not a marker", "Where are you based?", "Where are you based?"},
		{"a spaced question mark is not a marker", "What are your salary expectations annually ?", "What are your salary expectations annually ?"},

		{"surrounding whitespace", "  Email  ", "Email"},
		{"nothing at all", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cleanLabel(tt.label))
		})
	}
}
