package ports

import (
	"encoding/json"
	"sort"
)

// SchemaType is the JSON type of a value.
type SchemaType string

// The JSON types a response schema can describe.
const (
	SchemaTypeObject  SchemaType = "object"
	SchemaTypeArray   SchemaType = "array"
	SchemaTypeString  SchemaType = "string"
	SchemaTypeInteger SchemaType = "integer"
	SchemaTypeNumber  SchemaType = "number"
	SchemaTypeBoolean SchemaType = "boolean"
)

// Schema describes the shape of a JSON response, for providers that can
// constrain a model to it (ADR-0017).
//
// It deliberately cannot express `enum`, `pattern`, `minimum` or
// `maximum`, and this is the main thing to understand about the type.
//
// Two of those are not enforced at all. Ollama compiles a schema to a
// grammar, and a grammar can require "a string here" but not "a string
// matching this regex": measured, a `pattern` of ^\d{4}-\d{2}$ returned
// ".role started in July 2022.", and a 0–100 bound asked for 5000
// returned 0. Writing them would promise something untrue.
//
// `enum` is enforced, and that is the problem. Asked to classify a
// Facebook link against the six link kinds the profile has, a model
// returned "linkedin". Unconstrained it returns "facebook", which
// ProposedLinks drops because it is not a kind we have — the right
// outcome. The constraint turns an obviously wrong answer into a
// plausible one that no later check can catch, which is the failure
// ADR-0016 was also about.
//
// So a schema here describes shape and never meaning. Meaning is checked
// after decoding, by code, which is also the only thing that runs on the
// three backends that cannot apply a schema.
type Schema struct {
	Type SchemaType
	// Description is passed to the model and is a useful place to say what
	// a field means, including when to leave it empty.
	Description string
	// Nullable allows JSON null in addition to Type. It is how a model is
	// given a way to say it does not know, rather than being pushed to the
	// nearest plausible answer.
	Nullable bool
	// Properties and Required apply to an object. Every property is
	// required by default — a model that may not know a value should be
	// given a nullable field to say so, not an absent one, because an
	// absent field and a field it forgot are indistinguishable.
	Properties map[string]Schema
	// Items applies to an array.
	Items *Schema
}

// ObjectSchema describes an object whose every property is required.
func ObjectSchema(properties map[string]Schema) Schema {
	return Schema{Type: SchemaTypeObject, Properties: properties}
}

// StringSchema describes a string.
func StringSchema(description string) Schema {
	return Schema{Type: SchemaTypeString, Description: description}
}

// IntegerSchema describes an integer.
func IntegerSchema(description string) Schema {
	return Schema{Type: SchemaTypeInteger, Description: description}
}

// NumberSchema describes a number.
func NumberSchema(description string) Schema {
	return Schema{Type: SchemaTypeNumber, Description: description}
}

// BooleanSchema describes a boolean.
func BooleanSchema(description string) Schema {
	return Schema{Type: SchemaTypeBoolean, Description: description}
}

// ArraySchema describes an array of items.
func ArraySchema(description string, items Schema) Schema {
	return Schema{Type: SchemaTypeArray, Description: description, Items: &items}
}

// AsNullable returns a copy that also permits null.
func (s Schema) AsNullable() Schema {
	s.Nullable = true
	return s
}

// MarshalJSON writes the schema as a JSON Schema document.
//
// Property names are sorted so the output is byte-stable: an unstable
// schema would make the request body differ run to run for no reason, and
// would make a golden test flap.
func (s Schema) MarshalJSON() ([]byte, error) {
	document := map[string]any{}

	if s.Nullable {
		document["type"] = []string{string(s.Type), "null"}
	} else {
		document["type"] = string(s.Type)
	}
	if s.Description != "" {
		document["description"] = s.Description
	}

	switch s.Type {
	case SchemaTypeObject:
		properties := map[string]any{}
		required := make([]string, 0, len(s.Properties))
		for name, property := range s.Properties {
			properties[name] = property
			required = append(required, name)
		}
		sort.Strings(required)
		document["properties"] = properties
		document["required"] = required
		// Without this a model may add fields nobody asked for, which then
		// decode into nothing and hide the fact that it misread the task.
		document["additionalProperties"] = false
	case SchemaTypeArray:
		if s.Items != nil {
			document["items"] = *s.Items
		}
	}

	return json.Marshal(document)
}

// FieldNames lists an object schema's property names, sorted.
//
// This exists for the drift test: a schema and the Go struct it describes
// are two statements of the same shape, and they part company the moment
// someone adds a field to one.
func (s Schema) FieldNames() []string {
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
