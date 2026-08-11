"""Safe, phone-friendly rendering for deterministic Moto X journal Markdown."""

from __future__ import annotations

import html
import re
from datetime import datetime
from urllib.parse import quote


HEADING_RE = re.compile(r"^##\s+(.+?)(?:\s+·\s+([^·]+))?$")
ANCHOR_RE = re.compile(r'^<a id="([^"]+)"></a>$')
TIME_RE = re.compile(r"^\*\*([^*]+)\*\*$")
SPEAKER_RE = re.compile(r"^\*\*([^*]+):\*\*\s*(.*)$")
AUDIO_RE = re.compile(r"^\[Audio\]\(\.\./audio/([^/)]+)\)$")
AMBIENT_RE = re.compile(r"^<!--\s*(.+?)\s*-->$")


def parse_journal(markdown: str) -> list[dict]:
    """Parse the small, deterministic Markdown grammar emitted by MotoXStore."""
    conversations: list[dict] = []
    current: dict | None = None
    pending_time = ""

    for raw_line in markdown.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("# "):
            continue

        heading = HEADING_RE.fullmatch(line)
        if heading:
            current = {
                "time_range": heading.group(1).strip(),
                "status": (heading.group(2) or "").strip(),
                "anchor": "",
                "turns": [],
                "notes": [],
            }
            conversations.append(current)
            pending_time = ""
            continue
        if current is None:
            continue

        anchor = ANCHOR_RE.fullmatch(line)
        if anchor:
            current["anchor"] = anchor.group(1)
            continue

        speaker = SPEAKER_RE.fullmatch(line)
        if speaker:
            current["turns"].append(
                {
                    "time": pending_time,
                    "speaker": speaker.group(1).strip(),
                    "text": speaker.group(2).strip(),
                    "audio": "",
                }
            )
            pending_time = ""
            continue

        timestamp = TIME_RE.fullmatch(line)
        if timestamp:
            pending_time = timestamp.group(1).strip()
            continue

        audio = AUDIO_RE.fullmatch(line)
        if audio and current["turns"]:
            current["turns"][-1]["audio"] = audio.group(1)
            continue

        ambient = AMBIENT_RE.fullmatch(line)
        if ambient:
            note = ambient.group(1).strip()
            if not note.startswith("Generated from "):
                current["notes"].append(note)
            continue

        # Preserve unexpected/multiline transcript text instead of dropping it.
        if pending_time:
            current["turns"].append(
                {
                    "time": pending_time,
                    "speaker": "Unsorted",
                    "text": line,
                    "audio": "",
                }
            )
            pending_time = ""
        elif current["turns"]:
            current["turns"][-1]["text"] = (
                current["turns"][-1]["text"] + "\n" + line
            ).strip()

    return conversations


def _speaker_class(speaker: str) -> str:
    normalized = re.sub(r"[^a-z0-9]+", "-", speaker.lower()).strip("-")
    return normalized or "unknown"


def render_journal_html(markdown: str, day: str) -> str:
    conversations = parse_journal(markdown)
    try:
        parsed_day = datetime.strptime(day, "%Y-%m-%d")
        display_day = parsed_day.strftime("%A, %B %d").replace(" 0", " ")
    except ValueError:
        display_day = day

    cards: list[str] = []
    turn_count = 0
    for index, conversation in enumerate(conversations):
        turns: list[str] = []
        for turn in conversation["turns"]:
            turn_count += 1
            speaker = html.escape(turn["speaker"])
            timestamp = html.escape(turn["time"])
            transcript = html.escape(turn["text"]).replace("\n", "<br>")
            audio = ""
            if turn["audio"]:
                filename = quote(turn["audio"], safe="")
                audio = (
                    '<details class="clip"><summary>▶ Play clip</summary>'
                    f'<audio controls preload="none" src="/api/motox/journal-audio/{filename}"></audio>'
                    "</details>"
                )
            turns.append(
                f'<article class="turn speaker-{_speaker_class(turn["speaker"])}">'
                '<div class="turn-meta">'
                f'<strong>{speaker}</strong><time>{timestamp}</time>'
                "</div>"
                f'<p>{transcript}</p>{audio}</article>'
            )

        notes = "".join(
            f'<div class="context-note">✦ {html.escape(note)}</div>'
            for note in conversation["notes"]
        )
        anchor = html.escape(conversation["anchor"] or f"conversation-{index}", quote=True)
        status = html.escape(conversation["status"])
        status_badge = f'<span class="status">{status}</span>' if status else ""
        cards.append(
            f'<section class="conversation" id="{anchor}">'
            '<header class="conversation-head">'
            f'<h2>{html.escape(conversation["time_range"])}</h2>{status_badge}'
            "</header>"
            f'{"".join(turns)}{notes}</section>'
        )

    if cards:
        body = "".join(cards)
    else:
        body = (
            '<section class="empty"><div class="empty-spark">✦</div>'
            "<h2>Quiet so far</h2><p>Today’s conversations will appear here.</p></section>"
        )

    return f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
  <meta name="theme-color" content="#09070d">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <link rel="stylesheet" href="/dashboard-assets/journal.css">
  <title>Today’s journal · Claudia</title>
</head>
<body>
  <main>
    <nav><a class="back" href="/dashboard" aria-label="Back to Claudia">‹</a><span>MOTO X MEMORY</span></nav>
    <header class="page-head">
      <div><p class="eyebrow">TODAY’S JOURNAL</p><h1>{html.escape(display_day)}</h1></div>
      <a class="teach" href="/review">teach Claudia</a>
    </header>
    <p class="summary">{len(conversations)} conversations · {turn_count} moments</p>
    <div class="journal">{body}</div>
    <footer><a href="/api/motox/journal/{html.escape(day, quote=True)}">view source Markdown</a></footer>
  </main>
</body>
</html>"""
