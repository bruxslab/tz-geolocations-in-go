package integration_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/bruxslab/tz-geolocations-in-go/internal/handlers"
	"github.com/bruxslab/tz-geolocations-in-go/internal/middleware"
	"github.com/bruxslab/tz-geolocations-in-go/internal/repository"
	"github.com/bruxslab/tz-geolocations-in-go/internal/testdb"
)

func TestRegionScopedSearch(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	d, err := data.Pinned()
	if err != nil {
		t.Fatal(err)
	}
	repo := &repository.Repository{Pool: pool}
	h := handlers.New(repo, middleware.Options{RateRequests: 10000})
	t.Run("region scope covers every document", func(t *testing.T) {
		var mismatches int
		err := pool.QueryRow(ctx, `WITH expected AS (
   SELECT 'region' AS type,id AS entity_id,id AS region_id FROM locations.regions
   UNION ALL SELECT 'district',id,region_id FROM locations.districts
   UNION ALL SELECT 'ward',w.id,d.region_id FROM locations.wards w JOIN locations.districts d ON d.id=w.district_id
   UNION ALL SELECT 'street',s.id,d.region_id FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id JOIN locations.districts d ON d.id=w.district_id
  ) SELECT count(*) FROM locations.search_documents doc FULL JOIN expected e USING(type,entity_id)
    WHERE doc.region_scope_id IS DISTINCT FROM e.region_id`).Scan(&mismatches)
		if err != nil || mismatches != 0 {
			t.Fatalf("incorrect containing regions: %d, %v", mismatches, err)
		}
	})
	t.Run("all entity types use actual region relationships", func(t *testing.T) {
		for _, kind := range []string{"region", "district", "ward", "street"} {
			rows, err := repo.Search(ctx, "shinyanga", kind, "shinyanga", 50, false)
			if err != nil || len(rows) == 0 {
				t.Fatal(kind, err)
			}
			for _, r := range rows {
				var region string
				switch kind {
				case "region":
					region = d.Regions[r.EntityID-1].Slug
				case "district":
					region = d.Districts[r.EntityID-1].RegionSlug
				case "ward":
					region = d.Wards[r.EntityID-1].RegionSlug
				case "street":
					region = d.Streets[r.EntityID-1].RegionSlug
				}
				if region != "shinyanga" || r.Type != kind {
					t.Fatalf("leaked %s result: %+v in %s", kind, r, region)
				}
			}
		}
	})
	t.Run("ward matches include its streets and repeated names", func(t *testing.T) {
		rows, err := repo.Search(ctx, "ibadakuli", "street", "shinyanga", 50, false)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[int]bool{}
		for _, r := range rows {
			ids[r.EntityID] = true
			if d.Streets[r.EntityID-1].RegionSlug != "shinyanga" {
				t.Fatal("cross-region match", r)
			}
		}
		for _, id := range []int{58047, 58054, 56448} {
			if !ids[id] {
				t.Errorf("missing expected Airport/ward-name/repeated-street match: %d", id)
			}
		}
	})
	t.Run("HTTP filtering and prefix suggestions", func(t *testing.T) {
		for _, path := range []string{"/v1/search?q=ibadakuli&type=street&region_slug=shinyanga", "/v1/autocomplete?q=ibad&type=street&region_slug=shinyanga"} {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
			if rr.Code != 200 {
				t.Fatal(path, rr.Code, rr.Body)
			}
			var body struct {
				Data []struct {
					Name     string `json:"name"`
					Type     string `json:"type"`
					Postcode string `json:"postcode"`
					FullPath string `json:"full_path"`
				}
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Data) == 0 {
				t.Fatal("empty address suggestions")
			}
			for _, r := range body.Data {
				if r.Type != "street" || !strings.HasSuffix(r.FullPath, ", Shinyanga") || len(r.Postcode) != 5 {
					t.Fatal("invalid checkout suggestion", r)
				}
			}
		}
	})
	t.Run("empty and invalid filters", func(t *testing.T) {
		for _, endpoint := range []string{"search", "autocomplete"} {
			for _, filter := range []string{"", "SHINYANGA", "shinyanga%27%20OR%201%3D1--"} {
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/"+endpoint+"?q=ibadakuli&region_slug="+filter, nil))
				if rr.Code != 400 {
					t.Fatal(endpoint, filter, rr.Code, rr.Body)
				}
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/"+endpoint+"?q=ibadakuli&region_slug=unknown-region", nil))
			if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"data":[]`) {
				t.Fatal("unknown region should have no matches", rr.Code, rr.Body)
			}
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/autocomplete?q=ibad&type=invalid&region_slug=shinyanga", nil))
		if rr.Code != 400 {
			t.Fatal("invalid autocomplete type accepted", rr.Code)
		}
	})
}
