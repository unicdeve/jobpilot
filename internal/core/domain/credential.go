package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// DefaultCredentialLabel is the label a user gets when they name none.
// A connector with nothing configured asks for it (ADR-0019 §1).
const DefaultCredentialLabel = "default"

// credentialPartPattern is what a source or a label may look like: lower
// case, digits, and single interior hyphens.
//
// The restriction is what makes the pair composable. A ref addresses one
// keychain item as "source/label", so a part containing a slash would make
// "adzuna" + "work/eu" and "adzuna/work" + "eu" the same item — one
// credential silently overwriting another. Forbidding consecutive hyphens
// does the same job for the environment variable in ADR-0019 §5, where each
// hyphen becomes an underscore and two underscores separate the parts.
var credentialPartPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxCredentialPart bounds a part so that a ref cannot be used to write an
// unbounded key into the keychain or the index.
const maxCredentialPart = 64

// A CredentialRef names one stored secret.
//
// Source is owned by the code that needs it — a connector asks for
// "adzuna" — and Label is the user's, so they can hold more than one
// (ADR-0019 §1). The secret itself is never part of a ref: a ref is what
// the product may log, list and pass around, and the value is what it may
// not.
type CredentialRef struct {
	Source string `json:"source"`
	Label  string `json:"label"`
}

// NewCredentialRef builds a validated ref, defaulting an empty label.
//
// Validation lives here rather than at the HTTP edge so that no caller can
// construct a ref that does not round-trip through the keychain.
func NewCredentialRef(source, label string) (CredentialRef, error) {
	source = strings.TrimSpace(source)
	label = strings.TrimSpace(label)
	if label == "" {
		label = DefaultCredentialLabel
	}

	ref := CredentialRef{Source: source, Label: label}
	if err := ref.Validate(); err != nil {
		return CredentialRef{}, err
	}
	return ref, nil
}

// Validate reports whether the ref can address a stored credential.
func (r CredentialRef) Validate() error {
	if err := validateCredentialPart("source", r.Source); err != nil {
		return err
	}
	return validateCredentialPart("label", r.Label)
}

func validateCredentialPart(field, value string) error {
	switch {
	case value == "":
		return fmt.Errorf("a credential %s cannot be empty", field)
	case len(value) > maxCredentialPart:
		return fmt.Errorf("a credential %s cannot be longer than %d characters", field, maxCredentialPart)
	case !credentialPartPattern.MatchString(value):
		return fmt.Errorf(
			"a credential %s may use lower-case letters, digits and single hyphens between them, so %q will not do",
			field, value)
	}
	return nil
}

// String is how a ref addresses one keychain item, and how it reads in a
// log line or an error. It is safe to print: a ref holds no secret.
func (r CredentialRef) String() string {
	return r.Source + "/" + r.Label
}

// EnvVarName is the environment variable that overrides this credential
// (ADR-0019 §5). Each part is upper-cased with hyphens turned into
// underscores, and two underscores separate them — a single one is already
// what a hyphen becomes, and no valid part contains two in a row.
func (r CredentialRef) EnvVarName() string {
	upper := func(part string) string {
		return strings.ToUpper(strings.ReplaceAll(part, "-", "_"))
	}
	return "JOBPILOT_CREDENTIAL_" + upper(r.Source) + "__" + upper(r.Label)
}
