// Package locations provides immutable, offline access to Tanzania's pinned
// administrative dataset. No server or database is required. Results retain
// source array order; returned slices and pointers are safe for callers to modify.
package locations

import (
	"strings"
	"sync"

	"github.com/bruxslab/tz-geolocations-in-go/data"
)

type Region = data.Region
type District = data.District
type Ward = data.Ward
type Street = data.Street
type Stats = data.Stats
type PostcodeLookupResult struct {
	Ward    *Ward    `json:"ward"`
	Streets []Street `json:"streets"`
}
type SearchOptions struct {
	Type  string
	Limit int
}
type Location struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Slug         string `json:"slug,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	WardSlug     string `json:"ward_slug,omitempty"`
	DistrictSlug string `json:"district_slug,omitempty"`
	RegionSlug   string `json:"region_slug,omitempty"`
}

var load = sync.OnceValue(func() *index {
	d, err := data.Pinned()
	if err != nil {
		panic("invalid embedded Tanzania dataset: " + err.Error())
	}
	x := &index{data: d, districts: map[string][]District{}, wards: map[string][]Ward{}, streets: map[string][]Street{}, postcodes: map[string][]Street{}, ward: map[string]Ward{}}
	for _, r := range d.Districts {
		x.districts[r.RegionSlug] = append(x.districts[r.RegionSlug], r)
	}
	for _, r := range d.Wards {
		x.wards[data.DistrictKey(r.RegionSlug, r.DistrictSlug)] = append(x.wards[data.DistrictKey(r.RegionSlug, r.DistrictSlug)], r)
		x.ward[data.WardKey(r.RegionSlug, r.DistrictSlug, r.Slug)] = r
	}
	for _, r := range d.Streets {
		k := data.WardKey(r.RegionSlug, r.DistrictSlug, r.WardSlug)
		x.streets[k] = append(x.streets[k], r)
		x.postcodes[r.Postcode] = append(x.postcodes[r.Postcode], r)
	}
	return x
})

type index struct {
	data      *data.Dataset
	districts map[string][]District
	wards     map[string][]Ward
	streets   map[string][]Street
	postcodes map[string][]Street
	ward      map[string]Ward
}

func clone[T any](rows []T) []T { return append(make([]T, 0, len(rows)), rows...) }
func GetRegions() []Region      { return clone(load().data.Regions) }
func GetRegion(slug string) (Region, bool) {
	for _, r := range load().data.Regions {
		if r.Slug == slug {
			return r, true
		}
	}
	return Region{}, false
}

// GetDistricts accepts an optional region slug; an empty value returns all districts.
func GetDistricts(region ...string) []District {
	if len(region) > 0 && region[0] != "" {
		return clone(load().districts[region[0]])
	}
	return clone(load().data.Districts)
}
func GetDistrictsByRegion(region string) []District { return GetDistricts(region) }

// GetDistrict and GetWard select the first source match unless parents are supplied.
func GetDistrict(slug string, region ...string) (District, bool) {
	for _, r := range load().data.Districts {
		if r.Slug == slug && (len(region) == 0 || region[0] == "" || r.RegionSlug == region[0]) {
			return r, true
		}
	}
	return District{}, false
}

// GetWards filters only when both region and district are nonempty, as in the npm package.
func GetWards(parents ...string) []Ward {
	if len(parents) >= 2 && parents[0] != "" && parents[1] != "" {
		return clone(load().wards[data.DistrictKey(parents[0], parents[1])])
	}
	return clone(load().data.Wards)
}
func GetWardsByDistrict(region, district string) []Ward { return GetWards(region, district) }
func GetWard(slug string, parents ...string) (Ward, bool) {
	for _, r := range load().data.Wards {
		if r.Slug == slug && (len(parents) == 0 || parents[0] == "" || r.RegionSlug == parents[0]) && (len(parents) < 2 || parents[1] == "" || r.DistrictSlug == parents[1]) {
			return r, true
		}
	}
	return Ward{}, false
}
func GetStreets(region, district, ward string) []Street {
	return clone(load().streets[data.WardKey(region, district, ward)])
}
func GetByPostcode(postcode string) PostcodeLookupResult {
	rows := load().postcodes[postcode]
	result := PostcodeLookupResult{Streets: clone(rows)}
	if len(rows) > 0 {
		r := rows[0]
		if w, ok := load().ward[data.WardKey(r.RegionSlug, r.DistrictSlug, r.WardSlug)]; ok {
			result.Ward = &w
		}
	}
	return result
}

// SearchLocations uses case-insensitive name substrings, in entity/source order.
// Omitted options use limit 10; an explicit nonpositive limit yields no results.
func SearchLocations(query string, options ...SearchOptions) []Location {
	opt := SearchOptions{Limit: 10}
	if len(options) > 0 {
		opt = options[0]
	}
	out := []Location{}
	q := strings.ToLower(query)
	add := func(r Location) {
		if len(out) < opt.Limit && (opt.Type == "" || opt.Type == r.Type) && strings.Contains(strings.ToLower(r.Name), q) {
			out = append(out, r)
		}
	}
	if opt.Limit <= 0 {
		return out
	}
	for _, r := range load().data.Regions {
		add(Location{Type: "region", Name: r.Name, Slug: r.Slug})
	}
	for _, r := range load().data.Districts {
		add(Location{Type: "district", Name: r.Name, Slug: r.Slug, RegionSlug: r.RegionSlug})
	}
	for _, r := range load().data.Wards {
		add(Location{Type: "ward", Name: r.Name, Slug: r.Slug, DistrictSlug: r.DistrictSlug, RegionSlug: r.RegionSlug})
	}
	for _, r := range load().data.Streets {
		add(Location{Type: "street", Name: r.Name, Postcode: r.Postcode, WardSlug: r.WardSlug, DistrictSlug: r.DistrictSlug, RegionSlug: r.RegionSlug})
	}
	return out
}
func GetStats() Stats { return load().data.Stats() }
