#!/usr/bin/env python3
"""Скриншот TUI: omabitrix в pty + мини-эмуляция терминала -> последний кадр.

usage: shot.py [--chat ID] [--task ID] [--keys "j,enter"] [--wait 6] [--w 120] [--h 36]
"""
import argparse, fcntl, os, pty, re, select, struct, sys, termios, time

ap = argparse.ArgumentParser()
ap.add_argument("--chat"); ap.add_argument("--task")
ap.add_argument("--keys", default="")
ap.add_argument("--wait", type=float, default=6.0)
ap.add_argument("--w", type=int, default=120)
ap.add_argument("--h", type=int, default=36)
a = ap.parse_args()

cmd = [os.path.abspath("./bin/omabitrix")]
if a.chat: cmd += ["--chat", a.chat]
if a.task: cmd += ["--task", a.task]

pid, fd = pty.fork()
if pid == 0:
    os.environ["TERM"] = "xterm-256color"
    os.execv(cmd[0], cmd)

fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", a.h, a.w, 0, 0))

KEYMAP = {"enter": "\r", "esc": "\x1b", "up": "\x1b[A", "down": "\x1b[B",
          "pgdn": "\x1b[6~", "pgup": "\x1b[5~", "ctrlc": "\x03"}


class Term:
    def __init__(self, w, h):
        self.w, self.h = w, h
        self.grid = [[" "] * w for _ in range(h)]
        self.x = self.y = 0

    def clear(self):
        self.grid = [[" "] * self.w for _ in range(self.h)]

    def put(self, ch):
        if self.y < self.h and self.x < self.w:
            self.grid[self.y][self.x] = ch
        self.x += 1
        if self.x >= self.w:
            self.x = self.w - 1

    def feed(self, data):
        i = 0
        n = len(data)
        while i < n:
            c = data[i]
            if c == "\x1b":
                m = re.match(r"\x1b\[([0-9;?]*)([A-Za-z])", data[i:])
                if m:
                    params, cmd = m.group(1), m.group(2)
                    self.csi(params, cmd)
                    i += m.end()
                    continue
                m = re.match(r"\x1b\][^\x07\x1b]*(\x07|\x1b\\)", data[i:])
                if m:
                    i += m.end(); continue
                m = re.match(r"\x1b[()][B0]|\x1b[=>78MD]", data[i:])
                if m:
                    i += m.end(); continue
                i += 1; continue
            if c == "\r":
                self.x = 0
            elif c == "\n":
                self.y += 1
                if self.y >= self.h: self.y = self.h - 1
            elif c == "\b":
                self.x = max(0, self.x - 1)
            elif c >= " ":
                self.put(c)
            i += 1

    def csi(self, params, cmd):
        ps = [int(p) for p in params.split(";") if p.isdigit()]
        p1 = ps[0] if ps else 1
        if params == "?1049" or cmd == "J" and params in ("2", "3"):
            if cmd == "J":
                self.clear(); self.x = self.y = 0
            return
        if cmd == "H" or cmd == "f":
            self.y = (ps[0] - 1 if len(ps) > 0 else 0) if ps else 0
            self.y = max(0, (ps[0] if ps else 1) - 1)
            self.x = max(0, (ps[1] if len(ps) > 1 else 1) - 1)
        elif cmd == "A": self.y = max(0, self.y - p1)
        elif cmd == "B": self.y = min(self.h - 1, self.y + p1)
        elif cmd == "C": self.x = min(self.w - 1, self.x + p1)
        elif cmd == "D": self.x = max(0, self.x - p1)
        elif cmd == "K":
            if p1 == 1:
                for i in range(self.x): self.grid[self.y][i] = " "
            else:
                for i in range(self.x, self.w): self.grid[self.y][i] = " "
        elif cmd == "G": self.x = max(0, p1 - 1)
        elif cmd == "M":
            self.y = max(0, self.y - 1)

    def dump(self):
        return "\n".join("".join(r).rstrip() for r in self.grid)


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


pump(3.0)
keys = [k for k in a.keys.split(",") if k] if a.keys else []
for k in keys:
    try:
        os.write(fd, KEYMAP.get(k, k).encode())
    except OSError:
        break
    pump(1.2 if k in ("enter",) else 0.4)
pump(max(0.3, a.wait - 3.0 - (1.2 if "enter" in keys else 0.4 * len(keys))))
try:
    os.write(fd, b"q"); pump(0.4)
except OSError:
    pass
try:
    os.kill(pid, 15)
except ProcessLookupError:
    pass
term.feed(b"".join(pending).decode("utf-8", "replace"))
print(term.dump())
