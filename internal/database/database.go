package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/bruxslab/tz-geolocations-in-go/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, url string, max, min int32) (*pgxpool.Pool, error) {
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	c.MaxConns = max
	c.MinConns = min
	c.ConnConfig.ConnectTimeout = 5 * time.Second
	c.ConnConfig.RuntimeParams["application_name"] = "tz-locations"
	c.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, errors.New("cannot initialize PostgreSQL pool")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, errors.New("PostgreSQL connectivity check failed")
	}
	return pool, nil
}

// Migrate serializes migrations and verifies checksums on subsequent runs.
// Each version is committed atomically with its migration record.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock(720114001)"); err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := conn.Exec(c, "SELECT pg_advisory_unlock(720114001)"); e != nil {
			_ = conn.Conn().Close(c)
		}
	}()
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.tz_locations_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		checksum := hex.EncodeToString(sum[:])
		var existing string
		err = conn.QueryRow(ctx, "SELECT checksum FROM public.tz_locations_migrations WHERE version=$1", name).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("migration %s checksum changed", name)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		// Data backfills may take longer than request-time reads; keep them
		// bounded independently while retaining the caller's overall deadline.
		if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '120s'"); err != nil {
			_ = tx.Rollback(context.Background())
			return err
		}
		if _, err = tx.Exec(ctx, string(b)); err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO public.tz_locations_migrations(version,checksum) VALUES($1,$2)", name, checksum)
		}
		if err != nil {
			_ = tx.Rollback(context.Background())
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// RequireTestDatabase refuses any target other than tz_locations_test, including
// URL options that redirect the connection into a different database/schema.
func RequireTestDatabase(url string) error {
	c, err := pgx.ParseConfig(url)
	if err != nil {
		return errors.New("invalid TEST_DATABASE_URL")
	}
	if c.Database != "tz_locations_test" {
		return errors.New("integration tests require database tz_locations_test")
	}
	if _, ok := c.RuntimeParams["search_path"]; ok {
		return errors.New("test URL must not override search_path")
	}
	return nil
}
