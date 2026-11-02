package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/bruxslab/tz-geolocations-in-go/internal/database"
	"github.com/bruxslab/tz-geolocations-in-go/internal/handlers"
	"github.com/bruxslab/tz-geolocations-in-go/internal/middleware"
	"github.com/bruxslab/tz-geolocations-in-go/internal/repository"
	"github.com/bruxslab/tz-geolocations-in-go/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestPostgreSQL(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	d, err := data.Pinned()
	if err != nil {
		t.Fatal(err)
	}
	repo := &repository.Repository{Pool: pool}
	h := handlers.New(repo, middleware.Options{RateRequests: 10000})
	request := func(path string) (int, map[string]any) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return rr.Code, body
	}
	t.Run("migration and import repeat", func(t *testing.T) {
		if err := database.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
		var before, after string
		pool.QueryRow(ctx, "SELECT imported_at::text FROM locations.dataset_version").Scan(&before)
		if err := database.Import(ctx, pool, d, data.SourceCommit, false); err != nil {
			t.Fatal(err)
		}
		pool.QueryRow(ctx, "SELECT imported_at::text FROM locations.dataset_version").Scan(&after)
		if before == "" || before != after {
			t.Fatal("identical import was not a no-op")
		}
		stats, err := repo.Stats(ctx)
		if err != nil || stats != d.Stats() {
			t.Fatal(stats, err)
		}
	})
	t.Run("all stable IDs and parents", func(t *testing.T) {
		rows, err := pool.Query(ctx, `SELECT s.id,s.name,s.postcode,w.slug,d.slug,r.slug FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id JOIN locations.districts d ON d.id=w.district_id JOIN locations.regions r ON r.id=d.region_id ORDER BY s.id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		i := 0
		for rows.Next() {
			var id int
			var name, pc, w, district, region string
			if err = rows.Scan(&id, &name, &pc, &w, &district, &region); err != nil {
				t.Fatal(err)
			}
			v := d.Streets[i]
			if id != i+1 || name != v.Name || pc != v.Postcode || w != v.WardSlug || district != v.DistrictSlug || region != v.RegionSlug {
				t.Fatalf("source row %d changed", i)
			}
			i++
		}
		if rows.Err() != nil || i != 75061 {
			t.Fatal(i, rows.Err())
		}
	})
	t.Run("ambiguous slug parent filtering", func(t *testing.T) {
		seen := map[string]data.Ward{}
		var other data.Ward
		for _, w := range d.Wards {
			if first, ok := seen[w.Slug]; ok && (first.RegionSlug != w.RegionSlug || first.DistrictSlug != w.DistrictSlug) {
				other = w
				break
			}
			seen[w.Slug] = w
		}
		if other.Slug == "" {
			t.Fatal("no ambiguity fixture")
		}
		first := seen[other.Slug]
		v, err := repo.Ward(ctx, other.Slug, "", "")
		if err != nil || v.RegionSlug != first.RegionSlug || v.DistrictSlug != first.DistrictSlug {
			t.Fatal(v, err)
		}
		path := "/v1/wards/" + other.Slug + "?region_slug=" + other.RegionSlug + "&district_slug=" + other.DistrictSlug
		status, body := request(path)
		if status != 200 {
			t.Fatal(body)
		}
		vBody := body["data"].(map[string]any)
		if vBody["region"].(map[string]any)["slug"] != other.RegionSlug || !strings.Contains(vBody["_links"].(map[string]any)["self"].(string), "region_slug=") {
			t.Fatal(body)
		}
		status, _ = request("/v1/wards/" + other.Slug + "/streets?region_slug=" + other.RegionSlug + "&district_slug=" + other.DistrictSlug)
		if status != 200 {
			t.Fatal(status)
		}
	})
	t.Run("search all levels and safe input", func(t *testing.T) {
		for kind, query := range map[string]string{"region": "arusha", "district": "arumeru", "ward": "akheri", "street": "malapa"} {
			rows, err := repo.Search(ctx, query, kind, 50, false)
			if err != nil || len(rows) == 0 {
				t.Fatal(kind, rows, err)
			}
			for _, r := range rows {
				if r.Type != kind || r.FullPath == "" || r.EntityID <= 0 {
					t.Fatal(r)
				}
			}
		}
		status, body := request("/v1/search?q=malapa&type=street")
		if status != 200 || len(body["data"].([]any)) == 0 {
			t.Fatal(body)
		}
		first, e := repo.Search(ctx, "arusha", "ward", 50, false)
		if e != nil || len(first) == 0 {
			t.Fatal("path search", e)
		}
		second, e := repo.Search(ctx, "arusha", "ward", 50, false)
		if e != nil || !reflect.DeepEqual(first, second) {
			t.Fatal("nondeterministic search", e)
		}
		pc, e := repo.Search(ctx, "12102", "ward", 10, false)
		if e != nil || len(pc) == 0 {
			t.Fatal("postcode search", e)
		}
		status, body = request("/v1/autocomplete?q=bug")
		if status != 200 || len(body["data"].([]any)) == 0 {
			t.Fatal(body)
		}
		status, _ = request("/v1/search?q=%27%3B--")
		if status != 400 {
			t.Fatal(status)
		}
	})
	t.Run("pagination and validation", func(t *testing.T) {
		status, body := request("/v1/wards/buguruni/streets?limit=1&offset=1")
		if status != 200 || len(body["data"].([]any)) != 1 || body["meta"].(map[string]any)["total"] != float64(1) {
			t.Fatal(body)
		}
		for _, path := range []string{"/v1/wards/buguruni?region_slug=BAD", "/v1/districts/ilala?region_slug=", "/v1/wards/buguruni/streets?offset=Infinity"} {
			status, body = request(path)
			if status != 400 {
				t.Fatal(path, body)
			}
		}
	})
	t.Run("FK and uniqueness enforcement", func(t *testing.T) {
		for _, sql := range []string{`INSERT INTO locations.districts VALUES(900001,900001,'Bad','bad')`, `INSERT INTO locations.wards VALUES(900001,1,'Duplicate','akheri','23306')`, `INSERT INTO locations.streets VALUES(900001,1,'Invalid','123')`} {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, sql); err == nil {
				t.Error("invalid row accepted", sql)
			}
			_ = tx.Rollback(ctx)
		}
	})
	t.Run("replacement refusal and rollback", func(t *testing.T) {
		original := d.Fingerprint
		d.Fingerprint = "different"
		defer func() { d.Fingerprint = original }()
		if err := database.Import(ctx, pool, d, "changed", false); err == nil {
			t.Fatal("different import accepted without --replace")
		}
		// Failure happens after replacement TRUNCATE and COPY have started; the entire
		// previous dataset and its provenance must survive rollback.
		_, err := pool.Exec(ctx, `CREATE FUNCTION locations.reject_import() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected import failure'; END $$; CREATE TRIGGER reject_import BEFORE INSERT ON locations.search_documents FOR EACH ROW EXECUTE FUNCTION locations.reject_import()`)
		if err != nil {
			t.Fatal(err)
		}
		err = database.Import(ctx, pool, d, "changed", true)
		if _, cleanupErr := pool.Exec(ctx, `DROP TRIGGER reject_import ON locations.search_documents; DROP FUNCTION locations.reject_import()`); cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
		if err == nil {
			t.Fatal("injected failure was ignored")
		}
		stats, e := repo.Stats(ctx)
		if e != nil || stats != d.Stats() {
			t.Fatal("partial import", stats, e)
		}
		var fingerprint string
		if e = pool.QueryRow(ctx, "SELECT fingerprint FROM locations.dataset_version").Scan(&fingerprint); e != nil || fingerprint != original {
			t.Fatal("provenance changed", e)
		}
		// Successful explicit replacement preserves IDs and restores the pinned data.
		if err = database.Import(ctx, pool, d, "changed", true); err != nil {
			t.Fatal(err)
		}
		d.Fingerprint = original
		if err = database.Import(ctx, pool, d, data.SourceCommit, true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("malformed data and corrupted index", func(t *testing.T) {
		copy := *d
		copy.Streets = append([]data.Street(nil), d.Streets...)
		copy.Streets[0].WardSlug = "absent"
		if err := database.Import(ctx, pool, &copy, "invalid", true); err == nil {
			t.Fatal("orphan accepted")
		}
		if _, err := pool.Exec(ctx, "DELETE FROM locations.search_documents WHERE type='street' AND entity_id=1"); err != nil {
			t.Fatal(err)
		}
		if err := database.Import(ctx, pool, d, data.SourceCommit, false); err == nil {
			t.Fatal("index corruption missed")
		}
		if _, err := pool.Exec(ctx, "UPDATE locations.dataset_version SET fingerprint='repair-required'"); err != nil {
			t.Fatal(err)
		}
		if err := database.Import(ctx, pool, d, data.SourceCommit, true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cancelled operations", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := repo.Stats(cancelled)
		if err == nil {
			t.Fatal("cancellation ignored")
		}
		if !errors.Is(err, context.Canceled) {
			t.Logf("driver wrapped cancellation: %v", err)
		}
	})
	t.Run("database errors are sanitized", func(t *testing.T) {
		broken := handlers.New(&repository.Repository{Pool: pool}, middleware.Options{})
		if _, err := pool.Exec(ctx, "ALTER TABLE locations.regions RENAME TO regions_unavailable"); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		broken.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/regions", nil))
		_, err := pool.Exec(ctx, "ALTER TABLE locations.regions_unavailable RENAME TO regions")
		if err != nil {
			t.Fatal(err)
		}
		if rr.Code != 500 || strings.Contains(rr.Body.String(), "SELECT") || !strings.Contains(rr.Body.String(), "INTERNAL_ERROR") {
			t.Fatal(rr)
		}
	})
	t.Run("read only transactions", func(t *testing.T) {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "DELETE FROM locations.streets"); err == nil {
			t.Fatal("read-only transaction wrote data")
		}
	})
}
