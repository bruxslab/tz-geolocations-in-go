package contract_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/bruxslab/tz-geolocations-in-go/internal/handlers"
	"github.com/bruxslab/tz-geolocations-in-go/internal/middleware"
	"github.com/bruxslab/tz-geolocations-in-go/internal/repository"
	"github.com/bruxslab/tz-geolocations-in-go/internal/testdb"
	"github.com/bruxslab/tz-geolocations-in-go/pkg/locations"
)

func fixture(t *testing.T, name string, dst any) {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, dst); err != nil {
		t.Fatal(err)
	}
}
func canonical(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestReferenceAPI(t *testing.T) {
	var f struct {
		Commit string `json:"source_commit"`
		Cases  []struct {
			Path         string
			Status       int
			Comparison   string
			Body         any
			CacheControl *string `json:"cache_control"`
		}
	}
	fixture(t, "reference.json", &f)
	if f.Commit != data.SourceCommit {
		t.Fatal("wrong baseline")
	}
	pool := testdb.Open(t)
	h := handlers.New(&repository.Repository{Pool: pool}, middleware.Options{RateRequests: 10000})
	observed := []map[string]any{}
	t.Cleanup(func() {
		if path := os.Getenv("CONTRACT_RESULTS_PATH"); path != "" {
			b, err := json.MarshalIndent(map[string]any{"source_commit": data.SourceCommit, "cases": observed}, "", "  ")
			if err != nil {
				t.Error(err)
				return
			}
			if err = os.WriteFile(path, b, 0600); err != nil {
				t.Error(err)
			}
		}
	})
	for _, tc := range f.Cases {
		t.Run(tc.Path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", tc.Path, nil))
			if rr.Code != tc.Status {
				t.Fatalf("got %d %s; want %d", rr.Code, rr.Body, tc.Status)
			}
			var got any
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			observed = append(observed, map[string]any{"path": tc.Path, "status": rr.Code, "body": got})
			expected := tc.Body
			if strings.HasPrefix(tc.Path, "/v1/search") || strings.HasPrefix(tc.Path, "/v1/autocomplete") {
				// Explicitly approved presentation change; preserve the original
				// source fixture and compare everything else without modification.
				expected = canonical(t, tc.Body)
				if rows, ok := expected.(map[string]any)["data"].([]any); ok {
					for _, raw := range rows {
						row := raw.(map[string]any)
						if pc, ok := row["postcode"].(string); ok && pc != "" {
							row["full_path"] = pc + ", " + row["full_path"].(string)
						}
					}
				}
			}
			if tc.Comparison == "exact" && !reflect.DeepEqual(got, expected) {
				t.Fatalf("contract differs\ngot: %s\nwant: %s", rr.Body.String(), mustJSON(expected))
			}
			if tc.Comparison == "search" {
				checkSearch(t, got, tc.Body)
			}
			cache := ""
			if tc.CacheControl != nil {
				cache = *tc.CacheControl
			}
			if rr.Header().Get("Cache-Control") != cache {
				t.Errorf("cache header: %q != %q", rr.Header().Get("Cache-Control"), cache)
			}
		})
	}
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// Approved search differences retain result shapes/types and meta, while adding
// streets and changing full-text ranking. Exact cases above still compare results.
func checkSearch(t *testing.T, got, legacy any) {
	t.Helper()
	g := got.(map[string]any)
	l := legacy.(map[string]any)
	if (g["meta"] == nil) != (l["meta"] == nil) {
		t.Fatal("metadata shape changed")
	}
	rows, ok := g["data"].([]any)
	if !ok {
		t.Fatal("search data is not an array")
	}
	if len(l["data"].([]any)) > 0 && len(rows) == 0 {
		t.Fatal("search lost all legacy matches")
	}
	for _, raw := range rows {
		row := raw.(map[string]any)
		for _, key := range []string{"name", "type", "full_path"} {
			if _, ok := row[key].(string); !ok {
				t.Errorf("invalid %s: %v", key, row)
			}
		}
		if pc := row["postcode"]; pc != nil {
			if _, ok := pc.(string); !ok {
				t.Fatal("postcode must remain nullable string")
			}
		}
		if _, ok := row["postcode"]; !ok {
			t.Fatal("missing postcode")
		}
		if g["meta"] != nil {
			if _, ok := row["entity_id"].(float64); !ok {
				t.Fatal("missing entity_id")
			}
		} else if _, ok := row["entity_id"]; ok {
			t.Fatal("autocomplete must omit entity_id")
		}
	}
}

func TestReferenceOffline(t *testing.T) {
	var f struct {
		Commit string `json:"source_commit"`
		Cases  []struct {
			Fn     string
			Args   []json.RawMessage
			Result any
		}
	}
	fixture(t, "offline.json", &f)
	if f.Commit != data.SourceCommit {
		t.Fatal("wrong baseline")
	}
	for _, tc := range f.Cases {
		t.Run(tc.Fn+mustJSON(tc.Args), func(t *testing.T) {
			args := []string{}
			for _, a := range tc.Args {
				var s string
				if json.Unmarshal(a, &s) == nil {
					args = append(args, s)
				}
			}
			var got any
			switch tc.Fn {
			case "getRegions":
				got = locations.GetRegions()
			case "getRegion":
				v, ok := locations.GetRegion(args[0])
				if ok {
					got = v
				}
			case "getDistricts":
				got = locations.GetDistricts(args...)
			case "getDistrictsByRegion":
				got = locations.GetDistrictsByRegion(args[0])
			case "getDistrict":
				v, ok := locations.GetDistrict(args[0])
				if ok {
					got = v
				}
			case "getWards":
				got = locations.GetWards(args...)
			case "getWardsByDistrict":
				got = locations.GetWardsByDistrict(args[0], args[1])
			case "getWard":
				v, ok := locations.GetWard(args[0])
				if ok {
					got = v
				}
			case "getStreets":
				got = locations.GetStreets(args[0], args[1], args[2])
			case "getByPostcode":
				got = locations.GetByPostcode(args[0])
			case "searchLocations":
				if len(tc.Args) == 1 {
					got = locations.SearchLocations(args[0])
				} else {
					var opt struct {
						Type  string
						Limit int
					}
					if err := json.Unmarshal(tc.Args[1], &opt); err != nil {
						t.Fatal(err)
					}
					got = locations.SearchLocations(args[0], locations.SearchOptions{Type: opt.Type, Limit: opt.Limit})
				}
			case "getStats":
				got = locations.GetStats()
			default:
				t.Fatal(tc.Fn)
			}
			if !reflect.DeepEqual(canonical(t, got), tc.Result) {
				t.Fatalf("offline contract differs\ngot: %s\nwant: %s", mustJSON(got), mustJSON(tc.Result))
			}
		})
	}
}
