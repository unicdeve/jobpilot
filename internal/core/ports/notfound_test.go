package ports

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAsReference(t *testing.T) {
	t.Parallel()

	t.Run("a missing record a request named becomes an unknown reference, and nothing else", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("failed to create application: %w",
			AsReference(fmt.Errorf("failed to get job: %w", &NotFoundError{Kind: "job", ID: "j1"})))

		assert.ErrorIs(t, err, ErrUnknownReference)
		assert.NotErrorIs(t, err, ErrNotFound, "it would be answered as the requested record being gone")
		assert.EqualError(t, err, "failed to create application: job not found: j1")
	})

	t.Run("any other error is returned as it is", func(t *testing.T) {
		t.Parallel()
		failure := errors.New("database is locked")

		assert.Same(t, failure, AsReference(failure))
		assert.NoError(t, AsReference(nil))
	})
}
