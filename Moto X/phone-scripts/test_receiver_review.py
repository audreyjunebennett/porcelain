import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

import receiver
from motox_review import MotoXReviewStore
from motox_v1 import ChunkEvent, MotoXStore


class ReceiverReviewApiTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.audio_dir = root / "audio"
        self.audio_dir.mkdir()
        self.audio = self.audio_dir / "clip.aac"
        self.audio.write_bytes(b"0123456789" * 100)
        database = root / "motox.sqlite3"
        journal = MotoXStore(database, root / "daily")
        journal.record_chunk(
            ChunkEvent(
                capture_id="review-clip",
                captured_at="2026-08-08_20-00-00",
                kind="speech",
                duration_seconds=30,
                speech_seconds=10,
                rms=0.1,
                audio_path=str(self.audio),
                transcript="Ruby plays mine chat with Lynn.",
            )
        )
        self.original_store = receiver.REVIEW_STORE
        self.original_v1_store = receiver.V1_STORE
        self.original_audio_dir = receiver.AUDIO_DIR
        receiver.REVIEW_STORE = MotoXReviewStore(database)
        self.speaker_turn_id = "e" * 32
        receiver.REVIEW_STORE.add_speaker_turns(
            [{
                "turn_id": self.speaker_turn_id,
                "capture_id": "review-clip",
                "audio_start_seconds": 4.0,
                "audio_end_seconds": 8.0,
                "cluster_id": "api-report/SPEAKER_00",
                "embedding": [1.0, 0.0],
                "quality": 1.0,
            }],
            embedding_model="test-embedding",
            embedding_version="1",
            diarization_model="test-diarization",
            source_id="api-report",
        )
        receiver.V1_STORE = journal
        receiver.AUDIO_DIR = self.audio_dir
        self.client = receiver.APP.test_client()

    def tearDown(self):
        receiver.REVIEW_STORE = self.original_store
        receiver.V1_STORE = self.original_v1_store
        receiver.AUDIO_DIR = self.original_audio_dir
        self.temp.cleanup()

    def test_review_page_and_candidate_api(self):
        page = self.client.get("/review")
        self.assertEqual(200, page.status_code)
        self.assertIn('data-label="Music"', page.get_data(as_text=True))
        self.assertIn('id="trim-start"', page.get_data(as_text=True))
        self.assertIn('id="trim-end"', page.get_data(as_text=True))
        self.assertNotIn('id="range-start"', page.get_data(as_text=True))
        self.assertNotIn('id="range-end"', page.get_data(as_text=True))
        self.assertNotIn('id="range-loop"', page.get_data(as_text=True))
        self.assertNotIn('id="range-clear"', page.get_data(as_text=True))
        self.assertIn('id="transcript-version"', page.get_data(as_text=True))
        self.assertIn('data-label="Unknown person"', page.get_data(as_text=True))
        self.assertIn('id="load-context"', page.get_data(as_text=True))
        script = self.client.get("/dashboard-assets/review.js")
        script_text = script.get_data(as_text=True)
        self.assertIn("syncTranscriptToAudioRange", script_text)
        self.assertIn("transcript-selection", script_text)
        self.assertIn("pendingNonSpeakerLabels", script_text)
        self.assertIn("chooseNonSpeaker", script_text)
        self.assertIn("Confirm ${pendingSounds[0].label} & next", script_text)
        script.close()
        self.assertIn('id="exclude"', page.get_data(as_text=True))
        self.assertIn('value="identify"', page.get_data(as_text=True))
        self.assertIn('<meta name="theme-color" content="#000000">', page.get_data(as_text=True))
        self.assertIn('<button id="scope"', page.get_data(as_text=True))
        self.assertIn('class="transcript-actions"', page.get_data(as_text=True))
        page.close()
        response = self.client.get("/api/motox/review/candidates?limit=1")
        self.assertEqual(200, response.status_code)
        self.assertEqual("review-clip", response.get_json()[0]["capture_id"])
        response.close()

    def test_pwa_manifest_and_pages_request_an_opaque_dark_shell(self):
        manifest = self.client.get("/dashboard-assets/manifest.webmanifest")
        self.assertEqual("#000000", manifest.get_json()["theme_color"])
        self.assertEqual("#000000", manifest.get_json()["background_color"])
        self.assertEqual("standalone", manifest.get_json()["display"])
        self.assertIn("no-cache", manifest.headers.get("Cache-Control", ""))
        manifest.close()

        dashboard = self.client.get("/dashboard").get_data(as_text=True)
        journal = self.client.get("/journal/recent").get_data(as_text=True)
        for page in (dashboard, journal):
            self.assertIn('<meta name="theme-color" content="#000000">', page)
            self.assertIn('<meta name="color-scheme" content="dark">', page)
            self.assertIn('rel="manifest" href="/dashboard-assets/manifest.webmanifest"', page)

    def test_recent_journal_route_uses_rolling_view(self):
        page = self.client.get("/journal/recent")
        self.assertEqual(200, page.status_code)
        text = page.get_data(as_text=True)
        self.assertIn("Last 24 hours", text)
        self.assertIn('/api/motox/journal/recent', text)
        page.close()

    def test_exact_review_capture_api_supports_journal_edit_links(self):
        response = self.client.get("/api/motox/review/capture/review-clip")
        self.assertEqual(200, response.status_code)
        self.assertEqual("review-clip", response.get_json()["capture_id"])
        response.close()

    def test_identification_api_returns_and_accepts_exact_turn(self):
        response = self.client.get("/api/motox/review/identification?limit=1")
        self.assertEqual(200, response.status_code)
        candidate = response.get_json()[0]
        question = candidate["identification"]
        self.assertEqual(self.speaker_turn_id, question["turn_id"])
        self.assertEqual((4.0, 8.0), (
            question["audio_start_seconds"], question["audio_end_seconds"]
        ))
        response.close()

        saved = self.client.post(
            "/api/motox/review/annotations",
            json={
                "capture_id": "review-clip",
                "start_char": 0,
                "end_char": 0,
                "annotation_type": "speaker",
                "label": "Ruby",
                "speaker_turn_id": self.speaker_turn_id,
            },
        )
        self.assertEqual(201, saved.status_code)
        self.assertEqual((4.0, 8.0), (
            saved.get_json()["audio_start_seconds"], saved.get_json()["audio_end_seconds"]
        ))
        saved.close()
        self.assertEqual([], self.client.get("/api/motox/review/identification").get_json())

    def test_dashboard_keeps_actions_above_recent_transcripts(self):
        response = self.client.get("/dashboard")
        self.assertEqual(200, response.status_code)
        html = response.get_data(as_text=True)
        self.assertIn('@media (max-width: 420px)', html)
        self.assertIn('.top-actions { display: grid;', html)
        self.assertIn('<nav class="top-actions">', html)
        self.assertLess(html.index('<nav class="top-actions">'), html.index('<section id="recent">'))
        self.assertNotIn('<span class="actions">', html)
        self.assertNotIn("Claudia is listening", html)
        self.assertGreater(html.index('id="counts"'), html.index('<section id="recent">'))
        self.assertIn("'/journal/recent'", html)
        self.assertIn("offset=${recentRows.length}", html)
        self.assertIn("IntersectionObserver", html)
        self.assertIn("renderRecent(recentRows)", html)
        self.assertIn('class="bubble speaker-${speakerClass(segment.speaker)}"', html)
        self.assertIn('data-quick-speaker="Unsorted"', html)
        self.assertIn('data-quick-speaker="Ruby"', html)
        self.assertIn('data-quick-speaker="Lynn"', html)
        self.assertIn("addEventListener('scroll', closeSpeakerMenus", html)
        response.close()

    def test_recent_feed_api_supports_offset_pages(self):
        first = self.client.get("/api/motox/recent?limit=1&offset=0")
        self.assertEqual(200, first.status_code)
        self.assertEqual(1, len(first.get_json()))
        second = self.client.get("/api/motox/recent?limit=1&offset=1")
        self.assertEqual(200, second.status_code)
        self.assertEqual([], second.get_json())
        first.close()
        second.close()

    def test_dashboard_quick_speaker_can_label_and_clear_recent_audio(self):
        saved = self.client.post(
            "/api/motox/review/quick-speaker",
            json={
                "capture_id": "review-clip",
                "audio_start_seconds": 0,
                "audio_end_seconds": 30,
                "label": "Ruby",
            },
        )
        self.assertEqual(200, saved.status_code)
        self.assertEqual("Ruby", saved.get_json()["label"])
        saved.close()
        recent = self.client.get("/api/motox/recent?limit=8").get_json()
        self.assertEqual("Ruby", recent[0]["segments"][0]["speaker"])
        self.assertEqual(30.0, recent[0]["segments"][0]["audio_end_seconds"])

        cleared = self.client.post(
            "/api/motox/review/quick-speaker",
            json={
                "capture_id": "review-clip",
                "audio_start_seconds": 0,
                "audio_end_seconds": 30,
                "label": "Unsorted",
            },
        )
        self.assertEqual(200, cleared.status_code)
        self.assertEqual("Unsorted", cleared.get_json()["label"])
        cleared.close()
        recent = self.client.get("/api/motox/recent?limit=8").get_json()
        self.assertEqual("Unsorted", recent[0]["segments"][0]["speaker"])

    def test_recent_feed_projects_timed_speaker_bubbles(self):
        receiver.V1_STORE.record_transcription_pass(
            "review-clip",
            "Ruby plays mine chat with Lynn.",
            [{
                "word": "Ruby plays",
                "start_seconds": 4.1,
                "end_seconds": 5.0,
                "probability": 0.9,
                "char_start": 0,
                "char_end": 10,
            }],
        )
        receiver.REVIEW_STORE.add_annotation(
            capture_id="review-clip",
            start_char=0,
            end_char=10,
            annotation_type="speaker",
            label="Ruby",
            speaker_turn_id=self.speaker_turn_id,
        )
        response = self.client.get("/api/motox/recent?limit=8")
        self.assertEqual(200, response.status_code)
        segment = response.get_json()[0]["segments"][0]
        self.assertEqual("Ruby", segment["speaker"])
        self.assertEqual("Ruby plays", segment["text"])
        response.close()

    def test_journal_page_is_rendered_and_raw_markdown_is_preserved(self):
        page = self.client.get("/journal/2026-08-08")
        self.assertEqual(200, page.status_code)
        self.assertIn('class="conversation"', page.get_data(as_text=True))
        self.assertNotIn("**Ruby:**", page.get_data(as_text=True))
        page.close()

        source = self.client.get("/api/motox/journal/2026-08-08")
        self.assertEqual(200, source.status_code)
        self.assertIn("Ruby plays mine chat with Lynn.", source.get_data(as_text=True))
        source.close()

    def test_journal_audio_supports_range_requests(self):
        response = self.client.get(
            "/api/motox/journal-audio/clip.aac", headers={"Range": "bytes=0-9"}
        )
        self.assertEqual(206, response.status_code)
        self.assertEqual(10, len(response.data))
        response.close()

    def test_audio_supports_range_requests(self):
        response = self.client.get(
            "/api/motox/review/audio/review-clip", headers={"Range": "bytes=0-9"}
        )
        self.assertEqual(206, response.status_code)
        self.assertEqual(10, len(response.data))
        response.close()

    def test_context_api_returns_current_clip(self):
        response = self.client.get("/api/motox/review/context/review-clip?radius=2")
        self.assertEqual(200, response.status_code)
        self.assertEqual("review-clip", response.get_json()[0]["capture_id"])

    def test_timed_words_preserve_text_and_offsets(self):
        segments = [
            SimpleNamespace(
                words=[
                    SimpleNamespace(word=" Hello", start=0.2, end=0.6, probability=0.91),
                    SimpleNamespace(word=",", start=0.6, end=0.7, probability=0.88),
                    SimpleNamespace(word=" world", start=0.8, end=1.2, probability=0.92),
                ]
            )
        ]
        text, words = receiver.timed_words_from_segments(segments)
        self.assertEqual("Hello, world", text)
        self.assertEqual((0, 5), (words[0]["char_start"], words[0]["char_end"]))
        self.assertEqual((6, 12), (words[2]["char_start"], words[2]["char_end"]))

    def test_annotation_can_be_saved_and_undone(self):
        response = self.client.post(
            "/api/motox/review/annotations",
            json={
                "capture_id": "review-clip",
                "start_char": 0,
                "end_char": 4,
                "annotation_type": "speaker",
                "label": "Ruby",
            },
        )
        self.assertEqual(201, response.status_code)
        annotation = response.get_json()
        self.assertEqual("Ruby", annotation["selected_text"])
        undone = self.client.delete(
            "/api/motox/review/annotations/" + annotation["annotation_id"]
        )
        self.assertEqual(200, undone.status_code)

    def test_audio_range_annotation_api(self):
        response = self.client.post(
            "/api/motox/review/annotations",
            json={
                "capture_id": "review-clip",
                "start_char": 0,
                "end_char": 0,
                "audio_start_seconds": 4.2,
                "audio_end_seconds": 11.8,
                "annotation_type": "sound",
                "label": "Music",
            },
        )
        self.assertEqual(201, response.status_code)
        annotation = response.get_json()
        self.assertEqual("", annotation["selected_text"])
        self.assertEqual(4.2, annotation["audio_start_seconds"])
        self.assertEqual(11.8, annotation["audio_end_seconds"])
        response.close()


if __name__ == "__main__":
    unittest.main()
