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

// ErrUnknownReference is matched, with errors.Is, by every error that says
// a request named a record to act with, rather than the record it acts on,
// and that record does not exist: an application's job, a resume's
// candidate, a scout's saved search. The HTTP API answers it with 422, not
// 404, because the record a request is addressed to is there; it is the
// request that is wrong.
var ErrUnknownReference = errors.New("unknown reference")

// ReferenceError is a service's answer for an ID a request carried that
// names no record. It keeps the repository's message, and it matches
// ErrUnknownReference and deliberately not ErrNotFound, so it can never be
// read as the requested record being gone.
type ReferenceError struct {
	NotFound *NotFoundError
}

func (e *ReferenceError) Error() string {
	return e.NotFound.Error()
}

// Is reports whether target is ErrUnknownReference.
func (e *ReferenceError) Is(target error) bool {
	return target == ErrUnknownReference
}

// AsReference turns a repository's NotFoundError, wherever it is in err's
// chain, into a ReferenceError, for a service looking up a record a request
// named to act with. Any other error, or nil, is returned as it is.
func AsReference(err error) error {
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return &ReferenceError{NotFound: notFound}
	}
	return err
}
