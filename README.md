# Placard

One canonical, publicly fetchable home for every Construct service mark
(IDEA-22). A public asset store plus a small Go service with a designed front
page — not a design system, not a component library.

Anything that needs to render a service icon — the Cloudflare Access App
Launcher, Purser's Access-application connector (PRSR-29), a dev-instance
launcher tile — has one URL to build, and that URL is stable by construction
rather than merely correct today.

## The contract

Every service gets a directory at the repo root holding, by convention:

| file | role |
|---|---|
| `<service>/<service>-mark-light.png` | for light surfaces |
| `<service>/<service>-mark-dark.png` | for dark surfaces |
| `<service>/<service>-mark.svg` | carried, unverified — launcher SVG rendering is untested |
| `<service>/<service>-mark-{light,dark}-dev.png` | generated dev variants (yellow DEV badge) — never hand-drawn |

The canonical URL for a service icon:

```
https://cdn.jsdelivr.net/gh/Einlanzerous/placard@main/<service>/<service>-mark-light.png
```

(`-dark` per the consuming surface's background.) The placard service mirrors
the same paths at its own hostname; the jsDelivr URL against this public repo
is canonical.

PNG is the baseline: the launcher's one working icon pre-Placard was a PNG,
and whether the launcher renders SVG is untested. A mark that is one image for
both surfaces (lyceum) commits the same PNG under both names — the contract is
that the pair of paths exists.

## The one constraint

**Every asset URL is publicly fetchable with no session.** The launcher
renders `logo_url` as an `<img>` in the *viewer's* browser, so anything behind
an Access gate serves new invitees — the launcher's whole audience — an HTML
login page instead of an image. If Placard sits behind the Zero Gravity edge,
its hostname carries **no gated Access application**: nothing, a bookmark
tile, or a bypass policy — never `self_hosted` + members policy. Defaulting to
locking it down breaks the service's entire purpose, silently.

Silently is the operative word: Cloudflare accepts a `logo_url` pointing at a
404 without complaint and the launcher just falls back to initials, so a
broken URL is indistinguishable from an unset one, with no error anywhere.
That is why this repo exists — and why the placard service periodically
verifies its own canonical URLs and shows the result on the front page.

## Adding a service

1. `mkdir <service>/` and add the marks — either both PNGs, or just
   `<service>-mark.svg` plus `"raster_from_svg": true` in the manifest.
2. Add the service's row to `services.json`.
3. `go run ./cmd/placard gen .` — rasterizes svg-sourced PNGs and writes the
   `-dev.png` siblings.
4. Commit everything. CI regenerates and fails the build if the generated
   files drift from their sources.

## The service

`placard` is a single static Go binary (construct-server house style):
`serve` renders the front page (implemented from the Claude Design project),
mirrors every mark, serves `/api/services` for machine consumers, verifies
canonical URLs into Postgres on a timer, and accepts uploaded marks into a
staging area for a human to commit. `gen` is the generator above; `check` runs
one verification pass; `migrate` applies embedded migrations; `version`
prints build identity. Configuration is env-only, `PLACARD_`-prefixed — see
`internal/config`. Port 4009. Deploy wiring and the manual Cloudflare steps
live in [`docs/deploy.md`](docs/deploy.md).

## License

The marks in this repository are identifying assets of Construct services.
They are published here so that public surfaces can fetch them — they are not
licensed for use outside identifying the services they name. The embedded IBM
Plex Mono font is under the SIL Open Font License (`internal/gen/font/OFL.txt`).
