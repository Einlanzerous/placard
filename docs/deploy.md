# Deploying Placard

Placard runs as `placard` in the construct-server compose stack: image
`ghcr.io/einlanzerous/placard` pinned by `PLACARD_TAG` in `versions.env`,
port 4009, `construct_net`, shared Postgres 16.

## The edge, and why it is ungated

Route: cloudflared → Traefik `internal` entrypoint → `placard` router →
`placard:4009`.

The `placard` router carries **no `cf-access-jwt` middleware** — the second
origin-auth exemption after the Switchyard GitHub webhook, and like that one
it is named in `check-edge-auth.sh`'s EXEMPT allowlist with its reason. The
reason (IDEA-22): the Access launcher renders each app's `logo_url` as an
`<img>` in the *viewer's* browser, with no Access session for placard —
a gated placard serves login HTML instead of images and every icon silently
falls back to initials. Everything placard serves is already public in this
GitHub repo; the one write path (staged uploads) is token-gated inside the
app, not at the edge.

**Never attach a gated (`self_hosted` + members policy) Access application to
placard's hostname.** Nothing, a bookmark tile, or a bypass policy only.

## One-time provisioning

Recorded exactly as performed (2026-08-23) — this is the manual baseline
PRSR-22's `provision-service` exists to automate.

### 1. Database (done 2026-08-23, from the box)

```sh
docker exec postgres psql -U postgres -c \
  "CREATE ROLE placard_user WITH LOGIN PASSWORD '<generated>';"
docker exec postgres psql -U postgres -c "CREATE DATABASE placard OWNER placard_user;"
docker exec postgres psql -U postgres -c "CREATE DATABASE placard_test OWNER placard_user;"
```

`db/init-db.sh` carries the same `ensure_db` lines for a from-scratch
Postgres, so this step exists only because the cluster predates placard.

### 2. Stack wiring

The construct-server PR adds: the `placard` compose service, the postgres
env line, the init-db entries, the ungated Traefik router, the
`check-edge-auth.sh` EXEMPT entry, `PLACARD_TAG` in `versions.env`, the
promote.yml dropdown entry, and the `.env.example` documentation. Merging it
deploys (deploy.yml watches those paths).

### 3. Secrets (done 2026-08-23, via signet)

Secrets are vaulted in **signet**, which renders them into the
`PROD_ENV_FILE` environment secret the deploy reads — never edit that secret
by hand. Exactly as performed:

```sh
# the password the live placard_user role already has, from step 1
signet set --project construct-server --name PLACARD_DB_PASSWORD    # value on stdin
signet generate --project construct-server --name PLACARD_UPLOAD_TOKEN
signet target add-key --project construct-server --gh-secret PROD_ENV_FILE \
    --name PLACARD_DB_PASSWORD,PLACARD_UPLOAD_TOKEN
signet target add-key --project construct-server --path /home/magos/construct-server/.env \
    --name PLACARD_DB_PASSWORD,PLACARD_UPLOAD_TOKEN
signet target add-key --project construct-server --path /opt/construct-server/.env \
    --name PLACARD_DB_PASSWORD,PLACARD_UPLOAD_TOKEN
signet sync        # seals + pushes the PROD_ENV_FILE render
```

The deploy-root `/opt/construct-server/.env` picks the keys up on the next
deploy (`render-env.sh` merges `PROD_ENV_FILE` + `versions.env`); do **not**
`signet render` it to shortcut that — the vault's copies of the `*_TAG` pins
lag versions.env-driven promotes, and a render would roll live tags back.

`PLACARD_UPLOAD_TOKEN` may be left unset — uploads are then disabled and
everything else works.

### 4. Cloudflare dashboard (manual, ~2 minutes)

1. Zero Trust → Networks → Tunnels → the prod tunnel → **Public Hostname →
   Add**: hostname `placard.zerogravity.industries`, service
   `http://traefik:9080`. (This flow creates the DNS record itself.)
2. **No gated Access application for this hostname** — see above. Optional
   launcher tile: Access → Applications → Add → **Bookmark**, domain
   `https://placard.zerogravity.industries`, App Launcher visible, logo URL

   ```
   https://cdn.jsdelivr.net/gh/Einlanzerous/placard@main/placard/placard-mark-light.png
   ```

   `curl -I` it first and expect `200` + `image/png` — Cloudflare stores a
   404 without complaint, which is the failure this whole service exists to
   end.

### 5. Verify

```sh
./scripts/check-edge-auth.sh          # placard must be listed as exempt BY NAME, everything else gated
curl -s https://placard.zerogravity.industries/healthz          # {"status":"ok",...} with NO session
curl -sI https://placard.zerogravity.industries/placard/placard-mark-light.png | head -3
                                      # 200, image/png — from a browser with no Access cookie too
```

## Operations

- The serve loop re-verifies every canonical jsDelivr URL on
  `PLACARD_CHECK_INTERVAL` (default 6h); `placard check` is the one-shot
  form and exits non-zero if anything is failing.
- No database ⇒ checks and staged uploads off, marks and page still serve.
- A staged upload is fetched with
  `curl -H "X-Placard-Token: …" https://…/api/staged/<id> -o mark.png`,
  committed into `<service>/` per the README convention, and `placard gen`
  regenerates the derived files.
