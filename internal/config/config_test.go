package config

import "testing"

func TestConfiguration(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "HTTP_HOST", "HTTP_PORT", "DATABASE_MAX_CONNS", "DATABASE_MIN_CONNS", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW_SECONDS", "CORS_ORIGINS", "TRUSTED_PROXIES"} {
		t.Setenv(k, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("missing URL accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/tz_locations")
	c, err := Load()
	if err != nil || c.Address != "127.0.0.1:8080" || c.MaxConns != 10 {
		t.Fatal(c, err)
	}
	t.Setenv("DATABASE_MIN_CONNS", "11")
	if _, err = Load(); err == nil {
		t.Fatal("invalid pool")
	}
	t.Setenv("DATABASE_MIN_CONNS", "2")
	t.Setenv("TRUSTED_PROXIES", "127.0.0.0/8")
	if _, err = Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRUSTED_PROXIES", "bad")
	if _, err = Load(); err == nil {
		t.Fatal("invalid proxy")
	}
}
