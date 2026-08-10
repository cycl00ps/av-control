#!/usr/bin/env python3
"""Minimal eISCP helper for the local Integra DRX-3.4.

Usage:
  ./eiscp_send.py status
  ./eiscp_send.py apply-defaults
  ./eiscp_send.py raw MVLQSTN
  ./eiscp_send.py raw CTL+10
"""

from __future__ import annotations

import argparse
import re
import socket
import struct
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent
ENV_PATH = ROOT / ".env"

DEFAULTS = {
    "address": "192.168.210.114",
    "port": "60128",
    "listening_mode": "40",
    "master_volume": "50",
    "center_level": "+10",
    "late_night": "01",
}


def load_env(path: Path = ENV_PATH) -> dict[str, str]:
    cfg = dict(DEFAULTS)
    if not path.exists():
        return cfg
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, val = line.split("=", 1)
        cfg[key.strip()] = val.strip().strip('"').strip("'")
    return cfg


def packet(body: str) -> bytes:
    data = body.encode("ascii")
    return b"ISCP" + struct.pack(">II", 16, len(data)) + bytes([1, 0, 0, 0]) + data


def parse_messages(buf: bytes) -> list[tuple[str, str]]:
    return [
        (m.group(1).decode(), m.group(2).decode("utf-8", "replace"))
        for m in re.finditer(rb"!1([A-Z0-9]{3})([^\x1a\r\n]*)", buf)
    ]


def exchange(host: str, port: int, cmd: str, wait: float = 0.9) -> list[tuple[str, str]]:
    """Send ISCP command body without !1 / terminator (e.g. 'MVLQSTN')."""
    body = f"!1{cmd}\r"
    with socket.create_connection((host, port), 2) as sock:
        sock.sendall(packet(body))
        deadline = time.time() + wait
        buf = b""
        sock.settimeout(0.15)
        while time.time() < deadline:
            try:
                chunk = sock.recv(65536)
                if not chunk:
                    break
                buf += chunk
            except socket.timeout:
                if buf:
                    break
                continue
    return parse_messages(buf)


def last(msgs: list[tuple[str, str]], code: str) -> str | None:
    hits = [p for c, p in msgs if c == code]
    return hits[-1] if hits else None


def cmd_status(cfg: dict[str, str]) -> int:
    host, port = cfg["address"], int(cfg["port"])
    with socket.create_connection((host, port), 2) as sock:
        # reuse one connection
        def ex(cmd: str, wait: float = 0.8):
            body = f"!1{cmd}\r"
            sock.sendall(packet(body))
            deadline = time.time() + wait
            buf = b""
            sock.settimeout(0.15)
            while time.time() < deadline:
                try:
                    chunk = sock.recv(65536)
                    if not chunk:
                        break
                    buf += chunk
                except socket.timeout:
                    if buf:
                        break
                    continue
            time.sleep(0.05)
            return parse_messages(buf)

        rows = []
        for q, label in [
            ("SLIQSTN", "input"),
            ("MVLQSTN", "volume_iscp"),
            ("AMTQSTN", "mute"),
            ("LMDQSTN", "listening_mode"),
            ("CTLQSTN", "center_level"),
            ("LTNQSTN", "late_night"),
            ("IFAQSTN", "audio"),
            ("IFVQSTN", "video"),
        ]:
            msgs = ex(q)
            rows.append((label, last(msgs, q[:3])))

    for label, value in rows:
        if label == "volume_iscp" and value:
            try:
                abs_v = int(value, 16)
                display = abs_v / 2
                print(f"{label:16} {value}  (abs {abs_v}, display ~{display:g})")
                continue
            except ValueError:
                pass
        print(f"{label:16} {value}")
    return 0


def cmd_raw(cfg: dict[str, str], raw: str) -> int:
    host, port = cfg["address"], int(cfg["port"])
    raw = raw.strip().lstrip("!1")
    if raw.endswith("\r") or raw.endswith("\n"):
        raw = raw.rstrip("\r\n")
    msgs = exchange(host, port, raw)
    if not msgs:
        print("(no response)")
        return 1
    for code, param in msgs:
        print(f"{code}{param}")
    return 0


def cmd_apply_defaults(cfg: dict[str, str]) -> int:
    host, port = cfg["address"], int(cfg["port"])
    sequence = [
        ("LMD" + cfg["listening_mode"], "listening_mode"),
        ("MVL" + cfg["master_volume"], "master_volume"),
        ("CTL" + cfg["center_level"], "center_level"),
        ("LTN" + cfg["late_night"], "late_night"),
    ]
    with socket.create_connection((host, port), 2) as sock:
        for body, name in sequence:
            sock.sendall(packet(f"!1{body}\r"))
            time.sleep(0.08)
            deadline = time.time() + 0.7
            buf = b""
            sock.settimeout(0.15)
            while time.time() < deadline:
                try:
                    chunk = sock.recv(65536)
                    if not chunk:
                        break
                    buf += chunk
                except socket.timeout:
                    if buf:
                        break
                    continue
            msgs = parse_messages(buf)
            code = body[:3]
            print(f"{name:16} sent {body:8} -> {code}{last(msgs, code) or '(no ack)'}")
            time.sleep(0.05)
    print("done")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Integra DRX-3.4 eISCP helper")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("status", help="Query input / volume / audio / video")
    p_raw = sub.add_parser("raw", help="Send raw ISCP command (e.g. MVLQSTN)")
    p_raw.add_argument("command", help="Command body without !1 prefix")
    sub.add_parser("apply-defaults", help="Apply listening/volume/centre/late-night from .env")

    args = parser.parse_args(argv)
    cfg = load_env()

    if args.cmd == "status":
        return cmd_status(cfg)
    if args.cmd == "raw":
        return cmd_raw(cfg, args.command)
    if args.cmd == "apply-defaults":
        return cmd_apply_defaults(cfg)
    parser.error(f"unknown command {args.cmd}")
    return 2


if __name__ == "__main__":
    sys.exit(main())
