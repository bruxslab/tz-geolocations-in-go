#!/bin/sh
set -eu
# psql variables quote passwords as SQL literals, never shell-interpolated SQL.
psql --username "$POSTGRES_USER" --dbname postgres --set ON_ERROR_STOP=1 \
  --set owner_password="$TZ_OWNER_PASSWORD" --set reader_password="$TZ_RUNTIME_PASSWORD" <<'SQL'
CREATE ROLE tz_owner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'owner_password';
CREATE ROLE tz_reader LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'reader_password';
ALTER ROLE tz_reader SET default_transaction_read_only = on;
CREATE DATABASE tz_locations OWNER tz_owner;
CREATE DATABASE tz_locations_test OWNER tz_owner;
REVOKE ALL ON DATABASE tz_locations FROM PUBLIC;
REVOKE ALL ON DATABASE tz_locations_test FROM PUBLIC;
GRANT CONNECT ON DATABASE tz_locations TO tz_owner, tz_reader;
GRANT CONNECT ON DATABASE tz_locations_test TO tz_owner;
SQL
