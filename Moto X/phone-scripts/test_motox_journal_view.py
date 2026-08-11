import unittest

from motox_journal_view import parse_journal, render_journal_html


SAMPLE = """# Moto X Journal — 2026-08-10

<!-- Generated from motox_v1.sqlite3. Rebuild instead of editing this file. -->

## 6:21:19 AM–6:23:54 AM · completed

<a id="conversation_2026-08-10_06-21-19"></a>

**6:21:19 AM**

**Ruby:** Hello <script>alert('nope')</script>

[Audio](../audio/a_clip.aac)

<!-- 1 ambient context chunk(s) attached. -->
"""


class MotoXJournalViewTests(unittest.TestCase):
    def test_parser_extracts_conversation_turn_and_audio(self):
        conversations = parse_journal(SAMPLE)
        self.assertEqual(1, len(conversations))
        self.assertEqual("completed", conversations[0]["status"])
        self.assertEqual("Ruby", conversations[0]["turns"][0]["speaker"])
        self.assertEqual("a_clip.aac", conversations[0]["turns"][0]["audio"])
        self.assertEqual(["1 ambient context chunk(s) attached."], conversations[0]["notes"])

    def test_renderer_is_dark_readable_and_escapes_transcript(self):
        page = render_journal_html(SAMPLE, "2026-08-10")
        self.assertIn('href="/dashboard-assets/journal.css"', page)
        self.assertIn('class="conversation"', page)
        self.assertIn("Today’s journal", page)
        self.assertIn("&lt;script&gt;alert(&#x27;nope&#x27;)&lt;/script&gt;", page)
        self.assertNotIn("<script>alert('nope')</script>", page)
        self.assertIn('/api/motox/journal-audio/a_clip.aac', page)
        self.assertNotIn('**Ruby:**', page)


if __name__ == "__main__":
    unittest.main()
