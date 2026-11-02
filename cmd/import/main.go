package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/bruxslab/tz-geolocations-in-go/internal/database"
)

func run() error {
	replace := flag.Bool("replace", false, "atomically replace a different application dataset")
	directory := flag.String("data", "", "directory containing all four JSON files (default: embedded pinned snapshot)")
	source := flag.String("source", "", "source version required for external data")
	flag.Parse()
	var d *data.Dataset
	var err error
	if *directory == "" {
		d, err = data.Pinned()
		*source = data.SourceCommit
	} else {
		if *source == "" {
			return &usageError{"--source is required with --data"}
		}
		d, err = data.Read(os.DirFS(*directory))
	}
	if err != nil {
		return err
	}
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		return &usageError{"MIGRATION_DATABASE_URL or DATABASE_URL is required"}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, url, 2, 0)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = database.Import(ctx, pool, d, *source, *replace); err != nil {
		return err
	}
	slog.Info("dataset verified and imported", "source", *source, "fingerprint", d.Fingerprint, "counts", d.Stats())
	return nil
}

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }
func main() {
	if err := run(); err != nil {
		slog.Error("import failed", "error", err)
		os.Exit(1)
	}
}
