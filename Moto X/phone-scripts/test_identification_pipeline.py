import tempfile
import unittest
from datetime import datetime, timedelta
from pathlib import Path

from identification_pipeline import (
    choose_batch,
    live_speech_is_recent,
    record_complete,
    record_started,
)
from motox_review import MotoXReviewStore
from motox_v1 import ChunkEvent, MotoXStore


class IdentificationPipelineTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.database = root / "motox.sqlite3"
        self.store = MotoXStore(self.database, root / "daily", gap_seconds=120)
        MotoXReviewStore(self.database)

    def tearDown(self):
        self.temp.cleanup()

    def add_speech(self, capture_id: str, captured_at: datetime) -> None:
        self.store.record_chunk(
            ChunkEvent(
                capture_id=capture_id,
                captured_at=captured_at.strftime("%Y-%m-%d_%H-%M-%S"),
                kind="speech",
                duration_seconds=30,
                speech_seconds=12,
                audio_path=str(Path(self.temp.name) / f"{capture_id}.aac"),
                transcript="**Unsorted:** useful speech",
                speaker="Unsorted",
            )
        )

    def test_closed_conversation_is_selected_once(self):
        started = datetime(2026, 9, 6, 8, 0, 0)
        self.add_speech("capture-a", started)
        self.add_speech("capture-b", started + timedelta(seconds=30))
        self.store.record_chunk(
            ChunkEvent(
                capture_id="silence",
                captured_at=(started + timedelta(seconds=150)).strftime(
                    "%Y-%m-%d_%H-%M-%S"
                ),
                kind="silence",
            )
        )
        batch = choose_batch(self.database, now=started + timedelta(minutes=5))
        self.assertEqual("final", batch.kind)
        self.assertEqual(("capture-a", "capture-b"), batch.capture_ids)

        report = Path(self.temp.name) / "report.json"
        record_started(self.database, batch, report)
        record_complete(self.database, batch)
        self.assertIsNone(
            choose_batch(self.database, now=started + timedelta(minutes=5))
        )

    def test_long_open_conversation_uses_quiet_checkpoint(self):
        started = datetime(2026, 9, 6, 8, 0, 0)
        for index in range(20):
            self.add_speech(
                f"capture-{index:02d}", started + timedelta(seconds=index * 30)
            )
        batch = choose_batch(
            self.database,
            now=started + timedelta(minutes=11),
            checkpoint_seconds=600,
            quiet_seconds=45,
            target_seconds=300,
        )
        self.assertEqual("checkpoint", batch.kind)
        self.assertEqual(10, len(batch.capture_ids))
        self.assertEqual("capture-19", batch.capture_ids[-1])

    def test_recent_live_speech_pauses_background_work(self):
        self.add_speech("capture-live", datetime.now())
        self.assertTrue(live_speech_is_recent(self.database, 75))
        self.assertFalse(live_speech_is_recent(self.database, 0))


if __name__ == "__main__":
    unittest.main()
