package ports

import (
	"errors"
	"fmt"
)

// ErrNotFound is matched, with errors.Is, by every repository error that
// says the record an ID names does not exist. It lets an interface tell a
// missing record from a failure without knowing which repository said so:
// the HTTP API answers it with 404 rather than 500.
var ErrNotFound = errors.New("not found")

// NotFoundError is a repository's answer for an ID that names no record of
// its Kind. It matches ErrNotFound.
type NotFoundError struct {
	Kind string // what was looked for, in words: "job", "cover letter"
	ID   string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Kind, e.ID)
}

// Is reports whether target is ErrNotFound.
func (e *NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}
