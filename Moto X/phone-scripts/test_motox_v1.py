import tempfile
import unittest
from datetime import datetime
from pathlib import Path

from motox_v1 import ChunkEvent, MotoXStore, legacy_capture_identity


class LegacyCaptureIdentityTests(unittest.TestCase):
    def test_epoch_aac_filename_recovers_original_time_and_stable_id(self):
        identity = legacy_capture_identity("1784686082.aac")

        self.assertEqual("legacy-1784686082", identity[0])
        self.assertEqual(
            datetime.fromtimestamp(1784686082).strftime("%Y-%m-%d_%H-%M-%S"),
            identity[1],
        )

    def test_nonlegacy_filename_is_ignored(self):
        self.assertIsNone(legacy_capture_identity("motox_chunk.aac"))


class MotoXStoreTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.store = MotoXStore(root / "motox.sqlite3", root / "daily", gap_seconds=120)

    def tearDown(self):
        self.temp.cleanup()

    def add(self, capture_id, timestamp, kind, transcript="", audio_path=None):
        return self.store.record_chunk(
            ChunkEvent(
                capture_id=capture_id,
                captured_at=timestamp,
                kind=kind,
                duration_seconds=30,
                speech_seconds=3 if kind == "speech" else 0,
                rms=0.01,
                transcript=transcript,
                audio_path=audio_path,
            )
        )

    def test_speech_starts_conversation(self):
        result = self.add("a", "2026-07-19_01-00-00", "speech", "**Ruby:** Hi")
        self.assertEqual("conversation_2026-07-19_01-00-00", result["conversation_id"])
        self.assertEqual(
            "conversation_2026-07-19_01-00-00",
            self.store.status(datetime(2026, 7, 19, 1, 0, 10))["active_conversation"]["conversation_id"],
        )

    def test_ambient_does_not_start_conversation(self):
        result = self.add("a", "2026-07-19_01-00-00", "ambient")
        self.assertIsNone(result["conversation_id"])
        self.assertIsNone(self.store.status(datetime(2026, 7, 19, 1, 0, 10))["active_conversation"])

    def test_ambient_attaches_without_extending_speech_clock(self):
        self.add("speech-1", "2026-07-19_01-00-00", "speech", "First")
        attached = self.add("ambient", "2026-07-19_01-01-30", "ambient")
        closed = self.add("silence", "2026-07-19_01-02-00", "silence")
        self.assertEqual("conversation_2026-07-19_01-00-00", attached["conversation_id"])
        self.assertEqual("conversation_2026-07-19_01-00-00", closed["closed_conversation_id"])

    def test_exact_120_seconds_closes_conversation(self):
        self.add("speech-1", "2026-07-19_01-00-00", "speech", "First")
        result = self.add("speech-2", "2026-07-19_01-02-00", "speech", "Second")
        self.assertEqual("conversation_2026-07-19_01-00-00", result["closed_conversation_id"])
        self.assertEqual("conversation_2026-07-19_01-02-00", result["conversation_id"])

    def test_short_pause_keeps_conversation(self):
        first = self.add("speech-1", "2026-07-19_01-00-00", "speech", "First")
        second = self.add("speech-2", "2026-07-19_01-01-59", "speech", "Second")
        self.assertEqual(first["conversation_id"], second["conversation_id"])
        self.assertIsNone(second["closed_conversation_id"])

    def test_duplicate_capture_is_idempotent(self):
        first = self.add("same", "2026-07-19_01-00-00", "speech", "Once")
        second = self.add("same", "2026-07-19_01-00-00", "speech", "Twice")
        self.assertFalse(first["duplicate"])
        self.assertTrue(second["duplicate"])
        self.assertEqual(1, len(self.store.recent_transcript()))

    def test_daily_journal_is_deterministic_and_readable(self):
        audio = Path(self.temp.name) / "audio" / "a.aac"
        self.add("a", "2026-07-19_01-00-00", "speech", "**Ruby:** Hello", str(audio))
        self.add("b", "2026-07-19_01-00-30", "ambient")
        self.add("c", "2026-07-19_01-02-00", "silence")
        first = self.store.journal_text("2026-07-19")
        second = self.store.journal_text("2026-07-19")
        self.assertEqual(first, second)
        self.assertIn("· completed", first)
        self.assertIn("**Ruby:** Hello", first)
        self.assertIn("[Audio](../audio/a.aac)", first)
        self.assertIn("1 ambient context chunk", first)

    def test_recent_journal_is_a_rolling_24_hour_window(self):
        audio = Path(self.temp.name) / "audio" / "a.aac"
        self.add("old", "2026-07-18_23-59-59", "speech", "Too old", str(audio))
        self.add("yesterday", "2026-07-19_00-30-00", "speech", "Before midnight", str(audio))
        self.add("today", "2026-07-20_00-05-00", "speech", "After midnight", str(audio))

        recent = self.store.recent_journal_text(datetime(2026, 7, 20, 0, 15, 0))

        self.assertNotIn("Too old", recent)
        self.assertIn("Before midnight", recent)
        self.assertIn("After midnight", recent)
        self.assertIn("Jul 19", recent)
        self.assertIn("Jul 20", recent)

    def test_transcription_passes_are_versioned_with_word_timestamps(self):
        self.add("timed", "2026-07-19_02-00-00", "speech", "Hello world")
        first = self.store.record_transcription_pass(
            "timed",
            "Hello world",
            [
                {"word": "Hello", "start_seconds": 0.2, "end_seconds": 0.7, "probability": 0.9, "char_start": 0, "char_end": 5},
                {"word": " world", "start_seconds": 0.8, "end_seconds": 1.2, "probability": 0.8, "char_start": 5, "char_end": 11},
            ],
        )
        second = self.store.record_transcription_pass(
            "timed", "Hello, world", [], model_version="large-v3-repass"
        )
        with self.store._connection() as connection:
            passes = connection.execute(
                "SELECT pass_id, is_current FROM transcription_passes WHERE capture_id = 'timed' ORDER BY rowid"
            ).fetchall()
            word_count = connection.execute(
                "SELECT COUNT(*) FROM transcription_words WHERE pass_id = ?", (first,)
            ).fetchone()[0]
        self.assertEqual([(first, 0), (second, 1)], [(row[0], row[1]) for row in passes])
        self.assertEqual(2, word_count)

    def test_status_reports_capture_age(self):
        self.add("a", "2026-07-19_01-00-00", "silence")
        healthy = self.store.status(datetime(2026, 7, 19, 1, 0, 45))
        stale = self.store.status(datetime(2026, 7, 19, 1, 4, 0))
        self.assertEqual("healthy", healthy["capture_health"])
        self.assertEqual("stale", stale["capture_health"])

    def test_recorder_status_is_reported(self):
        self.store.update_recorder_status(
            "2026-07-19_12-00-00", "record_v2", 3, 30.75
        )

        status = self.store.status(datetime(2026, 7, 19, 12, 0, 1))

        self.assertEqual("record_v2", status["recorder"]["recorder"])
        self.assertEqual(3, status["recorder"]["queue_depth"])
        self.assertEqual(30.75, status["recorder"]["cycle_seconds"])


if __name__ == "__main__":
    unittest.main()
