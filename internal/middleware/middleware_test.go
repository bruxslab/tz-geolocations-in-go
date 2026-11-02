package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:123"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("CF-Connecting-IP", "8.8.8.8")
	if got := ClientIP(r, nil); got != "192.0.2.1" {
		t.Fatal(got)
	}
	trusted := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	r.Header.Set("X-Forwarded-For", "8.8.8.8, 198.51.100.3, 192.0.2.2")
	if got := ClientIP(r, trusted); got != "198.51.100.3" {
		t.Fatal(got)
	}
}
func TestRateLimitAndReset(t *testing.T) {
	h := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), Options{RateRequests: 2, RateWindow: 5 * time.Millisecond})
	request := func() *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/stats", nil))
		return rr
	}
	if request().Code != 200 || request().Code != 200 {
		t.Fatal("early limit")
	}
	rr := request()
	if rr.Code != 429 || rr.Header().Get("X-RateLimit-Remaining") != "0" || rr.Header().Get("Cache-Control") != "" {
		t.Fatal(rr)
	}
	time.Sleep(10 * time.Millisecond)
	if request().Code != 200 {
		t.Fatal("window did not reset")
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { request() })
	}
	wg.Wait()
}
func TestCORSSizeAndPanic(t *testing.T) {
	h := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("secret") }), Options{Origins: []string{"https://example.com"}})
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != 500 || strings.Contains(rr.Body.String(), "secret") || rr.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatal(rr)
	}
	r = httptest.NewRequest("OPTIONS", "/v1/stats", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != 204 {
		t.Fatal(rr)
	}
	r = httptest.NewRequest("GET", "/", strings.NewReader(strings.Repeat("x", 65537)))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != 413 {
		t.Fatal(rr.Code)
	}
}
