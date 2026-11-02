package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bruxslab/tz-geolocations-in-go/docs"
	"github.com/bruxslab/tz-geolocations-in-go/internal/middleware"
	"github.com/bruxslab/tz-geolocations-in-go/internal/models"
	"github.com/bruxslab/tz-geolocations-in-go/internal/repository"
	"github.com/bruxslab/tz-geolocations-in-go/internal/search"
	"github.com/bruxslab/tz-geolocations-in-go/internal/service"
	"github.com/jackc/pgx/v5"
)

type object = map[string]any
type apiError struct {
	status        int
	code, message string
}

func (e *apiError) Error() string  { return e.message }
func bad(message string) error     { return &apiError{400, "BAD_REQUEST", message} }
func missing(message string) error { return &apiError{404, "NOT_FOUND", message} }

type response struct {
	data any
	meta object
}
type endpoint func(*http.Request) (response, error)
type API struct {
	repo *repository.Repository
	log  *slog.Logger
}

func New(repo *repository.Repository, options middleware.Options) http.Handler {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	a := &API{repo, options.Logger}
	mux := http.NewServeMux()
	for path, fn := range map[string]endpoint{
		"/v1/regions": a.regions, "/v1/regions/{slug}": a.region, "/v1/regions/{slug}/districts": a.districts,
		"/v1/districts/{slug}": a.district, "/v1/districts/{slug}/wards": a.wards,
		"/v1/wards/{slug}": a.ward, "/v1/wards/{slug}/streets": a.streets,
		"/v1/streets/{id}": a.street, "/v1/postcodes/{postcode}": a.postcode,
		"/v1/search": a.search, "/v1/autocomplete": a.autocomplete, "/v1/stats": a.stats,
	} {
		mux.HandleFunc(path, a.handle(fn))
	}
	mux.HandleFunc("/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			middleware.Error(w, 404, "NOT_FOUND", "Endpoint not found")
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(docs.OpenAPI)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || (r.Method != "GET" && r.Method != "HEAD") {
			middleware.Error(w, 404, "NOT_FOUND", "Endpoint not found")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		_ = json.NewEncoder(w).Encode(object{
			"name": "tz-locations-api", "description": "Tanzania Locations API — Regions, Districts, Wards, Streets & Postcodes", "version": "1.0.0", "docs": "/v1/docs",
			"endpoints": object{"regions": "/v1/regions", "districts": "/v1/districts/:slug", "wards": "/v1/wards/:slug", "streets": "/v1/streets/:id", "postcodes": "/v1/postcodes/:postcode", "search": "/v1/search?q=query", "autocomplete": "/v1/autocomplete?q=query", "stats": "/v1/stats"},
		})
	})
	return middleware.Wrap(mux, options)
}

func (a *API) handle(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			middleware.Error(w, 404, "NOT_FOUND", "Endpoint not found")
			return
		}
		result, err := fn(r)
		if err != nil {
			var e *apiError
			if errors.As(err, &e) {
				middleware.Error(w, e.status, e.code, e.message)
			} else {
				a.log.Error("request failed", "path", r.URL.Path, "error_type", fmt.Sprintf("%T", err))
				middleware.Error(w, 500, "INTERNAL_ERROR", "An unexpected error occurred")
			}
			return
		}
		body := object{"data": result.data}
		if result.meta != nil {
			body["meta"] = result.meta
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			middleware.Error(w, 500, "INTERNAL_ERROR", "An unexpected error occurred")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(encoded)
	}
}
func params(r *http.Request) (string, string, string, error) {
	slug := r.PathValue("slug")
	if !service.SlugPattern.MatchString(slug) {
		return "", "", "", bad("Invalid slug format")
	}
	q := r.URL.Query()
	region, district := q.Get("region_slug"), q.Get("district_slug")
	if strings.HasPrefix(r.URL.Path, "/v1/regions/") {
		return slug, "", "", nil
	}
	keys := []string{"region_slug"}
	if strings.HasPrefix(r.URL.Path, "/v1/wards/") {
		keys = append(keys, "district_slug")
	}
	for _, key := range keys {
		if q.Has(key) && !service.SlugPattern.MatchString(q.Get(key)) {
			return "", "", "", bad("Invalid " + key + " format")
		}
	}
	return slug, region, district, nil
}
func parent(name, slug string) object { return object{"name": name, "slug": slug} }

