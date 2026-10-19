#!/usr/bin/env python3
"""Мини-эмуляция терминала: /tmp/raw.bin -> кадр. Используется shot.py."""
import re
import sys


class Term:
    def __init__(self, w, h):
        self.w, self.h = w, h
        self.grid = [[" "] * w for _ in range(h)]
        self.saved = None
        self.x = self.y = 0

    def clear(self):
        self.save_frame()
        self.grid = [[" "] * self.w for _ in range(self.h)]

    def save_frame(self):
        nz = sum(1 for r in self.grid if "".join(r).strip())
        if nz > 0:
            self.saved = [r[:] for r in self.grid]

    def put(self, ch):
        if 0 <= self.y < self.h and 0 <= self.x < self.w:
            self.grid[self.y][self.x] = ch
        self.x += 1
        if self.x >= self.w:
            self.x = self.w - 1

    def csi(self, params, cmd):
        ps = [int(p) for p in params.split(";") if p.isdigit()]
        p1 = ps[0] if ps else 1
        if cmd == "J":
            if params in ("2", "3", ""):
                self.clear()
                self.x = self.y = 0
            elif p1 == 2:
                self.clear()
            return
        if params.startswith("?"):
            if params == "?1049" and cmd == "l":
                self.save_frame()
            return
        if cmd == "H" or cmd == "f":
            self.y = max(0, (ps[0] if ps else 1) - 1)
            self.x = max(0, (ps[1] if len(ps) > 1 else 1) - 1)
        elif cmd == "A":
            self.y = max(0, self.y - p1)
        elif cmd == "B":
            self.y = min(self.h - 1, self.y + p1)
        elif cmd == "C":
            self.x = min(self.w - 1, self.x + p1)
        elif cmd == "D":
            self.x = max(0, self.x - p1)
        elif cmd == "K":
            # 0 (default) = от курсора вправо, 1 = влево, 2 = вся строка
            k = ps[0] if ps else 0
            if k == 0:
                for i in range(self.x, self.w):
                    self.grid[self.y][i] = " "
            elif k == 1:
                for i in range(self.x + 1):
                    self.grid[self.y][i] = " "
            else:
                for i in range(self.w):
                    self.grid[self.y][i] = " "
        elif cmd == "G":
            self.x = max(0, p1 - 1)

    RE_CSI = re.compile(r"\x1b\[([0-9;?]*)([A-Za-z])")
    RE_OSC = re.compile(r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)")

    def feed(self, data):
        i, n = 0, len(data)
        while i < n:
            c = data[i]
            if c == "\x1b":
                m = self.RE_CSI.match(data, i)
                if m:
                    self.csi(m.group(1), m.group(2))
                    i = m.end()
                    continue
                m = self.RE_OSC.match(data, i)
                if m:
                    i = m.end()
                    continue
                i += 1
                continue
            if c == "\r":
                self.x = 0
            elif c == "\n":
                self.y = min(self.h - 1, self.y + 1)
            elif c == "\b":
                self.x = max(0, self.x - 1)
            elif c >= " ":
                self.put(c)
            i += 1

    def dump(self):
        g = self.saved
        if g is None or sum(1 for r in self.grid if "".join(r).strip()) > sum(1 for r in g if "".join(r).strip()):
            g = self.grid
        return "\n".join("".join(r).rstrip() for r in g)


if __name__ == "__main__":
    t = Term(120, 36)
    t.feed(open(sys.argv[1], "rb").read().decode("utf-8", "replace"))
    print(t.dump())
