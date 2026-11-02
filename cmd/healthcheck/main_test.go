package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		healthy    bool
	}{
		{"populated", `{"data":[{"name":"Arusha"}]}`, 200, true},
		{"empty", `{"data":[]}`, 200, false},
		{"missing", `{}`, 200, false},
		{"broken-json", `bad`, 200, false},
		{"database-error", `{"data":[1]}`, 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			if got := check(server.Client(), server.URL) == nil; got != tc.healthy {
				t.Fatalf("healthy=%v want %v", got, tc.healthy)
			}
		})
	}
}
