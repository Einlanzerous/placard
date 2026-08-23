# Construct brand rules

The estate-wide rules a service should consult before rendering another
service's name, mark, or accent. Rules live here — one service's stylesheet is
where a convention goes to die quietly (CHRN-70, IDEA-22).

## Reference colours

**Coral `#e2623d` means Switchyard and gold `#d99b2b` means Amber, wherever a
reference to either resolves.**

The rule was discovered in Chronicle — whose own signal is vellum, which is
what freed those two colours to mean something specific — but it is not a
Chronicle rule. A surface that renders a reference to one of these services
uses that service's colour; picking your own kills the convention the same
silent way rotted logo URLs killed the launcher icons.

| service | colour | |
|---|---|---|
| Switchyard | coral `#e2623d` | the mark itself is this coral |
| Amber | gold `#d99b2b` | reserved — Amber has no mark yet |
| Chronicle | vellum `#e0d5be` | Chronicle's own signal |
| Placard | lime `#b8ff2e` | this service's accent |

`services.json` carries `color` only where a rule is established here. Leave
it absent rather than inventing one.

## Dev marks

Dev-instance variants are generated, never hand-drawn. `placard gen` overlays
the yellow badge — `#ffd400`, the word `DEV` in IBM Plex Mono SemiBold
`#141414`, bottom-right corner — onto each `<service>-mark-{light,dark}.png`
and writes the `-dev.png` siblings. CI regenerates and fails on drift, so a
dev variant can never drift from the mark it derives from.

## Placard UI palette

From the Claude Design front-page project, for anything that extends the
placard surface itself:

| token | value |
|---|---|
| background | `#0d0e0f` |
| panel | `#131517` |
| border | `#24282c` |
| text | `#e6e8ea` |
| muted | `#7d848a` |
| accent (lime) | `#b8ff2e` |
| alert (pink) | `#ff3d8b` |
| dev (yellow) | `#ffd400` |

Type: Archivo for UI text, IBM Plex Mono for paths, URLs, and labels.
