package middleware

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

func Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Del("Cache-Control")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

type entry struct {
	count int
	reset time.Time
}
type Options struct {
	RateRequests   int
	RateWindow     time.Duration
	Origins        []string
	TrustedProxies []netip.Prefix
	Logger         *slog.Logger
}

func Wrap(next http.Handler, opt Options) http.Handler {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.RateRequests <= 0 {
		opt.RateRequests = 100
	}
	if opt.RateWindow <= 0 {
		opt.RateWindow = time.Minute
	}
	if opt.Origins == nil {
		opt.Origins = []string{"*"}
	}
	var mu sync.Mutex
	store := map[string]entry{}
	lastCleanup := time.Now()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				opt.Logger.Error("unexpected request panic")
				Error(w, 500, "INTERNAL_ERROR", "An unexpected error occurred")
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		origin := r.Header.Get("Origin")
		for _, o := range opt.Origins {
			if o == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				break
			}
			if o == origin && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS")
			if headers := r.Header.Get("Access-Control-Request-Headers"); headers != "" {
				w.Header().Set("Access-Control-Allow-Headers", headers)
				w.Header().Add("Vary", "Access-Control-Request-Headers")
			}
			w.WriteHeader(204)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			now := time.Now()
			ip := ClientIP(r, opt.TrustedProxies)
			mu.Lock()
			if now.Sub(lastCleanup) >= opt.RateWindow {
				for k, v := range store {
					if !now.Before(v.reset) {
						delete(store, k)
					}
				}
				lastCleanup = now
			}
			e, exists := store[ip]
			full := !exists && len(store) >= 10000
			if !exists || !now.Before(e.reset) {
				e = entry{reset: now.Add(opt.RateWindow)}
			}
			e.count++
			if !full {
				store[ip] = e
			}
			mu.Unlock()
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(opt.RateRequests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, opt.RateRequests-e.count)))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt((e.reset.UnixMilli()+999)/1000, 10))
			if full || e.count > opt.RateRequests {
				Error(w, 429, "RATE_LIMITED", "Too many requests. Please try again later.")
				return
			}
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 65536)
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				Error(w, 413, "BAD_REQUEST", "Request body is too large")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ClientIP walks X-Forwarded-For right-to-left, stopping at the first untrusted
// hop. Headers from an untrusted immediate peer are ignored entirely.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	ip = ip.Unmap()
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	if !isTrusted(ip) {
		return ip.String()
	}
	if raw := r.Header.Get("X-Forwarded-For"); raw != "" {
		hops := strings.Split(raw, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			next, e := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if e != nil {
				return ip.String()
			}
			ip = next.Unmap()
			if !isTrusted(ip) {
				break
			}
		}
		return ip.String()
	}
	// CF's single-client header is accepted only from an explicitly trusted peer.
	if next, e := netip.ParseAddr(r.Header.Get("CF-Connecting-IP")); e == nil {
		return next.Unmap().String()
	}
	return ip.String()
}
