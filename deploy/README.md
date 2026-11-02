# First GitHub deployment: tz-geolocations-in-go

Prepared for github.com/bruxslab/tz-geolocations-in-go, branch main, owner unctom,
origin 207.180.225.174, directory /srv/apps/unctom/tz-geolocations. Redis is not
used by this release. These files configure deployment; their presence does not mean the server has
been commissioned. No server access/Ansible/container execution is performed by Codex.

## What runs

Existing CI verifies Go/SQL/contracts/OpenAPI and builds the Dockerfile. After a
successful main PUSH run (never a PR run), deploy.yml checks out that exact SHA,
builds/publishes ghcr.io/bruxslab/tz-geolocations-in-go:sha-SHA and records its digest.
Server deployment requires repository variable DEPLOY_ENABLED=true and GitHub
Environment api-test secrets. Keep it unset until commissioning below is done.
Protect main and workflow changes. Use api-test environment required reviewers
if available under your plan, and restrict deployment branches to main.
No self-hosted runner or server administrator key is used.

The separate deployment Compose file keeps the repository's development Compose
unchanged. It starts PostgreSQL 18 with a persistent project-specific volume and
no host port; API publishes 127.0.0.1:18080 -> 8080. CPU limits total at most 0.20
cores with one tool running, memory 1024 MiB; review other unctom workloads against
its 1536 MiB/0.25 CPU user ceiling. PostgreSQL tag is the repository's existing
18-bookworm; initialization does not recreate an existing database container.
Database image upgrades require separate backup/review, not routine CI.

## Commission once

1. Merge the reviewed deployment pull request into the API's main branch.
   Keep DEPLOY_ENABLED unset. Wait for CI then Publish and deploy API to pass its
   publish job; deploy is skipped. Record the exact SHA and image digest from the
   job summary. Set GHCR package visibility to public if public distribution is
   intended. Otherwise, log Docker into GHCR once AS UNCTOM using a dedicated
   read:packages credential (docker login --password-stdin); publishing in Actions
   uses its GITHUB_TOKEN and does not provide server pull credentials.
2. As administrator review ss -lntp for port 18080, rootless Docker availability,
   unctom's other workloads, and disk/RAM. Register this application/port under
   Stage 9 before activating managed public routing. Domains are still undecided;
   registry bindings cannot later be silently changed. Do not claim this bundle
   already updated applications.json or configured Nginx. Before registration,
   use the added image-native /healthcheck executable (CMD, /healthcheck) in the
   registry record. The Dockerfile now builds it for the distroless runtime; no
   shell/curl/wget is assumed inside that image.
3. Log in as unctom using its normal personal key. Confirm /srv/apps/unctom is
   owned by unctom and mode 0700. Create the dedicated directory:

   ```bash
   umask 077
   mkdir -p /srv/apps/unctom/tz-geolocations/releases
   chmod 0700 /srv/apps/unctom/tz-geolocations
   cd /srv/apps/unctom/tz-geolocations
   test ! -e .env && install -m 0600 /dev/null .env
   ```

   Edit .env ON THE SERVER (do not paste credentials in chat):

   ```dotenv
   POSTGRES_PASSWORD=<unique-random-64-hex-characters>
   TZ_OWNER_PASSWORD=<different-random-64-hex-characters>
   TZ_RUNTIME_PASSWORD=<different-random-64-hex-characters>
   CORS_ORIGINS=*
   ```

   Generate each value separately with openssl rand -hex 32. Hex avoids special
   character URL encoding in connection strings. Review CORS for your client.
   This .env is excluded from Git and releases. Never source it as shell code.
   Changing these values later does NOT rotate passwords in an initialized DB.
4. Obtain the reviewed source as unctom, without building on the server:

   ```bash
   git clone https://github.com/bruxslab/tz-geolocations-in-go.git source
   ```

   Set REVISION and IMAGE below to the recorded, exact values (not main/latest):

   ```bash
   REVISION=<40-character-successful-main-commit-sha>
   IMAGE=ghcr.io/bruxslab/tz-geolocations-in-go@sha256:<64-hex-digest>
   git -C source checkout --detach "$REVISION"
   mkdir -p "releases/$REVISION"
   git -C source archive "$REVISION" deploy migrations | tar -xf - -C "releases/$REVISION"
   bash "releases/$REVISION/deploy/server.sh" bootstrap "$IMAGE" "$REVISION"
   bash "releases/$REVISION/deploy/server.sh" deploy "$IMAGE" "$REVISION"
   ```

   Bootstrap explicitly initializes only this new application's database, applies
   migrations and imports the embedded dataset WITHOUT --replace. Do not attach an
   unrelated existing volume. It fails if already commissioned. Existing data is
   not erased on retry; investigate interrupted initialization/credential changes.
   A successful bootstrap alone does not prove API deployment. Expect the deploy
   command to report a successful nonempty database-backed regions query.
