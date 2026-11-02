package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Import changes only the dedicated locations schema. A transaction and an
// advisory lock make replacement atomic and concurrent imports safe.
func Import(ctx context.Context, pool *pgxpool.Pool, d *data.Dataset, source string, replace bool) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if d.Fingerprint == "" || source == "" {
		return errors.New("dataset fingerprint and source version are required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	// Bulk loading/index maintenance has a different budget from HTTP reads.
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '120s'"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(720114002)"); err != nil {
		return err
	}
	var old string
	err = tx.QueryRow(ctx, "SELECT fingerprint FROM locations.dataset_version WHERE singleton").Scan(&old)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if old == d.Fingerprint {
		if err = verify(ctx, tx, d.Stats()); err == nil {
			return tx.Commit(ctx)
		} else if !replace {
			return err
		}
	}
	if old != "" && !replace {
		return errors.New("a different dataset is installed; explicit --replace is required")
	}
	// Refuse to adopt populated tables without provenance unless replacement was requested.
	var populated bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM locations.regions UNION ALL SELECT 1 FROM locations.districts UNION ALL SELECT 1 FROM locations.wards UNION ALL SELECT 1 FROM locations.streets)`).Scan(&populated); err != nil {
		return err
	}
	if populated && !replace {
		return errors.New("unversioned data exists; explicit --replace is required")
	}
	if replace {
		if _, err = tx.Exec(ctx, `TRUNCATE locations.search_documents, locations.streets, locations.wards, locations.districts, locations.regions, locations.dataset_version`); err != nil {
			return err
		}
	}
	regions := map[string]int{}
	districts := map[string]int{}
	wards := map[string]int{}
	for i, r := range d.Regions {
		regions[r.Slug] = i + 1
	}
	for i, r := range d.Districts {
		districts[data.DistrictKey(r.RegionSlug, r.Slug)] = i + 1
	}
	for i, r := range d.Wards {
		wards[data.WardKey(r.RegionSlug, r.DistrictSlug, r.Slug)] = i + 1
	}
	copies := []struct {
		table   string
		columns []string
		count   int
		row     func(int) ([]any, error)
	}{
		{"regions", []string{"id", "name", "slug"}, len(d.Regions), func(i int) ([]any, error) { r := d.Regions[i]; return []any{i + 1, r.Name, r.Slug}, nil }},
		{"districts", []string{"id", "region_id", "name", "slug"}, len(d.Districts), func(i int) ([]any, error) {
			r := d.Districts[i]
			return []any{i + 1, regions[r.RegionSlug], r.Name, r.Slug}, nil
		}},
		{"wards", []string{"id", "district_id", "name", "slug", "postcode"}, len(d.Wards), func(i int) ([]any, error) {
			r := d.Wards[i]
			return []any{i + 1, districts[data.DistrictKey(r.RegionSlug, r.DistrictSlug)], r.Name, r.Slug, r.Postcode}, nil
		}},
		{"streets", []string{"id", "ward_id", "name", "postcode"}, len(d.Streets), func(i int) ([]any, error) {
			r := d.Streets[i]
			return []any{i + 1, wards[data.WardKey(r.RegionSlug, r.DistrictSlug, r.WardSlug)], r.Name, r.Postcode}, nil
		}},
	}
	for _, c := range copies {
		n, e := tx.CopyFrom(ctx, pgx.Identifier{"locations", c.table}, c.columns, pgx.CopyFromSlice(c.count, c.row))
		if e != nil {
			return fmt.Errorf("copy %s: %w", c.table, e)
		}
		if n != int64(c.count) {
			return fmt.Errorf("copy %s: expected %d rows, got %d", c.table, c.count, n)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO locations.search_documents(type,entity_id,region_id,district_id,ward_id,street_id,name,postcode,full_path)
 SELECT 'region',r.id,r.id,NULL::integer,NULL::integer,NULL::integer,r.name,NULL::text,r.name FROM locations.regions r
 UNION ALL SELECT 'district',d.id,NULL,d.id,NULL,NULL,d.name,NULL,d.name||', '||r.name FROM locations.districts d JOIN locations.regions r ON r.id=d.region_id
 UNION ALL SELECT 'ward',w.id,NULL,NULL,w.id,NULL,w.name,w.postcode,w.name||', '||d.name||', '||r.name FROM locations.wards w JOIN locations.districts d ON d.id=w.district_id JOIN locations.regions r ON r.id=d.region_id
 UNION ALL SELECT 'street',s.id,NULL,NULL,NULL,s.id,s.name,s.postcode,s.name||', '||w.name||', '||d.name||', '||r.name FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id JOIN locations.districts d ON d.id=w.district_id JOIN locations.regions r ON r.id=d.region_id`); err != nil {
		return err
	}
	counts, err := json.Marshal(d.Stats())
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO locations.dataset_version(singleton,fingerprint,source_version,counts) VALUES(true,$1,$2,$3)`, d.Fingerprint, source, counts); err != nil {
		return err
	}
	if err = verify(ctx, tx, d.Stats()); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "ANALYZE locations.regions, locations.districts, locations.wards, locations.streets, locations.search_documents"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func verify(ctx context.Context, tx pgx.Tx, want data.Stats) error {
	var got data.Stats
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM locations.regions),(SELECT count(*) FROM locations.districts),(SELECT count(*) FROM locations.wards),(SELECT count(*) FROM locations.streets)`).Scan(&got.Regions, &got.Districts, &got.Wards, &got.Streets); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("dataset counts mismatch: got %+v, want %+v", got, want)
	}
	for kind, count := range map[string]int{"region": want.Regions, "district": want.Districts, "ward": want.Wards, "street": want.Streets} {
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM locations.search_documents WHERE type=$1 AND document <> ''::tsvector", kind).Scan(&n); err != nil {
			return err
		}
		if n != count {
			return fmt.Errorf("%s search index count: got %d, want %d", kind, n, count)
		}
	}
	return nil
}
