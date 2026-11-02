CREATE SCHEMA locations;

CREATE TABLE locations.regions (
 id integer PRIMARY KEY CHECK (id > 0),
 name text COLLATE "C" NOT NULL CHECK (btrim(name) <> ''),
 slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]{1,100}$')
);
CREATE TABLE locations.districts (
 id integer PRIMARY KEY CHECK (id > 0),
 region_id integer NOT NULL REFERENCES locations.regions(id),
 name text COLLATE "C" NOT NULL CHECK (btrim(name) <> ''),
 slug text NOT NULL CHECK (slug ~ '^[a-z0-9-]{1,100}$'),
 UNIQUE(region_id,slug)
);
CREATE TABLE locations.wards (
 id integer PRIMARY KEY CHECK (id > 0),
 district_id integer NOT NULL REFERENCES locations.districts(id),
 name text COLLATE "C" NOT NULL CHECK (btrim(name) <> ''),
 slug text NOT NULL CHECK (slug ~ '^[a-z0-9-]{1,100}$'),
 postcode text NOT NULL CHECK (postcode ~ '^[0-9]{5}$'),
 UNIQUE(district_id,slug)
);
CREATE TABLE locations.streets (
 id integer PRIMARY KEY CHECK (id > 0),
 ward_id integer NOT NULL REFERENCES locations.wards(id),
 name text COLLATE "C" NOT NULL CHECK (btrim(name) <> ''),
 postcode text NOT NULL CHECK (postcode ~ '^[0-9]{5}$'),
 UNIQUE(ward_id,name)
);
CREATE INDEX districts_slug_idx ON locations.districts(slug,id);
CREATE INDEX wards_slug_idx ON locations.wards(slug,id);
CREATE INDEX wards_postcode_idx ON locations.wards(postcode,id);
CREATE INDEX streets_postcode_idx ON locations.streets(postcode,name,id);
CREATE INDEX streets_ward_name_idx ON locations.streets(ward_id,name,id);

-- Separate FK columns for each level keep search references enforceable.
CREATE TABLE locations.search_documents (
 type text NOT NULL CHECK (type IN ('region','district','ward','street')),
 entity_id integer NOT NULL,
 region_id integer REFERENCES locations.regions(id),
 district_id integer REFERENCES locations.districts(id),
 ward_id integer REFERENCES locations.wards(id),
 street_id integer REFERENCES locations.streets(id),
 name text COLLATE "C" NOT NULL,
 postcode text,
 full_path text NOT NULL,
 document tsvector GENERATED ALWAYS AS
  (to_tsvector('simple', name || ' ' || coalesce(postcode,'') || ' ' || full_path)) STORED,
 PRIMARY KEY (type,entity_id),
 CHECK (
  (type='region' AND region_id=entity_id AND district_id IS NULL AND ward_id IS NULL AND street_id IS NULL AND region_id IS NOT NULL) OR
  (type='district' AND district_id=entity_id AND region_id IS NULL AND ward_id IS NULL AND street_id IS NULL AND district_id IS NOT NULL) OR
  (type='ward' AND ward_id=entity_id AND region_id IS NULL AND district_id IS NULL AND street_id IS NULL AND ward_id IS NOT NULL) OR
  (type='street' AND street_id=entity_id AND region_id IS NULL AND district_id IS NULL AND ward_id IS NULL AND street_id IS NOT NULL)
 )
);
CREATE INDEX search_document_gin ON locations.search_documents USING gin(document);
CREATE INDEX search_type_idx ON locations.search_documents(type);
CREATE TABLE locations.dataset_version (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 fingerprint text NOT NULL,
 source_version text NOT NULL,
 counts jsonb NOT NULL,
 imported_at timestamptz NOT NULL DEFAULT now()
);
