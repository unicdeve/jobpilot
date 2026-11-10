package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireLoopback(t *testing.T) {
	reached := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := RequireLoopback(reached)

	cases := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{name: "the CLI, which names the address and sends no origin", host: "127.0.0.1:52011", want: http.StatusNoContent},
		{name: "localhost", host: "localhost:52011", want: http.StatusNoContent},
		{name: "the IPv6 loopback", host: "[::1]:52011", want: http.StatusNoContent},
		{name: "a page this daemon served", host: "127.0.0.1:52011", origin: "http://127.0.0.1:52011", want: http.StatusNoContent},
		{name: "the same page, opened as localhost", host: "localhost:52011", origin: "http://localhost:52011", want: http.StatusNoContent},
		{name: "the packaged desktop on macOS", host: "127.0.0.1:52011", origin: "tauri://localhost", want: http.StatusNoContent},
		{name: "the desktop in development", host: "127.0.0.1:52011", origin: "http://localhost:1420", want: http.StatusNoContent},
		{
			name: "a website whose name was rebound to 127.0.0.1",
			host: "attacker.example:52011", origin: "http://attacker.example:52011", want: http.StatusForbidden,
		},
		{name: "a rebound name with no origin", host: "attacker.example:52011", want: http.StatusForbidden},
		{name: "a form on another website", host: "127.0.0.1:52011", origin: "https://attacker.example", want: http.StatusForbidden},
		{
			name: "a page on another loopback port", host: "127.0.0.1:52011", origin: "http://127.0.0.1:8080",
			want: http.StatusForbidden,
		},
		{name: "a sandboxed frame, whose origin is null", host: "127.0.0.1:52011", origin: "null", want: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/shutdown", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tc.want, rec.Code)
		})
	}
}
