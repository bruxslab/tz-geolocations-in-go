package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedis(t *testing.T) {
	server := miniredis.RunT(t)
	c, err := New("redis://"+server.Addr(), "dataset-one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	if _, ok := c.Get(ctx, "missing"); ok {
		t.Fatal("unexpected hit")
	}
	c.Set(ctx, "regions", []byte(`{"data":[]}`))
	if b, ok := c.Get(ctx, "regions"); !ok || string(b) != `{"data":[]}` {
		t.Fatal("cache miss", string(b))
	}
	other, err := New("redis://"+server.Addr(), "dataset-two", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, ok := other.Get(ctx, "regions"); ok {
		t.Fatal("dataset namespace collision")
	}
	server.FastForward(time.Minute)
	if _, ok := c.Get(ctx, "regions"); ok {
		t.Fatal("TTL ignored")
	}
	server.Close()
	if _, ok := c.Get(ctx, "regions"); ok {
		t.Fatal("outage returned a hit")
	}
	if c.available() {
		t.Fatal("outage did not open circuit")
	}
	c.Set(ctx, "regions", []byte("ignored"))
	// A recovered cache can be retried after the cooldown.
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	c.retryAfter.Store(0)
	c.Set(ctx, "regions", []byte("recovered"))
	if b, ok := c.Get(ctx, "regions"); !ok || string(b) != "recovered" {
		t.Fatal("cache failed to recover")
	}
}
func TestInvalidURL(t *testing.T) {
	if _, err := New("https://invalid", "test", time.Minute); err == nil {
		t.Fatal("invalid URL accepted")
	}
}
