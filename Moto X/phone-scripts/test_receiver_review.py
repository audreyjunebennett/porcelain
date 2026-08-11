import tempfile
import unittest
from pathlib import Path

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
        self.assertIn('id="range-start"', page.get_data(as_text=True))
        self.assertIn('id="range-end"', page.get_data(as_text=True))
        page.close()
        response = self.client.get("/api/motox/review/candidates?limit=1")
        self.assertEqual(200, response.status_code)
        self.assertEqual("review-clip", response.get_json()[0]["capture_id"])
        response.close()

    def test_dashboard_footer_stacks_actions_on_narrow_screens(self):
        response = self.client.get("/dashboard")
        self.assertEqual(200, response.status_code)
        html = response.get_data(as_text=True)
        self.assertIn('@media (max-width: 420px)', html)
        self.assertIn('footer .actions { display: grid;', html)
        self.assertIn('<span class="actions">', html)
        self.assertIn('`/journal/${status.today}`', html)
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
