# Integra DRX-3.4 remote

Go + HTMX LAN remote for the Integra AVR over eISCP. UX reference: [`mockup/`](mockup/). Design notes: [`PLAN.md`](PLAN.md), [`CAPABILITIES.md`](CAPABILITIES.md), [`PROTOCOL.md`](PROTOCOL.md).

## Config

Copy [`.env.example`](.env.example) to `.env` and set values. `.env` is gitignored.

| Variable | Meaning |
|----------|---------|
| `INTEGRA_ADDRESS` | Amp IP |
| `INTEGRA_EISCP_PORT` | Default `60128` |
| `INTEGRA_MOCK` | `1` = in-memory fake amp (no network to AVR) |
| `INTEGRA_HTTP_ADDR` | Listen address, e.g. `0.0.0.0:8080` |
| `listening_mode` / `master_volume` / `center_level` / `late_night` | Apply-defaults prefs |

## Run

```bash
cd Integra
cp .env.example .env   # edit as needed
go run ./cmd/integra-web
```

Static mockup only (no Go):

```bash
cd mockup && python3 -m http.server 8090 --bind 0.0.0.0
```

## CLI helper

[`eiscp_send.py`](eiscp_send.py) — ad-hoc eISCP from a host that can reach the amp.
