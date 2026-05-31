#!/usr/bin/env python3
"""Analyze captured live CLI output by replaying terminal redraw sequences."""
import re
import sys


def replay_screen(raw: str) -> tuple[list[str], dict[str, int]]:
    lines: list[str] = []
    cur = 0
    max_counts = {
        "live-render-parent": 0,
        "prepare-run": 0,
        "spawn-children": 0,
        "visible_lines": 0,
        "header_lines": 0,
        "blank_lines": 0,
    }

    def snapshot() -> None:
        visible = [ln for ln in lines if ln.strip()]
        blanks = sum(1 for ln in lines if not ln.strip())
        max_counts["visible_lines"] = max(max_counts["visible_lines"], len(visible))
        max_counts["blank_lines"] = max(max_counts["blank_lines"], blanks)
        header_lines = sum(
            1
            for ln in visible
            if "flow-live-render-parent" in ln
            or (re.search(r"ws-acme", ln) and re.match(r"^\s*[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏✓✗]", ln))
        )
        max_counts["header_lines"] = max(max_counts["header_lines"], header_lines)
        for key in ("live-render-parent", "prepare-run", "spawn-children"):
            count = sum(ln.count(key) for ln in visible)
            max_counts[key] = max(max_counts[key], count)

    saved_cur = 0
    has_saved = False

    i = 0
    while i < len(raw):
        if raw[i] == "\x1b" and i + 1 < len(raw) and raw[i + 1] == "[":
            j = i + 2
            while j < len(raw) and (raw[j].isdigit() or raw[j] in ";?"):
                j += 1
            if j >= len(raw):
                break
            cmd = raw[j]
            param = raw[i + 2 : j]
            if cmd == "H":
                cur = 0
            elif cmd == "J" and param == "2":
                lines = []
                cur = 0
            elif cmd == "J" and param in ("", "0"):
                lines = lines[: cur + 1]
                if lines:
                    lines[cur] = ""
            elif cmd == "A":
                cur = max(0, cur - int(param or "1"))
            elif cmd == "B":
                cur = cur + int(param or "1")
            elif cmd == "K":
                while len(lines) <= cur:
                    lines.append("")
                lines[cur] = ""
            elif cmd == "s":
                saved_cur = cur
                has_saved = True
            elif cmd == "u" and has_saved:
                cur = saved_cur
            i = j + 1
            snapshot()
            continue
        if raw[i] == "\r":
            i += 1
            continue
        if raw[i] == "\n":
            cur += 1
            while len(lines) <= cur:
                lines.append("")
            i += 1
            snapshot()
            continue
        while len(lines) <= cur:
            lines.append("")
        lines[cur] += raw[i]
        i += 1
        snapshot()
    return lines, max_counts


def main() -> int:
    path = sys.argv[1] if len(sys.argv) > 1 else "/tmp/live-qa-capture.txt"
    raw = open(path, "rb").read().decode("utf-8", errors="replace")
    _, counts = replay_screen(raw)
    restore = raw.count("\x1b[u")
    save = raw.count("\x1b[s")
    cursor_up = sum(1 for _ in re.finditer(r"\x1b\[(\d+)A", raw))
    erase_tail = raw.count("\x1b[J") - raw.count("\x1b[2J")

    print(f"capture: {path}")
    print(f"save-cursor: {save}, restore-cursor: {restore}, cursor-up: {cursor_up}, erase-to-eos: {erase_tail}")
    print(f"max non-blank on-screen lines: {counts['visible_lines']}")
    print(f"max blank lines on screen: {counts['blank_lines']}")
    print(f"max header-like lines: {counts['header_lines']}")
    print(f"max on-screen live-render-parent mentions: {counts['live-render-parent']}")
    print(f"max on-screen prepare-run mentions: {counts['prepare-run']}")
    print(f"max on-screen spawn-children mentions: {counts['spawn-children']}")

    failed = False
    if counts["blank_lines"] > 1:
        print("FAIL: blank lines visible during replay")
        failed = True
    if counts["header_lines"] > 2:
        print("FAIL: header lines duplicated on screen")
        failed = True
    if counts["live-render-parent"] > 2:
        print("FAIL: live-render-parent duplicated on screen")
        failed = True
    if counts["prepare-run"] > 1:
        print("FAIL: prepare-run duplicated on screen")
        failed = True
    if counts["spawn-children"] > 1:
        print("FAIL: spawn-children duplicated on screen")
        failed = True
    if cursor_up == 0 and restore == 0 and erase_tail == 0:
        print("FAIL: no in-place redraw sequences found")
        failed = True
    if not failed:
        print("PASS: replay shows in-place redraw without duplication or blank lines")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
