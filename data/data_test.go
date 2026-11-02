package data

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPinned(t *testing.T) {
	d, err := Pinned()
	if err != nil {
		t.Fatal(err)
	}
	if d.Stats() != (Stats{31, 168, 4054, 75061}) {
		t.Fatal(d.Stats())
	}
	if len(d.Fingerprint) != 64 {
		t.Fatal("missing fingerprint")
	}
}
func TestValidationReportsAllErrors(t *testing.T) {
	d := Dataset{Regions: []Region{{Name: "A", Slug: "a"}, {Name: "Other", Slug: "a"}}, Districts: []District{{Name: "D", Slug: "d", RegionSlug: "absent"}}, Wards: []Ward{{Name: "W", Slug: "w", RegionSlug: "a", DistrictSlug: "missing", Postcode: "123"}}, Streets: []Street{{Name: "S", RegionSlug: "a", DistrictSlug: "d", WardSlug: "missing", Postcode: "12345"}}}
	err := d.Validate()
	if err == nil {
		t.Fatal("accepted invalid dataset")
	}
	for _, s := range []string{"duplicate key", "missing region", "missing district", "invalid postcode", "missing ward"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("missing %q: %v", s, err)
		}
	}
}
func TestReadFingerprintAndMalformedInput(t *testing.T) {
	d, err := Pinned()
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for name, rows := range map[string]any{"regions.json": d.Regions, "districts.json": d.Districts, "wards.json": d.Wards, "streets.json": d.Streets} {
		b, _ := json.Marshal(rows)
		files[name] = &fstest.MapFile{Data: b}
	}
	first, err := Read(files)
	if err != nil {
		t.Fatal(err)
	}
	files["regions.json"].Data = append(files["regions.json"].Data, ' ')
	second, err := Read(files)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("byte changes must change provenance fingerprint")
	}
	files["wards.json"].Data = []byte(`{"wrong":"shape"}`)
	delete(files, "streets.json")
	if _, err = Read(files); err == nil || !strings.Contains(err.Error(), "wards.json") || !strings.Contains(err.Error(), "streets.json") {
		t.Fatal(err)
	}
}
