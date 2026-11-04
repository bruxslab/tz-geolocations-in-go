package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bruxslab/tz-geolocations-in-go/internal/middleware"
)

type memoryCache map[string][]byte

func (m memoryCache) Get(_ context.Context, k string) ([]byte, bool) { b, ok := m[k]; return b, ok }
func (m memoryCache) Set(_ context.Context, k string, b []byte)      { m[k] = b }

func TestResponseCache(t *testing.T) {
	store := memoryCache{}
	a := &API{log: slog.Default(), cache: store}
	calls := 0
	h := a.handle(func(r *http.Request) (response, error) {
		calls++
		if r.URL.Query().Get("q") == "bad" {
			return response{}, bad("invalid")
		}
		if r.URL.Query().Get("q") == "fail" {
			return response{}, errors.New("database unavailable")
		}
		return response{data: r.URL.Query().Get("q")}, nil
	})
	request := func(method, url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, url, nil))
		return w
	}
	first := request("GET", "/v1/search?q=dar&type=ward")
	second := request("GET", "/v1/search?type=ward&q=dar")
	if first.Body.String() != second.Body.String() || calls != 1 {
		t.Fatal("repeated request executed endpoint", calls)
	}
	head := request("HEAD", "/v1/search?q=dar&type=ward")
	if calls != 1 || head.Body.Len() != 0 || head.Header().Get("Cache-Control") == "" {
		t.Fatal("HEAD cache behavior")
	}
	request("GET", "/v1/search?q=dar&type=street")
	request("GET", "/v1/autocomplete?q=dar&type=ward")
	if calls != 3 {
		t.Fatal("path or query collision", calls)
	}
	for _, q := range []string{"bad", "fail"} {
		for range 2 {
			w := request("GET", "/v1/search?q="+q)
			if w.Code == 200 || w.Header().Get("Cache-Control") != "" {
				t.Fatal("cached error")
			}
		}
	}
	if calls != 7 {
		t.Fatal("errors reused", calls)
	}
	request("POST", "/v1/search?q=dar&type=ward")
	if calls != 7 {
		t.Fatal("POST executed")
	}
}

func TestCacheRetainsRequestMiddleware(t *testing.T) {
	a := &API{log: slog.Default(), cache: memoryCache{}}
	calls := 0
	h := middleware.Wrap(a.handle(func(*http.Request) (response, error) { calls++; return response{data: "ok"}, nil }), middleware.Options{RateRequests: 2, RateWindow: time.Minute, Origins: []string{"https://one.test", "https://two.test"}})
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("GET", "/v1/regions", nil)
		origin := []string{"https://one.test", "https://two.test", "https://one.test"}[i]
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("CORS was cached")
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Fatal("cached request bypassed quota")
		}
	}
	if calls != 1 {
		t.Fatal("endpoint called", calls)
	}
}
