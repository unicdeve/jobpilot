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
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isReadableLabel(tt.label))
		})
	}
}
