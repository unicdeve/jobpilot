package services

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// jsonFieldNames lists the JSON names a struct decodes, sorted.
func jsonFieldNames(t *testing.T, value any) []string {
	t.Helper()

	structType := reflect.TypeOf(value)
	require.Equal(t, reflect.Struct, structType.Kind(), "expected a struct")

	names := make([]string, 0, structType.NumField())
	for i := 0; i < structType.NumField(); i++ {
		tag := structType.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// assertDescribes checks that a schema and the struct it constrains name
// exactly the same fields.
//
// This is ADR-0017 part 5. A schema and a Go struct are two statements of
// one shape, kept in different files, and they part company the moment
// someone adds a field to one of them. When that happens the model is
// either told to produce a field that decodes into nothing, or forbidden
// from producing one the code expects — and both fail quietly.
func assertDescribes(t *testing.T, schema *ports.Schema, value any) {
	t.Helper()
	require.NotNil(t, schema)
	assert.Equal(t, jsonFieldNames(t, value), schema.FieldNames())
}

func TestSchemasMatchTheStructsTheyDescribe(t *testing.T) {
	t.Parallel()

	t.Run("match extraction", func(t *testing.T) {
		t.Parallel()
		assertDescribes(t, matchExtractionSchema(), matchExtraction{})
	})

	t.Run("job page extraction", func(t *testing.T) {
		t.Parallel()
		assertDescribes(t, jobPageExtractionSchema(), jobPageExtraction{})
	})

	t.Run("apply control choice", func(t *testing.T) {
		t.Parallel()
		assertDescribes(t, applyControlChoiceSchema(), applyControlChoice{})
	})

	t.Run("profile draft, including the nested rows", func(t *testing.T) {
		t.Parallel()
		schema := profileDraftSchema()
		assertDescribes(t, schema, domain.ProfileDraft{})

		for _, nested := range []struct {
			field string
			value any
		}{
			{"skills", domain.DraftSkill{}},
			{"experiences", domain.DraftExperience{}},
			{"links", domain.DraftLink{}},
		} {
			property, ok := schema.Properties[nested.field]
			require.True(t, ok, "%s missing from the schema", nested.field)
			require.NotNil(t, property.Items, "%s should be an array of objects", nested.field)
			assert.Equal(t, jsonFieldNames(t, nested.value), property.Items.FieldNames(),
				"%s rows", nested.field)
		}
	})
}

// TestMatchExtractionCannotSetItsOwnIdentity covers why the model's reply
// is decoded into its own type rather than into domain.JobMatch.
func TestMatchExtractionCannotSetItsOwnIdentity(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"id", "job_id", "candidate_id", "created_at",
		"model_provider", "model_name"} {
		assert.NotContains(t, jsonFieldNames(t, matchExtraction{}), field,
			"a job posting that talked a model into emitting %q would be choosing "+
				"a value the product owns", field)
	}
}
