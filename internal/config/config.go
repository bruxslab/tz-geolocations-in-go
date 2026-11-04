package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address, DatabaseURL string
	RedisURL             string
	CacheTTL             time.Duration
	MaxConns, MinConns   int32
	RateRequests         int
	RateWindow           time.Duration
	CORSOrigins          []string
	TrustedProxies       []netip.Prefix
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), CORSOrigins: strings.Split(env("CORS_ORIGINS", "*"), ",")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	port, err := strconv.Atoi(env("HTTP_PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return c, errors.New("HTTP_PORT must be between 1 and 65535")
	}
	c.Address = net.JoinHostPort(env("HTTP_HOST", "127.0.0.1"), strconv.Itoa(port))
	type integerSetting struct {
		name, def string
		min, max  int
		dst       *int
	}
	var max, min, seconds, cacheSeconds int
	values := []integerSetting{
		{"CACHE_TTL_SECONDS", "300", 1, 86400, &cacheSeconds},
		{"DATABASE_MAX_CONNS", "10", 1, 1000, &max},
		{"DATABASE_MIN_CONNS", "2", 0, 1000, &min},
		{"RATE_LIMIT_REQUESTS", "100", 1, 1000000, &c.RateRequests},
		{"RATE_LIMIT_WINDOW_SECONDS", "60", 1, 86400, &seconds},
	}
	for _, v := range values {
		n, e := strconv.Atoi(env(v.name, v.def))
		if e != nil || n < v.min || n > v.max {
			return c, fmt.Errorf("invalid %s", v.name)
		}
		*v.dst = n
	}
	if min > max {
		return c, errors.New("DATABASE_MIN_CONNS exceeds DATABASE_MAX_CONNS")
	}
	c.RedisURL = os.Getenv("REDIS_URL")
	c.CacheTTL = time.Duration(cacheSeconds) * time.Second
	c.MaxConns = int32(max)
	c.MinConns = int32(min)
	c.RateWindow = time.Duration(seconds) * time.Second
	for i, o := range c.CORSOrigins {
		c.CORSOrigins[i] = strings.TrimSpace(o)
	}
	for _, raw := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p, e := netip.ParsePrefix(raw)
		if e != nil {
			return c, errors.New("TRUSTED_PROXIES must contain comma-separated CIDRs")
		}
		c.TrustedProxies = append(c.TrustedProxies, p)
	}
	return c, nil
}