5. Generate a SEPARATE passphrase-free deployment key on your controller:

   ```bash
   ssh-keygen -t ed25519 -f ~/.ssh/tz-geolocations-deploy -C github-tz-geolocations
   ```

   Append its .pub line to unctom's inventory ssh_keys (retain personal keys) and
   apply users.yml with your administrator key. Never use brxs-master in Actions.
   This key grants unctom's account access, including its other applications; it
   is not an application-only privilege boundary. Use a separate service identity
   in a future revision if independent per-application credentials are required.
6. Create GitHub Environment api-test and its secrets:
   - DEPLOY_SSH_KEY: complete dedicated private key from step 5.
   - DEPLOY_KNOWN_HOSTS: verified full ED25519 known_hosts LINE for 207.180.225.174,
     not merely a SHA256 fingerprint. On server read
     /etc/ssh/ssh_host_ed25519_key.pub; through your trusted administrator channel
     construct `207.180.225.174 ssh-ed25519 AAAA...` and independently compare the
     fingerprint. The workflow uses strict verification, never runtime keyscan.
   SSH port remains 22. Hosted runners need reachability to that origin/port;
   keep existing firewall policy and review source restrictions separately.
7. Set repository Actions variable DEPLOY_ENABLED to true. Push a normal reviewed
   main commit or re-run CI on that main SHA. Confirm the publishing digest and
   deploy health result. A skipped deploy while disabled is expected, not success.

## Test without a public hostname

From controller, keep this SSH tunnel open:

```bash
ssh -N -o ExitOnForwardFailure=yes -i ~/.ssh/YOUR_UNCTOM_PRIVATE_KEY \
  -L 127.0.0.1:18080:127.0.0.1:18080 unctom@207.180.225.174
```

Then request http://127.0.0.1:18080/v1/regions or /v1/docs locally. Choose a different
LOCAL port if occupied. The server port must remain private. The IP alone is not
an HTTPS domain choice. This bundle does not create http://207.180.225.174:18080
public access or change legacy Nginx IP/default sites. Public HTTPS routing is the
next step after selecting a DNS name/certificate and registering the application.

## Deployments, migration changes and recovery

A server flock and workflow concurrency serialize deployments. API is recreated
in place: brief interruption is possible; this is not blue/green or zero downtime.
Routine deploy pulls only the exact API digest, refuses changed migration files,
and queries /v1/regions until a nonempty JSON data list succeeds. PostgreSQL is
not stopped/recreated, no routine migration/import is run, and no volumes/images
are pruned. This endpoint is a useful smoke test, not complete API acceptance.

For reviewed new migrations, take/verify a recoverable PostgreSQL backup first,
review compatibility with the prior API and deploy budget; then as unctom run the
candidate release's migrate service with API_IMAGE set to its exact digest and
--env-file pointing to the application's .env. Only after migration succeeds,
write that candidate SHA to schema-release. Do not simply advance the marker to
bypass migration failure. Dataset replacement is separate and never automatic.

When API activation fails, server.sh attempts to restore the recorded previous
image AND previous Compose release and returns failure. First-run failure stops
the API but retains PostgreSQL and its volume. Database/schema changes are never
rolled back automatically; image rollback requires schema compatibility. Retain
release directories and digests for manual rollback using server.sh deploy with
the previously recorded image/SHA. Inspect rootless Compose ps/logs privately if
recovery fails; avoid sharing environment-expanded config or secrets in logs.

Backups remain disabled in the infrastructure inventory: do not use this first
trial deployment as proof of durable data protection. PostgreSQL native exports,
volume recovery and external repository/restore testing remain to commission.

References: https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images
and https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows

## Internal health passes but host readiness fails

The API joins both the frontend bridge and the internal backend. PostgreSQL and
one-off tools join only backend. An API attached exclusively to an internal
bridge can have working in-container healthchecks but no working published host
port. The separate frontend enables the existing 127.0.0.1:18080 publication; it
does not expose the database or bind the API to the public IP.

For the corrected Compose file, extract the newly reviewed release under its
commit SHA, keep the initialized database and its existing volume, and rerun
server.sh deploy with that release SHA and the already published API digest.
No bootstrap, reimport, database restart or database-volume removal is needed.
A successful in-container /healthcheck does not prove host publication; the
deployment's host-side regions query must also pass.
