-- Keep entity-specific region_id intact; every search document also needs its
-- containing region, independent of its administrative level.
ALTER TABLE locations.search_documents
 ADD COLUMN region_scope_id integer REFERENCES locations.regions(id);

UPDATE locations.search_documents SET region_scope_id = region_id
 WHERE type = 'region';
UPDATE locations.search_documents doc SET region_scope_id = d.region_id
 FROM locations.districts d WHERE doc.district_id = d.id;
UPDATE locations.search_documents doc SET region_scope_id = d.region_id
 FROM locations.wards w JOIN locations.districts d ON d.id = w.district_id
 WHERE doc.ward_id = w.id;
UPDATE locations.search_documents doc SET region_scope_id = d.region_id
 FROM locations.streets s
 JOIN locations.wards w ON w.id = s.ward_id
 JOIN locations.districts d ON d.id = w.district_id
 WHERE doc.street_id = s.id;

ALTER TABLE locations.search_documents ALTER COLUMN region_scope_id SET NOT NULL;
CREATE INDEX search_region_type_idx
 ON locations.search_documents(region_scope_id, type);
ANALYZE locations.search_documents;
