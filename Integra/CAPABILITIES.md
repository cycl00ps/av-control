# Integra DRX-3.4 — control surface capabilities

Inventory of what this unit exposes on the LAN, for the Go + HTMX remote ([`PLAN.md`](PLAN.md)).

**Sources**

| Source | Role |
|--------|------|
| [ISCP_AVR_146.xlsx](https://github.com/miracle2k/onkyo-eiscp/files/11500984/ISCP_AVR_146.xlsx) | Family command catalogue (column **DRX-3.4**) |
| Live `NRIQSTN` | Device XML: zones, inputs, net services, control flags |
| Live `*QSTN` sweep | What this firmware actually answers (2026-08-10) |
| Live Dirac set check | `DSSC00` vs `DSS00` while already Off (no Room EQ change) |

Stack note: day-to-day remote = **eISCP TCP 60128** via a single Go proxy; browsers use HTMX + SSE. See [`PLAN.md`](PLAN.md).

---

## Network endpoints

| Endpoint | Use for our remote |
|----------|-------------------|
| **TCP 60128 eISCP** | **Primary** — control + status |
| **UDP 60128 `ECN`** | Discovery — `!xECNQSTN` → `ECNDRX-3.4/60128/XX/<MAC>` (verified) |
| **HTTP :80** Web Setup | Config only (basic auth). `/api/` → 403 |
| **TCP 60229** Dirac | Calibration app channel (`NRI` `Dirac port="60229"`). **Not** day-to-day; use eISCP `DSS`/`DSSC` for slots |
| **TCP 8008 / 8009** | Chromecast built-in (stream sink) |
| **TCP 7000** | AirPlay / AirTunes (stream sink) |

---

## Device identity (`NRI` + `FWV` / `IFN`)

| Field | Value |
|-------|--------|
| Model | Integra **DRX-3.4** |
| Firmware | `R136-0815-1282-0033-0000` (`FWV` wire: `R136081512820033`) |
| MAC / serial | `00:09:B0:F6:56:9C` |
| Network name | `Integra DRX-3.4 F6569C` (`IFN` / `NFN`) |
| Cast build (`NGV`) | `1.68.cast_20240607_1430_RC06.641358537` |

### Zones (`NRI`)

| Zone | Enabled | Notes |
|------|---------|--------|
| Main | Yes | |
| Zone2 | Yes | |
| Zone3 | Yes | |
| Zone4 | **No** (`value="0"`) | `PW4`/`SL4`/… silent live |

### Inputs (`NRI` selectorlist)

| Code | Name | Notes |
|------|------|--------|
| `10` | XBOX | |
| `02` | GAME | |
| `01` | PS5 | |
| `11` | Shield | Often current (`SLI11`) |
| `05` | PC | Main-only zone mask in NRI |
| `03` | AUX | |
| `23` | CD | |
| `12` | TV | |
| `22` | PHONO | |
| `25` | AM | |
| `24` | FM | |
| `2B` | NET | |
| `2E` | BLUETOOTH | |
| `80` | Source | Zone follow |

### NET services (`NRI` netservicelist → `NSV` ids)

| id | Name |
|----|------|
| `0e` | TuneIn Radio |
| `0a` | Spotify |
| `12` | Deezer |
| `44` | AirPlay |
| `1b` | TIDAL |
| `00` | Music Server |
| `f0` | USB |
| `40` | Chromecast built-in |
| `1d` | Play Queue |
| `42` | DTS Play-Fi |
| `45` | Amazon Alexa |

`NSVQSTN` itself is silent; select via `NSV` after `SLI2B` (app/community pattern).

### Notable `NRI` controls (enabled)

Centre Level (−12…+12), Subwoofer Level, Bass/Treble, Vocal/Dialog (0–5), Music Optimizer, Dimmer, Dirac (`port=60229`), LMD Movie/Music/Game/Stereo, HDMI OUT Sub/Zone2, CEC BD/TV, UI Status, etc.

---

## Dirac slot verification (no Room EQ change)

At probe time Dirac was **Off** (`DSSQSTN` → `000`). Only **Off→Off** was sent so listening was undisturbed.

| Step | Command | Result |
|------|---------|--------|
| Query | `DSSQSTN` | `DSS000` |
| Set (community) | `DSSC00` | `DSS000` + FL text **`Dirac   : Off`** |
| Query | `DSSQSTN` | `DSS000` (unchanged) |
| Set (spreadsheet literal) | `DSS00` | **`DSSN/A`** — rejected |

**Conclusion for the app:** use **`DSSC00`–`DSSC03`** to set Off / Slot1 / Slot2 / Slot3; use **`DSSQSTN`** to read. Spreadsheet rows labelled `DSS00`… without the `C` are wrong on this unit (same finding as TX-RZ50 community reports). Slot1–3 set was **not** exercised here (would engage Room EQ).

ISCP_AVR_146 marks `DSS` / `00`–`03` / `QSTN` as **Yes** for DRX-3.4.

---

## Live `*QSTN` results (this unit)

Legend: **OK** = non-`N/A` answer · **N/A** = mnemonic recognized but unavailable · **Silent** = no reply (treat as unsupported here).

### Core remote (Tier 1) — OK

| Cmd | Live value (sample) | Role |
|-----|---------------------|------|
| `PWR` | `01` | Power |
| `MVL` | `5A` | Master volume (hex) |
| `AMT` | `00` | Mute |
| `SLI` | `11` | Input (Shield) |
| `LMD` | `82` | Listening mode (Neural:X at probe) |
| `CTL` | `+10` | Centre trim |
| `LTN` | `01` | Late Night Low |
| `DSS` | `000` | Dirac Off |
| `EQS` | `00` | Equalizer off |
| `SWL` | `000` | Sub temp trim |
| `TFR` | `B00T00` | Front tone flat |

### Status / telemetry — OK

| Cmd | Live value (sample) | Role |
|-----|---------------------|------|
| `IFA` | HDMI 4, DTS-HD, 48 kHz, 5.1→3.1, … | Audio path |
| `IFV` | 4K 59 Hz YCbCr422 pass-through | Video path |
| `FLD` | `Shield    45.0` (hex-decoded) | Front-panel text |
| `TPD` | `F 82C 28` | Temperature |
| `FWV` | `R136081512820033` | Firmware |
| `NRI` | large XML | Capability dump |
| `IFN` / `NFN` | IP / friendly name | Network identity |
| `HDO` / `HAO` / `HOI` / `HAS` | `01` / `02` / `11` / `02` | HDMI |
| `SLA` | `04` | Audio selector |
| `DIM` / `DIF` / `SLP` | `00` / `00` / `OFF` | Display / sleep |
| `CEC` / `ARC` | `01` / `01` | CEC / ARC |
| `MOT` / `RAS` / `AVS` / `DGF` | `00` / `00` / `0000` / `03` | Audio extras |
| `CTS` / `DLC` / `DCE` / `VOC` | `00` | Centre spread / dialog-related |
| `APS` | `01` | Auto power (answered) |
| `NSB` / `NDS` | `ON` / `E-x` | BT / NET status |
| `NST` / `NJA` | `Sxx1` / `BMP` | NET / jacket stubs |
| `AEQ` / `STW` / `SPI` / `SAM` | `00` / `00` / `1110000---` / `FR` | EQ / speakers / etc. |
| `TGA`/`TGB`/`TGC` | `01`/`00`/`01` | 12V triggers |
| `DUS` | `NM--0` | Device UI status |
| `MDI` | XML (device id, net standby, …) | App metadata blob |
| `NGV` | Cast build string | Chromecast version |
| `NGT` | `Asia/Hong_Kong` | Timezone |
| `PPS` | `101` | (answered) |

### Zones / tuner — OK

| Cmd | Live | Notes |
|-----|------|--------|
| `ZPW` `ZVL` `ZMT` `SLZ` | `00` `50` `00` `80` | Zone2 |
| `PW3` `VL3` `MT3` `SL3` | `00` `50` `00` `80` | Zone3 |
| `TUN` `PRS` / `TUZ` `PRZ` / `TU3` `PR3` | freq / `00` | Tuner mirrors |

### Answered `N/A` (present but unavailable now)

`DBR` `DER` `DPT` `ECO` `SW2` `ZBL` `ZTN` — do not enable as primary controls until a path returns real values.

### Silent (sample of important misses)

Zone4 (`PW4`…), Audyssey (`ADY`/`ADQ`/`ADV`), `NSV` query, `HST`/`SPR`/`RES`, `SPA`/`SPB`/`SPL`, `CMT`, `ITV`, `PMB`/`PCT`/`LFE`, `SCD`, AirPlay metadata (`AAT`…), many HD Radio / XM / legacy codes.

Sheet marks `OSD` `MEM` `POP` `UPD` `RST` as DRX-3.4 **Yes** but they have no useful `QSTN` (action-only / dangerous). Keep `UPD`/`RST` out of the UI.

---

## ISCP_AVR_146 — DRX-3.4 “Yes” on MAIN (50 mnemonics)

Commands with at least one parameter marked **Yes** for **DRX-3.4** in the spreadsheet:

`AEQ` `AMT` `APS` `ARC` `AVS` `CEC` `CTL` `DCE` `DGF` `DIF` `DIM` `DLC` `DSS` `DUS` `ECO` `EQS` `FWU` `FWV` `HAO` `HDO` `HOI` `HSF` `IFA` `IFN` `IFV` `LMD` `LTN` `MEM` `MOT` `MVL` `OSD` `POP` `PRS` `PWR` `RAS` `RST` `SAM` `SLA` `SLI` `SPI` `STW` `SWL` `TFR` `TGA` `TGB` `TGC` `TPD` `TUN` `UPD` `VOC`

**Live confirmed answering** among those: most of the list above except action-only `MEM`/`OSD`/`POP`/`UPD`/`RST` (and `ECO` → `N/A`).

Additional live answers **not** required to be in that MAIN Yes column (zones/NET/etc.): e.g. `FLD`, `NRI`, `ZPW`/`PW3` family, `NSB`/`NDS`, `CTS`, Cast `NGV`, …

Family-wide MAIN catalogue size in ISCP_AVR_146: **151** mnemonics (many model-specific). Rule: **sheet Yes ≠ ship in UI** until live probe; **live answer ⇒ candidate**.

---

## App mapping (what to build)

| Product feature | Commands | Evidence |
|-----------------|----------|----------|
| Day-to-day remote | `PWR` `MVL` `AMT` `SLI` `LMD` `CTL` `LTN` `SWL` `TFR` | Live OK |
| Dirac Off / Slot1–3 | `DSSQSTN` + **`DSSC0x`** | Query + Off no-op set verified |
| EQ presets | `EQS` | Live OK |
| Status strip / detail | `IFA` `IFV` `FLD` `TPD` `FWV` `HDO` `HAO` `HOI` `NSB` `NDS` | Live OK |
| Discovery | UDP `ECN` | Verified |
| NET service picker | `SLI2B` + `NSV` + NRI ids | NRI list; query silent |
| Zones 2/3 | `Z*` / `*3` | Live OK; no Zone4 |
| Advanced (gated) | `DLC`/`CTS`/`AVS`/`RAS`/`DGF`/… | Query often OK; sets path-dependent |
| Not our remote | `:60229` calib, Cast/AirPlay clients, Sonos/Alexa ecosystems | Endpoints / product features only |

---

## Preferred defaults (from `.env`)

Unrelated to this probe’s listening-mode snapshot (`LMD82` at probe time):

| Setting | ISCP | Meaning |
|---------|------|---------|
| Listening mode | `LMD40` | Straight Decode |
| Volume | `MVL50` | Display ~40.0 |
| Centre | `CTL+10` | Dialogue trim |
| Late Night | `LTN01` | Low |

Optional later: preferred `DSSC` slot in `.env` after Slot1–3 is confirmed in a quiet session.

---

## Probe notes

- Timing: ≥ 50 ms between messages; one TCP client.
- `NRI` / `MDI` payloads are large — parse once and cache.
- `FLD` is hex-encoded ASCII on the wire.
- Spreadsheet support columns can lag firmware; trust live `QSTN` for enablement.
- This inventory used **queries** plus one **Dirac-Off no-op** (`DSSC00`). No volume/input/mode/Dirac-slot changes.

See also: [`PROTOCOL.md`](PROTOCOL.md), [`README.md`](README.md), [`eiscp_send.py`](eiscp_send.py), [`PLAN.md`](PLAN.md).
