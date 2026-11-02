#!/usr/bin/env bash
# Run only as unctom. No sudo, builds, git pulls, migrations or imports on routine deploy.
set -Eeuo pipefail
umask 077
base=/srv/apps/unctom/tz-geolocations
mode=${1:-}
image=${2:-}
release=${3:-}
[[ $(id -un) == unctom ]] || { echo 'Run as unctom' >&2; exit 1; }
[[ $mode == deploy || $mode == bootstrap ]] || exit 2
[[ $image =~ ^ghcr\.io/bruxslab/tz-geolocations-in-go@sha256:[a-f0-9]{64}$ ]] || exit 2
[[ $release =~ ^[a-f0-9]{40}$ ]] || exit 2
cd "$base"
[[ -f .env && ! -L .env && $(stat -c '%a:%u' .env) == "600:$(id -u)" ]] || { echo 'Expected owner-only .env (0600)' >&2; exit 1; }
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
unset DOCKER_CONTEXT
[[ -S "$XDG_RUNTIME_DIR/docker.sock" ]] || { echo 'Rootless socket missing' >&2; exit 1; }
exec 9> .deployment.lock
flock -w 900 9 || exit 1
export API_IMAGE="$image"
# PostgreSQL reads this nonsecret bind-mounted script after dropping privileges.
chmod 0644 "$base/releases/$release/deploy/init-db.sh"
compose() { docker compose --project-name unctom_tz_geolocations --env-file "$base/.env" -f "$base/releases/$release/deploy/compose.yaml" "$@"; }
compose config --quiet
if [[ $mode == bootstrap ]]; then
  [[ ! -e .database-ready ]] || { echo 'Database already commissioned' >&2; exit 1; }
  compose pull api postgres
  compose up -d --no-recreate --wait --wait-timeout 180 postgres
  compose run --rm --no-deps migrate
  compose run --rm --no-deps import
  printf '%s\n' "$release" > schema-release
  touch .database-ready
  echo 'Database initialized. Now run deploy with the same image and release.'
  exit 0
fi
[[ -f .database-ready ]] || { echo 'Run documented first-time database bootstrap first' >&2; exit 1; }
compose pull api
# Migration changes require the separate operator procedure, not an automatic hook.
previous=''
[[ ! -f current-image ]] || previous=$(cat current-image)
previous_release=''
[[ ! -f current-release ]] || previous_release=$(cat current-release)
schema_release=''
[[ ! -f schema-release ]] || schema_release=$(cat schema-release)
if [[ $schema_release != "$release" ]]; then
  [[ $schema_release =~ ^[a-f0-9]{40}$ ]] || { echo 'Missing schema approval' >&2; exit 1; }
  diff -qr "$base/releases/$schema_release/migrations" "$base/releases/$release/migrations" >/dev/null || { echo 'Migration changes require operator review and schema approval; API unchanged.' >&2; exit 1; }
fi
probe() {
  curl --noproxy '*' --fail --silent --show-error --max-time 10 http://127.0.0.1:18080/v1/regions |
    python3 -c 'import json,sys; d=json.load(sys.stdin); assert isinstance(d.get("data"),list) and len(d["data"])>0'
}
restore() {
  trap - ERR
  if [[ $previous =~ ^ghcr\.io/bruxslab/tz-geolocations-in-go@sha256:[a-f0-9]{64}$ && $previous_release =~ ^[a-f0-9]{40}$ ]]; then
    export API_IMAGE="$previous"
    release=$previous_release
    if compose up -d --no-deps api && probe; then
      echo 'Previous API image restored; deployment failed.' >&2
    else
      echo 'CRITICAL: API rollback failed; inspect the project manually.' >&2
    fi
  else
    compose stop api || true
    echo 'First deployment failed; API stopped, database volume retained.' >&2
  fi
  exit 1
}
trap restore ERR
compose up -d --no-deps api
healthy=false
for attempt in {1..30}; do
  if probe >/dev/null 2>&1; then healthy=true; break; fi
  sleep 2
done
[[ $healthy == true ]]
printf '%s\n' "$image" > current-image.tmp
printf '%s\n' "$release" > current-release.tmp
mv current-image.tmp current-image
mv current-release.tmp current-release
trap - ERR
echo 'API deployed; database-backed regions query passed.'
