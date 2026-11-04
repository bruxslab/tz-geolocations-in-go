package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bruxslab/tz-geolocations-in-go/internal/cache"
	"github.com/bruxslab/tz-geolocations-in-go/internal/config"
	"github.com/bruxslab/tz-geolocations-in-go/internal/database"
	"github.com/bruxslab/tz-geolocations-in-go/internal/handlers"
	"github.com/bruxslab/tz-geolocations-in-go/internal/middleware"
	"github.com/bruxslab/tz-geolocations-in-go/internal/repository"
)

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, c.DatabaseURL, c.MaxConns, c.MinConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	var fingerprint string
	if err = pool.QueryRow(ctx, "SELECT fingerprint FROM locations.dataset_version WHERE singleton").Scan(&fingerprint); err != nil || fingerprint == "" {
		return errors.New("application dataset is not initialized; run migrate and import")
	}
	var store cache.Store
	if c.RedisURL != "" {
		rc, e := cache.New(c.RedisURL, c.DatabaseURL+"\x00"+fingerprint, c.CacheTTL)
		if e != nil {
			return e
		}
		defer rc.Close()
		store = rc
		logger.Info("Redis response cache configured", "ttl", c.CacheTTL)
	}
	server := &http.Server{Addr: c.Address, Handler: handlers.New(&repository.Repository{Pool: pool}, middleware.Options{RateRequests: c.RateRequests, RateWindow: c.RateWindow, Origins: c.CORSOrigins, TrustedProxies: c.TrustedProxies, Logger: logger}, store), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	done := make(chan error, 1)
	go func() { logger.Info("API listening", "address", c.Address); done <- server.ListenAndServe() }()
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		slog.Error("API stopped", "error", err)
		os.Exit(1)
	}
}
