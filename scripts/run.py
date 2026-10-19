#!/usr/bin/env python3
"""Скриншот TUI: omabitrix в pty (размер до exec) + эмуляция терминала.

usage: run.py [--chat ID] [--task ID] [--keys "j,enter"] [--wait 6] [--w 120] [--h 36] [--bin ./bin/omabitrix]
"""
import argparse
import fcntl
import os
import pty
import select
import struct
import sys
import termios
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from term import Term  # noqa: E402

ap = argparse.ArgumentParser()
ap.add_argument("--chat")
ap.add_argument("--task")
ap.add_argument("--keys", default="")
ap.add_argument("--wait", type=float, default=8.0)
ap.add_argument("--w", type=int, default=120)
ap.add_argument("--h", type=int, default=36)
ap.add_argument("--bin", default="./bin/omabitrix")
a = ap.parse_args()

cmd = [os.path.abspath(a.bin)]
if a.chat:
    cmd += ["--chat", a.chat]
if a.task:
    cmd += ["--task", a.task]

KEYMAP = {"enter": "\r", "esc": "\x1b", "up": "\x1b[A", "down": "\x1b[B",
          "pgdn": "\x1b[6~", "pgup": "\x1b[5~", "ctrlc": "\x03"}

pid, fd = pty.fork()
if pid == 0:
    os.environ["TERM"] = "xterm-256color"
    winsz = struct.pack("HHHH", a.h, a.w, 0, 0)
    try:
        fcntl.ioctl(0, termios.TIOCSWINSZ, winsz)  # размер до exec — иначе race
    except OSError:
        pass
    os.execv(cmd[0], cmd)

pending = []
term = Term(a.w, a.h)


def pump(dur):
    end = time.time() + dur
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.05)
        if r:
            try:
                chunk = os.read(fd, 65536)
            except OSError:
                return False
            if not chunk:
                return False
            pending.append(chunk)
    return True


pump(3.5)
keys = [k for k in a.keys.split(",") if k] if a.keys else []
for k in keys:
    try:
        os.write(fd, KEYMAP.get(k, k).encode())
    except OSError:
        break
    pump(1.2 if k in ("enter",) else 0.4)
pump(max(0.3, a.wait - 3.5 - (1.2 if "enter" in keys else 0.4 * len(keys))))
try:
    os.write(fd, b"\x03q")
    pump(0.4)
except OSError:
    pass
try:
    os.kill(pid, 15)
except ProcessLookupError:
    pass
term.feed(b"".join(pending).decode("utf-8", "replace"))
open("/tmp/raw.bin", "wb").write(b"".join(pending))
print(term.dump())
