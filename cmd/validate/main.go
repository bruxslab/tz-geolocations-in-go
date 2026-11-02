package main

import (
	"github.com/bruxslab/tz-geolocations-in-go/data"
	"log/slog"
	"os"
)

func main() {
	d, err := data.Pinned()
	if err != nil {
		slog.Error("data validation failed", "error", err)
		os.Exit(1)
	}
	slog.Info("pinned data valid", "counts", d.Stats(), "fingerprint", d.Fingerprint)
}
