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
        self.assertIn('href="/dashboard-assets/journal.css?v=20260906-speaker-link"', page)
        self.assertIn('class="conversation"', page)
        self.assertIn("Today’s journal", page)
        self.assertIn("&lt;script&gt;alert(&#x27;nope&#x27;)&lt;/script&gt;", page)
        self.assertNotIn("<script>alert('nope')</script>", page)
        self.assertIn('/api/motox/journal-audio/a_clip.aac', page)
        self.assertNotIn('**Ruby:**', page)
        self.assertNotIn("recording ended", page)
        self.assertNotIn(">completed<", page)
        self.assertNotIn('class="status"', page)

    def test_renderer_projects_conservative_speaker_layer(self):
        page = render_journal_html(
            SAMPLE,
            "2026-08-10",
            {
                "a_clip.aac": {
                    "capture_id": "capture-a",
                    "speakers": ["Ruby", "Lynn"],
                    "source": "predicted",
                }
            },
        )
        self.assertIn("Unsorted", page)
        self.assertNotIn("Ruby / Lynn", page)
        self.assertIn("1 speaker-enriched clips", page)

    def test_renderer_only_shows_status_for_active_recording(self):
        active = SAMPLE.replace("· completed", "· active")
        page = render_journal_html(active, "2026-08-10")
        self.assertIn('<span class="status">recording now</span>', page)

    def test_renderer_splits_timed_speakers_into_chat_bubbles(self):
        page = render_journal_html(
            SAMPLE,
            "2026-08-10",
            {
                "a_clip.aac": {
                    "capture_id": "capture-a",
                    "speakers": ["Ruby", "Lynn"],
                    "source": "predicted",
                    "segments": [
                        {
                            "speaker": "Ruby",
                            "source": "confirmed",
                            "text": "Hello",
                            "audio_start_seconds": 0.2,
                            "audio_end_seconds": 0.8,
                        },
                        {
                            "speaker": "Lynn",
                            "source": "predicted",
                            "text": "Hi back",
                            "audio_start_seconds": 1.0,
                            "audio_end_seconds": 1.8,
                        },
                    ],
                }
            },
        )
        self.assertIn('class="turn speaker-ruby"', page)
        self.assertIn('class="turn speaker-lynn"', page)
        self.assertNotIn("Ruby / Lynn", page)
        self.assertIn("Hello", page)
        self.assertIn("Hi back", page)
        self.assertIn('data-audio-start="0.200"', page)
        self.assertIn('class="speaker-edit" href="/review?capture=capture-a&amp;start=0.200&amp;end=0.800"', page)
        self.assertNotIn("Edit speaker", page)

    def test_narrow_untimed_identity_does_not_claim_the_whole_transcript(self):
        page = render_journal_html(
            SAMPLE,
            "2026-08-10",
            {
                "a_clip.aac": {
                    "capture_id": "capture-a",
                    "duration_seconds": 30.0,
                    "speakers": ["Ruby"],
                    "source": "confirmed",
                    "turns": [{
                        "speaker": "Ruby",
                        "audio_start_seconds": 4.0,
                        "audio_end_seconds": 8.0,
                    }],
                    "segments": [],
                }
            },
        )
        self.assertIn('class="turn speaker-unsorted"', page)
        self.assertNotIn('class="turn speaker-ruby"', page)

    def test_renderer_pages_long_journals_from_the_newest_conversations(self):
        many = "\n".join(
            SAMPLE.replace("conversation_2026-08-10_06-21-19", f"conversation-{index}")
            .replace("Hello", f"Message {index}")
            for index in range(15)
        )
        page = render_journal_html(
            many,
            "Last 24 hours",
            page=1,
            page_size=12,
            page_path="/journal/recent",
        )
        self.assertNotIn("Message 2", page)
        self.assertIn("Message 14", page)
        self.assertIn("15 conversations · showing 12", page)
        self.assertIn('/journal/recent?page=2', page)


if __name__ == "__main__":
    unittest.main()
