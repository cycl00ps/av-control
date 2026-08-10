# eISCP / ISCP protocol (DRX-3.4)

## Transport

| | |
|--|--|
| Protocol | TCP |
| Default port | **60128** |
| Auth | None |
| Framing | 16-byte eISCP header + ISCP ASCII body |

### eISCP header (big-endian)

| Offset | Size | Value |
|--------|------|--------|
| 0 | 4 | ASCII `ISCP` |
| 4 | 4 | Header size `0x00000010` (16) |
| 8 | 4 | Data size (length of ISCP body) |
| 12 | 1 | Version `0x01` |
| 13–15 | 3 | Reserved `0x000000` |

### ISCP body

```text
!  1  CCC  PPPPP  <terminators>
│  │  │    │
│  │  │    └─ parameter (variable; use QSTN to query)
│  │  └────── 3-character command mnemonic
│  └───────── unit type (1 = receiver)
└──────────── start character
```

Controller → device terminators (Ethernet): usually `\r` in the data size, device replies with body ending in `EOF` (`0x1A`) often followed by `\r\n`.

Example power query body: `!1PWRQSTN\r`

### Timing / connection

- ≥ 50 ms between messages
- Device should answer within ~50 ms for a successful transaction
- Hold one persistent TCP connection to receive unsolicited status (power, volume, input, …)
- Only one client is reliable at a time

---

## Message kinds

1. **Command** — set something (`PWR01`, `MVL50`, `SLI11`)
2. **Question** — append `QSTN` (`PWRQSTN`, `IFAQSTN`)
3. **Event / notice** — unsolicited status with the same shape as answers

---

## Command groups useful on this unit

Not every ISCP mnemonic applies to every model/firmware. Prefer probing with `QSTN` first. Commands marked **verified** were exercised live against this DRX-3.4.

### System / amp (verified subset)

| Cmd | Role | Notes |
|-----|------|--------|
| `PWR` | Main power | `00` standby, `01` on — **verified** |
| `AMT` | Mute | `00`/`01`/`TG` — **verified** |
| `MVL` | Master volume | Hex absolute; see README volume table — **verified** |
| `SLI` | Input selector | See README input table — **verified** |
| `LMD` | Listening mode | `40` Straight Decode, `82` Neural-style — **verified** |
| `CTL` | Centre temporary level | `+10`, `000`, `UP`, `DOWN` — **verified** |
| `LTN` | Late Night | `00` off, `01` low — **verified** (`02` N/A here) |
| `IFA` | Audio information | Query only — **verified** |
| `IFV` | Video information | Query only — **verified** |
| `HDO` | HDMI output | `01` = Main — **verified** |
| `SLA` | Audio selector | **verified** (value `04` observed) |
| `DIM` | Dimmer | **verified** |
| `SLP` | Sleep | `OFF` observed — **verified** |
| `DIF` | Display info mode | **verified** |
| `CEC` | CEC | `01` on observed — **verified** |
| `NRI` | Device XML info | Large payload — **verified** |
| `SWL` | Subwoofer temporary level | **verified** |
| `TFR` | Front tone | `B00T00` flat — **verified** |
| `MOT` | Music Optimizer | **verified** |
| `RAS` | Cinema filter / Re-EQ style | **verified** |
| `DLC` | Vocal/Dialog | Present in `NRI` but **N/A** for DTS-HD Straight Decode path |
| `DCE` | Dialog Control enable | No useful response on this path |

### Zones

| Cmd | Zone |
|-----|------|
| `ZPW` `ZMT` `ZVL` `SLZ` | Zone 2 — queries **verified** |
| `PW3` `MT3` `VL3` `SL3` | Zone 3 — queries **verified** |
| `PW4` / … | Zone 4 — **not present** (no reply) |

### Network / USB (when on NET/USB)

| Cmd | Role |
|-----|------|
| `NTC` | Transport: `PLAY` `PAUSE` `STOP` `TRUP` `TRDN` … |
| `NAT` `NAL` `NTI` `NTM` `NTR` | Metadata / time / track |
| `NST` | Network status |

### Other families (protocol-wide; model-dependent)

Tuner (`TUN` `PRS`), HDMI extras (`RES` `VOS`), Audyssey (`ADY` `ADQ` `ADV`), OSD menu keys (`OSD MENU|UP|DOWN|LEFT|RIGHT|ENTER|EXIT`), 12V triggers (`TGA` `TGB` `TGC`), speaker calibration (`SLC` — **avoid while watching**; plays test tones), memory (`MEM STR|RCL` — not a durable store for `CTL`).

---

## Listening mode codes (selected)

| Code | Typical meaning | Use on 3.1? |
|------|-----------------|-------------|
| `00` | Stereo | Music / 2.0 |
| `01` | Direct | Minimal processing |
| `11` | Pure Audio | |
| `40` | **Straight Decode** | **Yes — preferred for 5.1 movies** |
| `0C` | All Channel Stereo | Not for movies |
| `82` | Neo:6 Cinema / Neural-family (device-dependent label) | Poor for dialogue here |

Exact labels for a code can differ by firmware; trust `IFAQSTN` for the human-readable mode name currently active.

---

## Centre / dialogue controls

| Mechanism | Command | Persists? |
|-----------|---------|-----------|
| Temporary centre trim | `CTL±nn` | Session / until reset — **use for quick boost** |
| Late Night | `LTN01` | May reset — helps speech vs peaks |
| Vocal/Dialog | `DLC0`–`DLC5` | N/A on current DTS-HD path |
| Permanent speaker level | Setup UI / Dirac | Yes — calibrate when idle |

`CTL` range on this unit’s `NRI`: Centre Level **−12 … +12** dB.

---

## Minimal Python send/receive

```python
import socket, struct

def eiscp_packet(body: str) -> bytes:
    data = body.encode("ascii")
    return b"ISCP" + struct.pack(">II", 16, len(data)) + bytes([1, 0, 0, 0]) + data

def send(host, cmd, port=60128):
    # cmd like "MVLQSTN" or "CTL+10" — helper adds !1 and \\r
    body = f"!1{cmd}\r"
    with socket.create_connection((host, port), 2) as s:
        s.sendall(eiscp_packet(body))
        s.settimeout(1.0)
        return s.recv(65536)
```

Or use [`eiscp_send.py`](eiscp_send.py) in this folder.

---

## References

- Integra/Onkyo ISCP documentation (various Excel/PDF revisions; community mirrors)
- [miracle2k/onkyo-eiscp](https://github.com/miracle2k/onkyo-eiscp) command YAML
- AI4AV Integra DRX series specs (ISCP over TCP summary)
