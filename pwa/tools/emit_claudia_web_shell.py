#!/usr/bin/env python3
"""Emit pwa/static/claudia_web.html from Claudia Core mobile_orchestrator_api.py web_chat template."""
from pathlib import Path

REBIRTH = Path(__file__).resolve().parents[2]
SRC = Path(r"D:\Previously Claudia Core\Scripts\mobile_orchestrator_api.py")
OUT = REBIRTH / "pwa" / "static" / "claudia_web.html"

lines = SRC.read_text(encoding="utf-8").splitlines()
# web_chat(): skip "    html = """ line; static HTML starts at <!doctype> (line 4984, index 4983).
inject = """      <a href="/dashboard" class="sb-top-btn" title="Journal, quick facts, reminders">Dashboard</a>
"""
# Part A: from <!doctype through User button (line 5650)
part_a = "\n".join(lines[4983:5650])
# Part B: from Files link onward — skip concat lines 5651-5652 (indices 5650-5651)
part_b = "\n".join(lines[5652:5828])
html = part_a + "\n" + inject + "\n" + part_b
# Ensure script tag uses placeholder for cache bust (already in source as __BUILD_ID__)
OUT.parent.mkdir(parents=True, exist_ok=True)
OUT.write_text(html, encoding="utf-8")
print(f"Wrote {OUT.relative_to(REBIRTH)} ({len(html)} chars)")
