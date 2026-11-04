// Package cache provides optional, bounded-latency response caching.
package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store interface {
	Get(context.Context, string) ([]byte, bool)
	Set(context.Context, string, []byte)
}

type Redis struct {
	client     *redis.Client
	prefix     string
	ttl        time.Duration
	retryAfter atomic.Int64
}

// New validates configuration without requiring Redis to be reachable. Failures
// open a short circuit so an outage does not add a timeout to every request.
func New(rawURL, namespace string, ttl time.Duration) (*Redis, error) {
	opt, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL")
	}
	opt.DialTimeout = 100 * time.Millisecond
	opt.ReadTimeout = 100 * time.Millisecond
	opt.WriteTimeout = 100 * time.Millisecond
	opt.PoolTimeout = 100 * time.Millisecond
	opt.ContextTimeoutEnabled = true
	opt.MaxRetries = -1
	return &Redis{client: redis.NewClient(opt), prefix: fmt.Sprintf("tz:responses:v1:%x:", sha256.Sum256([]byte(namespace))), ttl: ttl}, nil
}
func (c *Redis) Close() error { return c.client.Close() }
func (c *Redis) key(key string) string {
	return fmt.Sprintf("%s%x", c.prefix, sha256.Sum256([]byte(key)))
}
func (c *Redis) available() bool { return time.Now().UnixNano() >= c.retryAfter.Load() }
func (c *Redis) failed()         { c.retryAfter.Store(time.Now().Add(5 * time.Second).UnixNano()) }
func (c *Redis) Get(ctx context.Context, key string) ([]byte, bool) {
	if !c.available() {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	body, err := c.client.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		if err != redis.Nil {
			c.failed()
		}
		return nil, false
	}
	return body, true
}
func (c *Redis) Set(ctx context.Context, key string, body []byte) {
	if !c.available() || len(body) > 2<<20 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := c.client.Set(ctx, c.key(key), body, c.ttl).Err(); err != nil {
		c.failed()
	}
}
