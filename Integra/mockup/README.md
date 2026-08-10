# Integra remote — static UX mock

Phone-first HTML/CSS/JS mock of the planned web remote. **No amp connection**, no Python app, no HTMX server.

## Open

```bash
# Option A — open the file in a browser
xdg-open index.html   # or double-click

# Option B — optional static HTTP (handy for phone on LAN)
python3 -m http.server 8090 --bind 0.0.0.0
# then http://127.0.0.1:8090/  or http://<lan-ip>:8090/
```

## What works

- Status strip (power, input, volume, mute, mode, Dirac)
- Volume ± / slider; **centre trim** directly under volume
- Mute, Apply defaults; **Standby asks for confirmation** (Power on is immediate)
- Input grid (NRI names)
- Straight Decode / Neural:X, Late Night **Off / Low only** (High/`02` was N/A on this unit; Auto unverified)
- Dirac slots, EQ presets
- **Web tuner** when FM/AM selected: band, seek (±0.05 MHz / ±9 kHz), presets 1–8 — mocks `SLI24/25` + `TUN`/`PRS`
- A/V detail with temperature as **°C** (parsed from a `TPD`-shaped string)
- Later: NET / zones still placeholders

Seeded defaults: Straight Decode, vol 40, centre +10, Late Night Low, Dirac Off, Shield; tuner seed FM 92.10 MHz.

Real HTMX + eISCP comes later — see [`../PLAN.md`](../PLAN.md). Tuner in production: `POST /cmd/...` → Go → eISCP (`TUN`/`PRS`), status via SSE.
