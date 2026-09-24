package ports

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema_MarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("an object lists every property as required, in a stable order", func(t *testing.T) {
		t.Parallel()
		schema := ObjectSchema(map[string]Schema{
			"score":     IntegerSchema("0 to 100"),
			"rationale": StringSchema("why"),
			"tags":      ArraySchema("labels", StringSchema("")),
		})

		encoded, err := json.Marshal(schema)
		require.NoError(t, err)

		var document map[string]any
		require.NoError(t, json.Unmarshal(encoded, &document))
		assert.Equal(t, "object", document["type"])
		assert.Equal(t, []any{"rationale", "score", "tags"}, document["required"],
			"sorted, so the request body does not differ run to run")
		assert.Equal(t, false, document["additionalProperties"],
			"a field nobody asked for hides the fact that the model misread the task")

		// Marshalling twice must produce identical bytes; Go map order is
		// random, and a schema that changes shape every call would make a
		// golden test flap for no reason.
		again, err := json.Marshal(schema)
		require.NoError(t, err)
		assert.Equal(t, string(encoded), string(again))
	})

	t.Run("a nullable field permits null, which is how a model says it does not know", func(t *testing.T) {
		t.Parallel()
		encoded, err := json.Marshal(NumberSchema("years, if stated").AsNullable())
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":["number","null"],"description":"years, if stated"}`, string(encoded))
	})

	t.Run("an array carries its item shape", func(t *testing.T) {
		t.Parallel()
		encoded, err := json.Marshal(ArraySchema("skills", StringSchema("a skill")))
		require.NoError(t, err)

		var document map[string]any
		require.NoError(t, json.Unmarshal(encoded, &document))
		assert.Equal(t, "array", document["type"])
		assert.NotNil(t, document["items"])
	})
}

// TestSchema_CannotConstrainMeaning is the type-level half of ADR-0017.
//
// The decision is that a schema describes shape and never meaning, and the
// enforcement is that Schema has no field capable of expressing the
// alternative. This test states the rule where someone adding a field
// would look, and fails the moment a keyword leaks into the output.
//
// `pattern` and numeric bounds are not enforced by the grammar anyway, so
// writing them would promise something untrue. `enum` is enforced, which
// is worse: asked to classify a Facebook link against the six kinds the
// profile has, a model answered "linkedin" — a wrong answer no later check
// can catch, where the unconstrained "facebook" is dropped on sight.
func TestSchema_CannotConstrainMeaning(t *testing.T) {
	t.Parallel()

	schema := ObjectSchema(map[string]Schema{
		"kind":  StringSchema("one of linkedin, github, other"),
		"score": IntegerSchema("0 to 100"),
		"start": StringSchema("as YYYY-MM-DD"),
		"items": ArraySchema("things", ObjectSchema(map[string]Schema{
			"name": StringSchema("what it is"),
		})),
	})

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)

	for _, forbidden := range []string{`"enum"`, `"pattern"`, `"minimum"`, `"maximum"`,
		`"minLength"`, `"maxLength"`, `"format"`} {
		assert.NotContains(t, string(encoded), forbidden,
			"a schema states shape; meaning is checked after decoding, by code")
	}
}

func TestSchema_FieldNames(t *testing.T) {
	t.Parallel()

	schema := ObjectSchema(map[string]Schema{
		"b": StringSchema(""),
		"a": StringSchema(""),
	})

	assert.Equal(t, []string{"a", "b"}, schema.FieldNames())
	assert.Empty(t, StringSchema("").FieldNames(), "a scalar has no properties")
}
