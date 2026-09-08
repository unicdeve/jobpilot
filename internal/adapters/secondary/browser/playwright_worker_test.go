//go:build integration

package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fixtureFormHTML is a small application-form-shaped page covering every
// field kind ExtractForm/Fill need to handle: a labelled text input, a
// textarea, a select, a checkbox, and a fieldset/legend radio group — plus
// a hidden input and a submit button that must be skipped.
const fixtureFormHTML = `<!doctype html>
<html>
<body>
<form>
	<label for="full-name">Full name</label>
	<input id="full-name" name="full_name" type="text" required>

	<label for="cover-note">Why do you want to work here?</label>
	<textarea id="cover-note" name="cover_note"></textarea>

	<label for="country">Country</label>
	<select id="country" name="country">
		<option>United States</option>
		<option>United Kingdom</option>
		<option>Nigeria</option>
	</select>

	<label><input type="checkbox" name="agree"> I agree to the terms</label>

	<fieldset>
		<legend>Are you authorized to work in this country?</legend>
		<label><input type="radio" name="authorized" value="yes"> Yes</label>
		<label><input type="radio" name="authorized" value="no"> No</label>
	</fieldset>

	<input type="hidden" name="source" value="jobpilot-test">
	<button type="submit">Submit application</button>
</form>
</body>
</html>`

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureFormHTML))
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestWorker(t *testing.T) *PlaywrightWorker {
	t.Helper()
	worker, err := NewPlaywrightWorker(t.TempDir(), true)
	if err != nil {
		t.Skipf("skipping: real Chrome/Playwright driver not available in this environment: %v", err)
	}
	t.Cleanup(func() { _ = worker.Close(context.Background()) })
	return worker
}

func TestPlaywrightWorker_ExtractAndFillForm(t *testing.T) {
	server := newFixtureServer(t)
	worker := newTestWorker(t)
	ctx := context.Background()

	require.NoError(t, worker.Navigate(ctx, server.URL))

	reason, blocked := worker.DetectBlocker(ctx)
	assert.False(t, blocked, "an ordinary form page must never be flagged as blocked: %s", reason)

	fields, err := worker.ExtractForm(ctx)
	require.NoError(t, err)

	byLabel := map[string]ports.FormField{}
	for _, f := range fields {
		byLabel[f.Label] = f
	}

	// Hidden input and submit button must be skipped, leaving exactly the
	// five real fields (the radio pair collapses into one grouped field).
	require.Len(t, fields, 5)

	name, ok := byLabel["Full name"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindText, name.Kind)
	assert.True(t, name.Required)

	note, ok := byLabel["Why do you want to work here?"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindTextarea, note.Kind)

	country, ok := byLabel["Country"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindSelect, country.Kind)
	assert.Equal(t, []string{"United States", "United Kingdom", "Nigeria"}, country.Options)

	agree, ok := byLabel["I agree to the terms"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindCheckbox, agree.Kind)

	authorized, ok := byLabel["Are you authorized to work in this country?"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindRadio, authorized.Kind)
	assert.Equal(t, []string{"Yes", "No"}, authorized.Options)

	// Fill every field and verify the value actually landed, reading the
	// live page back directly (this test lives in the same package, so
	// the worker's private page field is reachable).
	require.NoError(t, worker.Fill(ctx, name.Ref, "Ada Lovelace"))
	got, err := worker.page.Locator("#full-name").InputValue()
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", got)

	require.NoError(t, worker.Fill(ctx, note.Ref, "Because I love Go."))
	got, err = worker.page.Locator("#cover-note").InputValue()
	require.NoError(t, err)
	assert.Equal(t, "Because I love Go.", got)

	require.NoError(t, worker.Fill(ctx, country.Ref, "United Kingdom"))
	got, err = worker.page.Locator("#country").InputValue()
	require.NoError(t, err)
	assert.NotEmpty(t, got)

	require.NoError(t, worker.Fill(ctx, agree.Ref, "yes"))
	checked, err := worker.page.Locator("input[name='agree']").IsChecked()
	require.NoError(t, err)
	assert.True(t, checked)

	require.NoError(t, worker.Fill(ctx, authorized.Ref, "No"))
	checked, err = worker.page.Locator("input[name='authorized'][value='no']").IsChecked()
	require.NoError(t, err)
	assert.True(t, checked)
}
