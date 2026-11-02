// Package testdb serializes database suites and refuses development databases.
package testdb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/bruxslab/tz-geolocations-in-go/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_DATABASE_TESTS") == "1" {
			t.Fatal("TEST_DATABASE_URL is required")
		}
		t.Skip("PostgreSQL suite skipped: set TEST_DATABASE_URL (tz_locations_test)")
	}
	if err := database.RequireTestDatabase(url); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := database.Open(ctx, url, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	var actual string
	if err = lock.QueryRow(ctx, "SELECT current_database()").Scan(&actual); err != nil || actual != "tz_locations_test" {
		lock.Release()
		pool.Close()
		t.Fatalf("unsafe actual database %q: %v", actual, err)
	}
	if _, err = lock.Exec(ctx, "SET statement_timeout = '300s'"); err != nil {
		lock.Release()
		pool.Close()
		t.Fatal(err)
	}
	if _, err = lock.Exec(ctx, "SELECT pg_advisory_lock(720114003)"); err != nil {
		lock.Release()
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := lock.Exec(c, "SELECT pg_advisory_unlock(720114003)"); err != nil {
			_ = lock.Conn().Close(c)
		}
		lock.Release()
		pool.Close()
	})
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	d, err := data.Pinned()
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Import(ctx, pool, d, data.SourceCommit, true); err != nil {
		t.Fatal(err)
	}
	return pool
}
