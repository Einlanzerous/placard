# CLAUDE.md — Placard

One canonical, publicly fetchable home for every Construct service mark
(IDEA-22). Public asset store (this repo, served via jsDelivr) + a single
static Go binary that renders the designed front page, mirrors the marks,
verifies canonical URLs, and stages uploads. Sibling to the other
construct-server Go services (shared Postgres 16, `construct_net`).

Tracked in Switchyard under the **PCAD** project (epic `PCAD-1`).

## Layout

- Asset dirs live at the repo **root** (`argosy/`, `switchyard/`, …) so the
  jsDelivr path is `…/placard@main/<svc>/<svc>-mark-light.png` — do not move
  them under `assets/`; the flat path IS the contract.
- `services.json` — the manifest: slug, name, audit note, optional `color`
  (BRAND.md rules only), `raster_from_svg` for services whose PNGs derive
  from their SVG.
- `assets.go` — root package embedding the service dirs + manifest.
- `cmd/placard/` — entrypoint + subcommands (`serve`, `gen`, `check`,
  `migrate`, `version`). No args defaults to `serve`.
- `internal/gen/` — the deterministic generator (SVG raster + DEV badges).
- `internal/store/` — pgx pool, embedded migrator, queries (checks + staged
  uploads). `migrations/` — `NNNN_name.{up,down}.sql`, applied on boot.
- `internal/api/` — HTTP surface + embedded front page (`web/`).
- `internal/checker/` — canonical-URL verification.

## Conventions (construct-server house style)

- Go 1.26, `pgx/v5`. No ORM, no external migration tool.
- Config env-only, `PLACARD_`-prefixed, `DATABASE_URL` fallback. No files.
- Logs: stdlib `log` to stdout. Health: `GET /healthz` (unauthenticated,
  `application/json`, `{status, version, sha}`). Port 4009.
- Release-please + GHCR `ghcr.io/einlanzerous/placard`. Conventional commits.
  Publish uses the `workflow_call`-from-release-please shape (SERV-125).

## Invariants — don't break these

- **Every asset URL is publicly fetchable with no session.** The front page,
  the mirror paths, `/api/services` — all of it serves without auth, and
  Placard's hostname carries no gated Access application. Adding auth
  anywhere the launcher fetches breaks the service's entire purpose,
  silently (the launcher shows initials for a 404 and errors nowhere).
- **Generated files are never hand-edited.** `-dev.png` always, and the mark
  PNGs of any `raster_from_svg` service, are written by `placard gen` from
  the committed sources. CI regenerates and diffs; a hand edit fails there.
- **`gen` is deterministic.** Same inputs, same bytes, every run, every
  platform — that is what lets `git diff --exit-code` be the drift check.
  Nothing in it may depend on time, maps' iteration order, or float
  formatting quirks.
- **The service runs without a database.** No `PLACARD_DATABASE_URL` means
  checks and staged uploads are off, marks and the front page still serve.
  The asset store must not depend on infrastructure state.
- **Postgres is a cache of observations** (check results, staged bytes
  awaiting a human commit). The repo is the source of truth for every mark;
  nothing in the DB is needed to rebuild the served assets.
