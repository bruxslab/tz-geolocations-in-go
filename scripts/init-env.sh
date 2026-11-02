#!/bin/sh
set -eu
if [ -e .env ]; then
  echo '.env already exists; refusing to overwrite it' >&2
  exit 1
fi
umask 077
admin=$(openssl rand -hex 24)
owner=$(openssl rand -hex 24)
reader=$(openssl rand -hex 24)
set -C
cat > .env <<EOF
POSTGRES_PASSWORD=$admin
TZ_OWNER_PASSWORD=$owner
TZ_RUNTIME_PASSWORD=$reader
POSTGRES_PORT=55432
HTTP_HOST=127.0.0.1
HTTP_PORT=8080
DATABASE_URL=postgres://tz_reader:$reader@127.0.0.1:55432/tz_locations?sslmode=disable
MIGRATION_DATABASE_URL=postgres://tz_owner:$owner@127.0.0.1:55432/tz_locations?sslmode=disable
TEST_DATABASE_URL=postgres://tz_owner:$owner@127.0.0.1:55432/tz_locations_test?sslmode=disable
RUNTIME_DATABASE_ROLE=tz_reader
DATABASE_MAX_CONNS=10
DATABASE_MIN_CONNS=2
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW_SECONDS=60
CORS_ORIGINS=*
TRUSTED_PROXIES=
EOF
echo 'Created private .env with separate random database credentials.'
