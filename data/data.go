// Package data contains the pinned, unmodified source dataset and its validator.
package data

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

const SourceCommit = "d835ea318dbb17f789d4b7195b600e2e07e20f1e"

//go:embed *.json
var Files embed.FS

type Region struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}
type District struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	RegionSlug string `json:"region_slug"`
}
type Ward struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	DistrictSlug string `json:"district_slug"`
	RegionSlug   string `json:"region_slug"`
	Postcode     string `json:"postcode"`
}
type Street struct {
	Name         string `json:"name"`
	Postcode     string `json:"postcode"`
	WardSlug     string `json:"ward_slug"`
	DistrictSlug string `json:"district_slug"`
	RegionSlug   string `json:"region_slug"`
}
type Stats struct {
	Regions   int `json:"regions"`
	Districts int `json:"districts"`
	Wards     int `json:"wards"`
	Streets   int `json:"streets"`
}
type Dataset struct {
	Regions     []Region
	Districts   []District
	Wards       []Ward
	Streets     []Street
	Fingerprint string
}
type Manifest struct {
	Source string `json:"source"`
	Commit string `json:"commit"`
	Files  map[string]struct {
		SHA256 string `json:"sha256"`
		Count  int    `json:"count"`
	} `json:"files"`
}

func (d *Dataset) Stats() Stats {
	return Stats{len(d.Regions), len(d.Districts), len(d.Wards), len(d.Streets)}
}
func DistrictKey(region, district string) string   { return region + "/" + district }
func WardKey(region, district, ward string) string { return region + "/" + district + "/" + ward }

// Read validates complete parent-qualified relationships before returning any records.
// Fingerprint covers the exact bytes, filenames and order of all four source files.
func Read(source fs.FS) (*Dataset, error) {
	d := new(Dataset)
	h := sha256.New()
	var problems []error
	for _, f := range []struct {
		name string
		dst  any
	}{{"regions.json", &d.Regions}, {"districts.json", &d.Districts}, {"wards.json", &d.Wards}, {"streets.json", &d.Streets}} {
		b, err := fs.ReadFile(source, f.name)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", f.name, err))
			continue
		}
		fmt.Fprintf(h, "%s:%d:", f.name, len(b))
		h.Write(b)
		if err = json.Unmarshal(b, f.dst); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", f.name, err))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return nil, err
	}
	d.Fingerprint = hex.EncodeToString(h.Sum(nil))
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return d, nil
}

// Pinned also checks provenance checksums and acceptance counts.
func Pinned() (*Dataset, error) {
	var m Manifest
	b, err := Files.ReadFile("manifest.json")
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Commit != SourceCommit {
		return nil, errors.New("dataset source commit mismatch")
	}
	for _, name := range []string{"regions.json", "districts.json", "wards.json", "streets.json"} {
		b, err := Files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		var rows []json.RawMessage
		if err = json.Unmarshal(b, &rows); err != nil {
			return nil, err
		}
		f := m.Files[name]
		if hex.EncodeToString(sum[:]) != f.SHA256 || len(rows) != f.Count {
			return nil, fmt.Errorf("%s: checksum/count mismatch", name)
		}
	}
	d, err := Read(Files)
	if err != nil {
		return nil, err
	}
	if d.Stats() != (Stats{31, 168, 4054, 75061}) {
		return nil, errors.New("pinned dataset counts changed")
	}
	return d, nil
}

var slugRE = regexp.MustCompile(`^[a-z0-9-]{1,100}$`)
var postcodeRE = regexp.MustCompile(`^[0-9]{5}$`)

// Validate reports all malformed records, duplicate parent keys and missing parents.
func (d *Dataset) Validate() error {
	var problems []error
	fail := func(format string, args ...any) { problems = append(problems, fmt.Errorf(format, args...)) }
	regions := map[string]bool{}
	districts := map[string]bool{}
	wards := map[string]bool{}
	streets := map[string]bool{}
	check := func(kind, key, name, slug string, seen map[string]bool) {
		if strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) {
			fail("%s %q: invalid name", kind, key)
		}
		if slug != "" && !slugRE.MatchString(slug) {
			fail("%s %q: invalid slug", kind, key)
		}
		if seen[key] {
			fail("%s %q: duplicate key", kind, key)
		}
		seen[key] = true
	}
	if len(d.Regions) == 0 || len(d.Districts) == 0 || len(d.Wards) == 0 || len(d.Streets) == 0 {
		fail("all four entity levels must be populated")
	}
	for _, r := range d.Regions {
		check("region", r.Slug, r.Name, r.Slug, regions)
		if r.Slug == "" {
			fail("region: missing slug")
		}
	}
	for _, r := range d.Districts {
		k := DistrictKey(r.RegionSlug, r.Slug)
		check("district", k, r.Name, r.Slug, districts)
		if r.Slug == "" {
			fail("district %q: missing slug", k)
		}
		if !regions[r.RegionSlug] {
			fail("district %q: missing region", k)
		}
	}
	for _, r := range d.Wards {
		k := WardKey(r.RegionSlug, r.DistrictSlug, r.Slug)
		check("ward", k, r.Name, r.Slug, wards)
		if r.Slug == "" {
			fail("ward %q: missing slug", k)
		}
		if !districts[DistrictKey(r.RegionSlug, r.DistrictSlug)] {
			fail("ward %q: missing district", k)
		}
		if !postcodeRE.MatchString(r.Postcode) {
			fail("ward %q: invalid postcode", k)
		}
	}
	for _, r := range d.Streets {
		k := WardKey(r.RegionSlug, r.DistrictSlug, r.WardSlug)
		check("street", k+"/"+r.Name, r.Name, "", streets)
		if !wards[k] {
			fail("street %q: missing ward", k+"/"+r.Name)
		}
		if !postcodeRE.MatchString(r.Postcode) {
			fail("street %q: invalid postcode", k+"/"+r.Name)
		}
	}
	return errors.Join(problems...)
}
