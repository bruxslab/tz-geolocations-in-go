package repository

import (
	"context"

	"github.com/bruxslab/tz-geolocations-in-go/data"
	"github.com/bruxslab/tz-geolocations-in-go/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ Pool *pgxpool.Pool }

func (r *Repository) Regions(ctx context.Context) ([]models.Region, error) {
	rows, err := r.Pool.Query(ctx, `SELECT r.id,r.name,r.slug,(SELECT count(*) FROM locations.districts d WHERE d.region_id=r.id) FROM locations.regions r ORDER BY r.name,r.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.Region, error) {
		var v models.Region
		err := row.Scan(&v.ID, &v.Name, &v.Slug, &v.DistrictCount)
		return v, err
	})
}
func (r *Repository) Region(ctx context.Context, slug string) (models.Region, error) {
	var v models.Region
	err := r.Pool.QueryRow(ctx, `SELECT r.id,r.name,r.slug,
 (SELECT count(*) FROM locations.districts d WHERE d.region_id=r.id),
 (SELECT count(*) FROM locations.wards w JOIN locations.districts d ON d.id=w.district_id WHERE d.region_id=r.id),
 (SELECT count(*) FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id JOIN locations.districts d ON d.id=w.district_id WHERE d.region_id=r.id)
 FROM locations.regions r WHERE r.slug=$1`, slug).Scan(&v.ID, &v.Name, &v.Slug, &v.DistrictCount, &v.WardCount, &v.StreetCount)
	return v, err
}
func (r *Repository) Districts(ctx context.Context, region int) ([]models.District, error) {
	rows, err := r.Pool.Query(ctx, `SELECT d.id,d.name,d.slug,(SELECT count(*) FROM locations.wards w WHERE w.district_id=d.id) FROM locations.districts d WHERE d.region_id=$1 ORDER BY d.name,d.id`, region)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.District, error) {
		var v models.District
		err := row.Scan(&v.ID, &v.Name, &v.Slug, &v.WardCount)
		return v, err
	})
}
func (r *Repository) District(ctx context.Context, slug, region string) (models.District, error) {
	var v models.District
	err := r.Pool.QueryRow(ctx, `SELECT d.id,d.name,d.slug,r.name,r.slug,
 (SELECT count(*) FROM locations.wards w WHERE w.district_id=d.id),
 (SELECT count(*) FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id WHERE w.district_id=d.id)
 FROM locations.districts d JOIN locations.regions r ON r.id=d.region_id WHERE d.slug=$1 AND ($2='' OR r.slug=$2) ORDER BY d.id LIMIT 1`, slug, region).Scan(&v.ID, &v.Name, &v.Slug, &v.RegionName, &v.RegionSlug, &v.WardCount, &v.StreetCount)
	return v, err
}
func (r *Repository) Wards(ctx context.Context, district int) ([]models.Ward, error) {
	rows, err := r.Pool.Query(ctx, `SELECT w.id,w.name,w.slug,w.postcode,(SELECT count(*) FROM locations.streets s WHERE s.ward_id=w.id) FROM locations.wards w WHERE w.district_id=$1 ORDER BY w.name,w.id`, district)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.Ward, error) {
		var v models.Ward
		err := row.Scan(&v.ID, &v.Name, &v.Slug, &v.Postcode, &v.StreetCount)
		return v, err
	})
}

const wardSelect = `SELECT w.id,w.name,w.slug,w.postcode,d.name,d.slug,r.name,r.slug,(SELECT count(*) FROM locations.streets s WHERE s.ward_id=w.id) FROM locations.wards w JOIN locations.districts d ON d.id=w.district_id JOIN locations.regions r ON r.id=d.region_id `

func scanWard(row pgx.Row) (models.Ward, error) {
	var v models.Ward
	err := row.Scan(&v.ID, &v.Name, &v.Slug, &v.Postcode, &v.DistrictName, &v.DistrictSlug, &v.RegionName, &v.RegionSlug, &v.StreetCount)
	return v, err
}
func (r *Repository) Ward(ctx context.Context, slug, region, district string) (models.Ward, error) {
	return scanWard(r.Pool.QueryRow(ctx, wardSelect+`WHERE w.slug=$1 AND ($2='' OR r.slug=$2) AND ($3='' OR d.slug=$3) ORDER BY w.id LIMIT 1`, slug, region, district))
}
func (r *Repository) WardByPostcode(ctx context.Context, postcode string) (models.Ward, error) {
	return scanWard(r.Pool.QueryRow(ctx, wardSelect+`WHERE w.postcode=$1 ORDER BY w.id LIMIT 1`, postcode))
}

const streetSelect = `SELECT s.id,s.name,s.postcode,w.name,w.slug,w.postcode,d.name,d.slug,r.name,r.slug FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id JOIN locations.districts d ON d.id=w.district_id JOIN locations.regions r ON r.id=d.region_id `

func scanStreet(row pgx.Row) (models.Street, error) {
	var v models.Street
	err := row.Scan(&v.ID, &v.Name, &v.Postcode, &v.WardName, &v.WardSlug, &v.WardPostcode, &v.DistrictName, &v.DistrictSlug, &v.RegionName, &v.RegionSlug)
	return v, err
}
func (r *Repository) Street(ctx context.Context, id int) (models.Street, error) {
	return scanStreet(r.Pool.QueryRow(ctx, streetSelect+`WHERE s.id=$1`, id))
}
func (r *Repository) Streets(ctx context.Context, ward, limit, offset int) ([]models.Street, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,name,postcode FROM locations.streets WHERE ward_id=$1 ORDER BY name,id LIMIT $2 OFFSET $3`, ward, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.Street, error) {
		var v models.Street
		err := row.Scan(&v.ID, &v.Name, &v.Postcode)
		return v, err
	})
}
func (r *Repository) StreetsByPostcode(ctx context.Context, postcode string) ([]models.Street, error) {
	rows, err := r.Pool.Query(ctx, `SELECT s.id,s.name,w.name,w.slug FROM locations.streets s JOIN locations.wards w ON w.id=s.ward_id WHERE s.postcode=$1 ORDER BY s.name,s.id`, postcode)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.Street, error) {
		var v models.Street
		err := row.Scan(&v.ID, &v.Name, &v.WardName, &v.WardSlug)
		return v, err
	})
}
func (r *Repository) Stats(ctx context.Context) (data.Stats, error) {
	var v data.Stats
	err := r.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM locations.regions),(SELECT count(*) FROM locations.districts),(SELECT count(*) FROM locations.wards),(SELECT count(*) FROM locations.streets)`).Scan(&v.Regions, &v.Districts, &v.Wards, &v.Streets)
	return v, err
}
func (r *Repository) Search(ctx context.Context, query, kind string, limit int, prefix bool) ([]models.SearchResult, error) {
	fn := "phraseto_tsquery"
	if prefix {
		fn = "to_tsquery"
	}
	// fn is selected internally, never interpolated from request input.
	rows, err := r.Pool.Query(ctx, `WITH q AS (SELECT `+fn+`('simple',$1) AS query) SELECT name,type,postcode,full_path,entity_id FROM locations.search_documents,q WHERE document @@ q.query AND ($2='' OR type=$2) ORDER BY ts_rank_cd(document,q.query) DESC,name,type,entity_id LIMIT $3`, query, kind, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.SearchResult, error) {
		var v models.SearchResult
		err := row.Scan(&v.Name, &v.Type, &v.Postcode, &v.FullPath, &v.EntityID)
		return v, err
	})
}
