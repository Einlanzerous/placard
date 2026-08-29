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
- **A mark's shape is measured from the committed bytes, never from a fetch**
  (PRSR-44). `internal/shape` reads the dimensions and answers whether a square
  launcher tile will show the mark whole; `catalog.Build` measures every present
  PNG from the embedded tree. That is why it is a `catalog` fact rather than a
  `MarkCheck` field: a check is a dated observation about a URL and needs both a
  network and a database, while the aspect ratio is a property of the *asset* —
  known offline, on a PR, before jsDelivr has heard of the commit. Filing it
  under `check` would have made it null on exactly the deployment that has
  published a mark and never verified it.
  **A report is keyed on the file a human edits, not on the file affected.**
  `File.Source` is what `placard gen` would derive a file from — a per-variant
  SVG, the shared one, or the mark PNG itself — and `Entry.ShapeFindings()`
  groups on it. One badly shaped glyph in a `raster_from_svg` service produces
  four bad PNGs (two marks, two `-dev` siblings), and reporting four findings
  both overstates the problem and names generated files, whose only correct fix
  is elsewhere. A service that commits its PNGs directly still reports twice
  when both are wrong, which is right: those are two files to edit.
  **The carried SVG is deliberately not measured.** For a `raster_from_svg`
  service the PNG's aspect follows the viewBox, so measuring the PNG already
  catches a badly shaped source and names the file a human edits; a second
  measurement path answering the same question would also break the README's
  "carried, unverified" contract.
- **Runtime reports the shape; CI asserts it.** `placard check` prints a `note`
  line and does **not** count it in its exit code, `/api/services` serves the
  measurement, and the front page shows it — a badly proportioned mark still
  serves, every consumer that fits rather than fills renders it correctly, and a
  letterboxed icon beats two grey initials, so refusing to publish over
  proportions would be worse than the problem. The one place that *fails* is
  `TestEveryEmbeddedMarkSurvivesASquareTile`, whose subject is this repo's own
  committed marks — the same standard `Build` already applies by erroring on a
  `raster_from_svg` service with no generated PNGs, and CI by failing on
  generated-file drift. A PR is the last moment a bad mark is cheap to fix. If a
  service ever needs a deliberately non-square mark, that assertion is the thing
  to revisit, not the band.
  The band lives in `internal/shape` with its reasoning beside it, and matches
  Purser's (PRSR-43) on purpose: the launcher is the only consumer that *crops*,
  so a mark that survives the strictest one survives the rest.
