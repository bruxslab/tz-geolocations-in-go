package locations

import (
	"sync"
	"testing"
)

func TestGetters(t *testing.T) {
	if GetStats() != (Stats{Regions: 31, Districts: 168, Wards: 4054, Streets: 75061}) {
		t.Fatal(GetStats())
	}
	if _, ok := GetRegion("unknown"); ok {
		t.Fatal("unexpected region")
	}
	if r, ok := GetRegion("arusha"); !ok || r.Name != "Arusha" {
		t.Fatal(r, ok)
	}
	if len(GetDistricts()) != 168 || len(GetDistrictsByRegion("dar-es-salaam")) == 0 || len(GetDistricts("unknown")) != 0 {
		t.Fatal("district filtering")
	}
	if _, ok := GetDistrict("ilala", "arusha"); ok {
		t.Fatal("parent ignored")
	}
	if len(GetWards("dar-es-salaam")) != 4054 {
		t.Fatal("partial parents must preserve npm behavior")
	}
	if len(GetWardsByDistrict("dar-es-salaam", "ilala")) == 0 {
		t.Fatal("ward filtering")
	}
	if len(GetStreets("dar-es-salaam", "ilala", "buguruni")) == 0 {
		t.Fatal("streets")
	}
	pc := GetByPostcode("12102")
	if pc.Ward == nil || pc.Ward.Slug != "buguruni" || len(pc.Streets) == 0 {
		t.Fatal(pc)
	}
	if pc = GetByPostcode("99999"); pc.Ward != nil || pc.Streets == nil || len(pc.Streets) != 0 {
		t.Fatal(pc)
	}
	seen := map[string]bool{}
	for _, w := range GetWards() {
		if w.Slug == "maweni" {
			seen[w.RegionSlug+"/"+w.DistrictSlug] = true
			got, ok := GetWard("maweni", w.RegionSlug, w.DistrictSlug)
			if !ok || got != w {
				t.Fatal("parent-aware lookup failed")
			}
		}
	}
	if len(seen) < 2 {
		t.Fatal("fixture should exercise ambiguity")
	}
}
func TestCopiesAndConcurrentCalls(t *testing.T) {
	rows := GetRegions()
	rows[0].Name = "corrupted"
	if GetRegions()[0].Name == "corrupted" {
		t.Fatal("shared mutable data")
	}
	pc := GetByPostcode("12102")
	pc.Ward.Name = "corrupted"
	pc.Streets[0].Name = "corrupted"
	if GetByPostcode("12102").Ward.Name == "corrupted" || GetByPostcode("12102").Streets[0].Name == "corrupted" {
		t.Fatal("postcode result aliases embedded data")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				_ = GetRegions()
				_ = GetByPostcode("12102")
				_ = SearchLocations("buguruni")
			}
		})
	}
	wg.Wait()
}
func TestSearch(t *testing.T) {
	a := SearchLocations("BUGURUNI")
	b := SearchLocations("buguruni")
	if len(a) == 0 || len(a) != len(b) {
		t.Fatal("case folding")
	}
	for _, r := range SearchLocations("a", SearchOptions{Type: "street", Limit: 3}) {
		if r.Type != "street" {
			t.Fatal(r)
		}
	}
	if len(SearchLocations("", SearchOptions{Limit: 0})) != 0 {
		t.Fatal("explicit zero limit")
	}
	if got := SearchLocations("", SearchOptions{Limit: 1}); len(got) != 1 || got[0].Type != "region" {
		t.Fatal(got)
	}
}