// Once a request supplies parent context, carry the resolved full context on
// child links, including wards with the same name in multiple districts.
func parentContext(r *http.Request, region, district string) *http.Request {
	if r.URL.Query().Get("region_slug") == "" && r.URL.Query().Get("district_slug") == "" {
		return r
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	copy.URL = &u
	q := copy.URL.Query()
	q.Set("region_slug", region)
	q.Set("district_slug", district)
	copy.URL.RawQuery = q.Encode()
	return copy
}
func qualify(path string, r *http.Request, ward bool) string {
	if strings.HasPrefix(r.URL.Path, "/v1/regions/") {
		return path
	}
	q := url.Values{}
	for _, k := range []string{"region_slug", "district_slug"} {
		if k == "district_slug" && !ward {
			continue
		}
		if v := r.URL.Query().Get(k); v != "" {
			q.Set(k, v)
		}
	}
	if len(q) > 0 {
		return path + "?" + q.Encode()
	}
	return path
}
func regionJSON(v models.Region, detail bool) object {
	out := object{"id": v.ID, "name": v.Name, "slug": v.Slug, "district_count": v.DistrictCount, "_links": object{"self": "/v1/regions/" + v.Slug, "districts": "/v1/regions/" + v.Slug + "/districts"}}
	if detail {
		out["ward_count"] = v.WardCount
		out["street_count"] = v.StreetCount
	}
	return out
}
func districtJSON(v models.District, r *http.Request, detail bool) object {
	out := object{"id": v.ID, "name": v.Name, "slug": v.Slug, "ward_count": v.WardCount, "region": parent(v.RegionName, v.RegionSlug), "_links": object{"self": qualify("/v1/districts/"+v.Slug, r, false), "wards": qualify("/v1/districts/"+v.Slug+"/wards", r, false), "region": "/v1/regions/" + v.RegionSlug}}
	if detail {
		out["street_count"] = v.StreetCount
	}
	return out
}
func wardJSON(v models.Ward, r *http.Request) object {
	r = parentContext(r, v.RegionSlug, v.DistrictSlug)
	return object{"id": v.ID, "name": v.Name, "slug": v.Slug, "postcode": v.Postcode, "street_count": v.StreetCount, "district": parent(v.DistrictName, v.DistrictSlug), "region": parent(v.RegionName, v.RegionSlug), "_links": object{"self": qualify("/v1/wards/"+v.Slug, r, true), "streets": qualify("/v1/wards/"+v.Slug+"/streets", r, true), "district": qualify("/v1/districts/"+v.DistrictSlug, r, false)}}
}
func streetJSON(v models.Street, r *http.Request) object {
	r = parentContext(r, v.RegionSlug, v.DistrictSlug)
	return object{"id": v.ID, "name": v.Name, "postcode": v.Postcode, "ward": object{"name": v.WardName, "slug": v.WardSlug, "postcode": v.WardPostcode}, "district": parent(v.DistrictName, v.DistrictSlug), "region": parent(v.RegionName, v.RegionSlug), "_links": object{"self": "/v1/streets/" + strconv.Itoa(v.ID), "ward": qualify("/v1/wards/"+v.WardSlug, r, true)}}
}
func (a *API) regions(r *http.Request) (response, error) {
	rows, err := a.repo.Regions(r.Context())
	if err != nil {
		return response{}, err
	}
	out := make([]object, 0, len(rows))
	for _, v := range rows {
		out = append(out, regionJSON(v, false))
	}
	return response{out, object{"total": len(out)}}, nil
}
func (a *API) region(r *http.Request) (response, error) {
	slug, _, _, err := params(r)
	if err != nil {
		return response{}, err
	}
	v, err := a.repo.Region(r.Context(), slug)
	if errors.Is(err, pgx.ErrNoRows) {
		err = missing("Region '" + slug + "' not found")
	}
	return response{regionJSON(v, true), nil}, err
}
func (a *API) districts(r *http.Request) (response, error) {
	slug, _, _, err := params(r)
	if err != nil {
		return response{}, err
	}
	p, err := a.repo.Region(r.Context(), slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return response{}, missing("Region '" + slug + "' not found")
	}
	if err != nil {
		return response{}, err
	}
	rows, err := a.repo.Districts(r.Context(), p.ID)
	if err != nil {
		return response{}, err
	}
	out := make([]object, 0, len(rows))
	for _, v := range rows {
		v.RegionName = p.Name
		v.RegionSlug = p.Slug
		out = append(out, districtJSON(v, r, false))
	}
	return response{out, object{"total": len(out), "region": p.Name}}, nil
}
func (a *API) district(r *http.Request) (response, error) {
	slug, region, _, err := params(r)
	if err != nil {
		return response{}, err
	}
	v, err := a.repo.District(r.Context(), slug, region)
	if errors.Is(err, pgx.ErrNoRows) {
		err = missing("District '" + slug + "' not found")
	}
	return response{districtJSON(v, r, true), nil}, err
}
func (a *API) wards(r *http.Request) (response, error) {
	slug, region, _, err := params(r)
	if err != nil {
		return response{}, err
	}
	p, err := a.repo.District(r.Context(), slug, region)
	if errors.Is(err, pgx.ErrNoRows) {
		return response{}, missing("District '" + slug + "' not found")
	}
	if err != nil {
		return response{}, err
	}
	rows, err := a.repo.Wards(r.Context(), p.ID)
	if err != nil {
		return response{}, err
	}
	out := make([]object, 0, len(rows))
	for _, v := range rows {
		v.RegionName = p.RegionName
		v.RegionSlug = p.RegionSlug
		v.DistrictName = p.Name
		v.DistrictSlug = p.Slug
		out = append(out, wardJSON(v, r))
	}
	return response{out, object{"total": len(out), "district": p.Name}}, nil
}
func (a *API) ward(r *http.Request) (response, error) {
	slug, region, district, err := params(r)
	if err != nil {
		return response{}, err
	}
	v, err := a.repo.Ward(r.Context(), slug, region, district)
	if errors.Is(err, pgx.ErrNoRows) {
		err = missing("Ward '" + slug + "' not found")
	}
	return response{wardJSON(v, r), nil}, err
}
func (a *API) streets(r *http.Request) (response, error) {
	slug, region, district, err := params(r)
	if err != nil {
		return response{}, err
	}
	p, err := a.repo.Ward(r.Context(), slug, region, district)
	if errors.Is(err, pgx.ErrNoRows) {
		return response{}, missing("Ward '" + slug + "' not found")
	}
	if err != nil {
		return response{}, err
	}
	limit, err := service.Pagination(r.URL.Query().Get("limit"), 100, 500)
	if err != nil {
		return response{}, err
	}
	offset, err := service.Offset(r.URL.Query().Get("offset"))
	if err != nil {
		if errors.Is(err, service.ErrFractional) {
			return response{}, err
		}
		return response{}, bad(err.Error())
	}
	rows, err := a.repo.Streets(r.Context(), p.ID, limit, offset)
	if err != nil {
		return response{}, err
	}
	out := make([]object, 0, len(rows))
	for _, v := range rows {
		v.WardName = p.Name
		v.WardSlug = p.Slug
		v.WardPostcode = p.Postcode
		v.DistrictName = p.DistrictName
		v.DistrictSlug = p.DistrictSlug
		v.RegionName = p.RegionName
		v.RegionSlug = p.RegionSlug
		out = append(out, streetJSON(v, r))
	}
	return response{out, object{"total": len(out), "ward": p.Name, "limit": limit, "offset": offset}}, nil
}
func (a *API) street(r *http.Request) (response, error) {
	raw := r.PathValue("id")
	n := service.Number(raw)
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 1 || math.Trunc(n) != n {
		return response{}, missing("Invalid street ID")
	}
	if n > 2147483647 {
		return response{}, missing("Street with ID " + strconv.FormatFloat(n, 'f', -1, 64) + " not found")
	}
	v, err := a.repo.Street(r.Context(), int(n))
	if errors.Is(err, pgx.ErrNoRows) {
		err = missing(fmt.Sprintf("Street with ID %d not found", int(n)))
	}
	return response{streetJSON(v, r), nil}, err
}
func (a *API) postcode(r *http.Request) (response, error) {
	pc := r.PathValue("postcode")
	if !service.PostcodePattern.MatchString(pc) {
		return response{}, bad("Postcode must be a 5-digit number")
	}
	p, err := a.repo.WardByPostcode(r.Context(), pc)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return response{}, err
	}
	var ward any
	if err == nil {
		ward = object{"id": p.ID, "name": p.Name, "slug": p.Slug, "district": parent(p.DistrictName, p.DistrictSlug), "region": parent(p.RegionName, p.RegionSlug)}
	}
	rows, err := a.repo.StreetsByPostcode(r.Context(), pc)
	if err != nil {
		return response{}, err
	}
	if ward == nil && len(rows) == 0 {
		return response{}, missing("No locations found for postcode " + pc)
	}
	out := make([]object, 0, len(rows))
	for _, v := range rows {
		out = append(out, object{"id": v.ID, "name": v.Name, "ward": parent(v.WardName, v.WardSlug)})
	}
	return response{object{"postcode": pc, "ward": ward, "streets": out}, nil}, nil
}
func (a *API) search(r *http.Request) (response, error)       { return a.find(r, false) }
func (a *API) autocomplete(r *http.Request) (response, error) { return a.find(r, true) }
func (a *API) find(r *http.Request, prefix bool) (response, error) {
	q := r.URL.Query().Get("q")
	kind := r.URL.Query().Get("type")
	def, maxLimit, minChars := 10, 50, 1
	if prefix {
		def, maxLimit, minChars = 5, 20, 2
	}
	if search.UTF16Length(q) < minChars {
		msg := "Query parameter 'q' is required"
		if prefix {
			msg += " (min 2 characters)"
		}
		return response{}, bad(msg)
	}
	if kind != "" && kind != "region" && kind != "district" && kind != "ward" && kind != "street" {
		return response{}, bad("Invalid type. Must be one of: region, district, ward, street")
	}
	region := r.URL.Query().Get("region_slug")
	if r.URL.Query().Has("region_slug") && !service.SlugPattern.MatchString(region) {
		return response{}, bad("Invalid region_slug format")
	}
	if len(q) > 4096 {
		return response{}, bad("Query is too long (maximum 4096 bytes)")
	}
	sanitized := search.Sanitize(q)
	if strings.TrimSpace(sanitized) == "" {
		return response{}, bad("Query must contain at least one alphanumeric character")
	}
	limit, err := service.Pagination(r.URL.Query().Get("limit"), def, maxLimit)
	if err != nil {
		return response{}, err
	}
	if prefix {
		sanitized = search.Prefix(sanitized)
	}
	rows, err := a.repo.Search(r.Context(), sanitized, kind, region, limit, prefix)
	if err != nil {
		return response{}, err
	}
	for i := range rows {
		if rows[i].Postcode != nil && *rows[i].Postcode != "" {
			rows[i].FullPath = *rows[i].Postcode + ", " + rows[i].FullPath
		}
	}
	if prefix {
		out := make([]object, 0, len(rows))
		for _, v := range rows {
			out = append(out, object{"name": v.Name, "type": v.Type, "postcode": v.Postcode, "full_path": v.FullPath})
		}
		return response{out, nil}, nil
	}
	if rows == nil {
		rows = []models.SearchResult{}
	}
	return response{rows, object{"total": len(rows), "query": q}}, nil
}
func (a *API) stats(r *http.Request) (response, error) {
	v, err := a.repo.Stats(r.Context())
	return response{v, nil}, err
}
