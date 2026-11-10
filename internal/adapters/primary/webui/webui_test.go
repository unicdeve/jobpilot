package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func builtInterface() fstest.MapFS {
	return fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html><title>JobPilot</title>")},
		"assets/index-a1.js": {Data: []byte("console.log(1)")},
	}
}

func get(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestServe(t *testing.T) {
	handler := serve(builtInterface())

	t.Run("the page is served at the root, and revalidated", func(t *testing.T) {
		rec := get(t, handler, http.MethodGet, "/")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<title>JobPilot</title>")
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	})

	t.Run("it cannot be framed by another page", func(t *testing.T) {
		rec := get(t, handler, http.MethodGet, "/")
		assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "connect-src 'self'")
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	})

	t.Run("an asset, named after its content, is cached for good", func(t *testing.T) {
		rec := get(t, handler, http.MethodGet, "/assets/index-a1.js")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Cache-Control"), "immutable")
	})

	t.Run("a file that is not there is 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, get(t, handler, http.MethodGet, "/nope.js").Code)
	})

	t.Run("nothing but a read is answered", func(t *testing.T) {
		rec := get(t, handler, http.MethodPost, "/")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
	})
}

func TestNotBuilt(t *testing.T) {
	handler := http.HandlerFunc(notBuilt)

	rec := get(t, handler, http.MethodGet, "/")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "make build-web")

	assert.Equal(t, http.StatusNotFound, get(t, handler, http.MethodGet, "/assets/x.js").Code)
}
