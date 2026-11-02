package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/bruxslab/tz-geolocations-in-go/internal/database"
	"github.com/jackc/pgx/v5"
)

func run() error {
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		return errors.New("MIGRATION_DATABASE_URL or DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, url, 2, 0)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = database.Migrate(ctx, pool); err != nil {
		return err
	}
	if role := os.Getenv("RUNTIME_DATABASE_ROLE"); role != "" {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(context.Background())
		quoted := pgx.Identifier{role}.Sanitize()
		if _, err = tx.Exec(ctx, "GRANT USAGE ON SCHEMA locations TO "+quoted); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "GRANT SELECT ON ALL TABLES IN SCHEMA locations TO "+quoted); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA locations GRANT SELECT ON TABLES TO "+quoted); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	slog.Info("migrations applied")
	return nil
}
func main() {
	if err := run(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
