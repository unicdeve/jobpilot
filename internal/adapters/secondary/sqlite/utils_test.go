package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "nil slice", in: nil, want: ""},
		{name: "empty slice", in: []string{}, want: ""},
		{name: "single element", in: []string{"go"}, want: `["go"]`},
		{name: "multiple elements", in: []string{"go", "sql"}, want: `["go","sql"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, joinStrings(tt.in))
		})
	}
}

func TestSplitStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty string", in: "", want: []string{}},
		{name: "single element", in: `["go"]`, want: []string{"go"}},
		{name: "multiple elements", in: `["go","sql"]`, want: []string{"go", "sql"}},
		{name: "malformed JSON decodes to empty slice", in: "not json", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, splitStrings(tt.in))
		})
	}
}

func TestJoinSplitStringsRoundTrip(t *testing.T) {
	t.Parallel()

	in := []string{"go", "sql", "testing"}
	assert.Equal(t, in, splitStrings(joinStrings(in)))
}
