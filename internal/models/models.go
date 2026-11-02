// Package models holds database read models; response shapes remain in handlers.
package models

type Region struct {
	ID                                    int
	Name, Slug                            string
	DistrictCount, WardCount, StreetCount int
}
type District struct {
	ID                     int
	Name, Slug             string
	RegionName, RegionSlug string
	WardCount, StreetCount int
}
type Ward struct {
	ID                                                 int
	Name, Slug, Postcode                               string
	DistrictName, DistrictSlug, RegionName, RegionSlug string
	StreetCount                                        int
}
type Street struct {
	ID                                                                                   int
	Name, Postcode                                                                       string
	WardName, WardSlug, WardPostcode, DistrictName, DistrictSlug, RegionName, RegionSlug string
}
type SearchResult struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Postcode *string `json:"postcode"`
	FullPath string  `json:"full_path"`
	EntityID int     `json:"entity_id"`
}
