import sqlite3
import tempfile
import unittest
from pathlib import Path

from motox_review import MotoXReviewStore
from motox_v1 import ChunkEvent, MotoXStore


class MotoXReviewStoreTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.database = root / "motox.sqlite3"
        self.audio = root / "sample.aac"
        self.audio.write_bytes(b"audio")
        journal = MotoXStore(self.database, root / "daily")
        journal.record_chunk(
            ChunkEvent(
                capture_id="clip-1",
                captured_at="2026-08-08_20-00-00",
                kind="speech",
                duration_seconds=30,
                speech_seconds=12,
                rms=0.1,
                audio_path=str(self.audio),
                transcript="Ruby built a mine chat house with Lynn.",
            )
        )
        self.review = MotoXReviewStore(self.database)

    def tearDown(self):
        self.temp.cleanup()

    def test_candidate_keeps_machine_transcript_and_audio_private(self):
        candidate = self.review.candidates(target_seconds=300)[0]
        self.assertEqual("clip-1", candidate["capture_id"])
        self.assertEqual("Unsorted", candidate["review_group"])
        self.assertNotIn("audio_path", candidate)

    def test_speaker_annotation_is_additive_and_counted(self):
        annotation = self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=10,
            annotation_type="speaker",
            label="Lynn",
        )
        candidate = self.review.candidates()[0]
        self.assertEqual("Lynn", candidate["annotations"][0]["label"])
        self.assertGreater(self.review.progress()["Lynn"]["seconds"], 0)
        self.assertLess(self.review.progress()["Lynn"]["seconds"], 12)
        self.assertTrue(self.review.revert_annotation(annotation["annotation_id"]))
        self.assertEqual([], self.review.candidates()[0]["annotations"])

    def test_word_correction_preserves_original_text(self):
        start = "Ruby built a ".__len__()
        annotation = self.review.add_annotation(
            capture_id="clip-1",
            start_char=start,
            end_char=start + len("mine chat"),
            annotation_type="transcript",
            replacement_text="Minecraft",
        )
        self.assertEqual("mine chat", annotation["selected_text"])
        self.assertEqual("Minecraft", annotation["replacement_text"])
        self.assertIn("mine chat", self.review.candidates()[0]["transcript"])

    def test_word_timed_pass_drives_review_and_projects_corrections(self):
        journal = MotoXStore(self.database, Path(self.temp.name) / "daily")
        journal.record_transcription_pass(
            "clip-1",
            "Ruby plays mine chat.",
            [
                {"word": "Ruby", "start_seconds": 0.1, "end_seconds": 0.5, "probability": 0.9, "char_start": 0, "char_end": 4},
                {"word": " plays", "start_seconds": 0.5, "end_seconds": 0.9, "probability": 0.9, "char_start": 4, "char_end": 10},
            ],
            model_version="large-v3",
        )
        self.review.add_annotation(
            capture_id="clip-1",
            start_char=11,
            end_char=20,
            annotation_type="transcript",
            replacement_text="Minecraft",
        )
        candidate = self.review.candidates()[0]
        self.assertEqual("Ruby plays mine chat.", candidate["transcript"])
        self.assertEqual("Ruby plays Minecraft.", candidate["corrected_transcript"])
        self.assertEqual(2, len(candidate["words"]))

    def test_context_expands_within_conversation(self):
        journal = MotoXStore(self.database, Path(self.temp.name) / "daily")
        journal.record_chunk(
            ChunkEvent(
                capture_id="clip-2",
                captured_at="2026-08-08_20-00-30",
                kind="speech",
                duration_seconds=30,
                speech_seconds=8,
                audio_path=str(self.audio),
                transcript="The monologue continues.",
            )
        )
        context = self.review.context("clip-1", radius=2)
        self.assertEqual(["clip-1", "clip-2"], [row["capture_id"] for row in context])

    def test_privacy_exclusion_removes_clip_from_review_queue(self):
        self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=0,
            annotation_type="privacy",
            label="Exclude",
        )
        self.assertEqual([], self.review.candidates())
        journal = MotoXStore(self.database, Path(self.temp.name) / "daily")
        self.assertEqual([], journal.recent_transcript())
        self.assertNotIn("Ruby built a mine chat house", journal.journal_text("2026-08-08"))

    def test_audio_range_annotation_can_overlap_text_and_counts_exact_time(self):
        annotation = self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=4,
            annotation_type="speaker",
            label="Ruby",
            audio_start_seconds=3.25,
            audio_end_seconds=8.75,
        )
        self.assertEqual(3.25, annotation["audio_start_seconds"])
        self.assertEqual(8.75, annotation["audio_end_seconds"])
        self.assertEqual("Ruby", annotation["selected_text"])
        self.assertEqual(5.5, self.review.progress()["Ruby"]["seconds"])

        music = self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=0,
            annotation_type="sound",
            label="Music",
            audio_start_seconds=3.25,
            audio_end_seconds=8.75,
        )
        self.assertEqual("", music["selected_text"])
        annotations = self.review.candidates()[0]["annotations"]
        self.assertEqual({"Ruby", "Music"}, {row["label"] for row in annotations})

    def test_audio_range_requires_a_forward_finite_pair(self):
        with self.assertRaises(ValueError):
            self.review.add_annotation(
                capture_id="clip-1",
                start_char=0,
                end_char=0,
                annotation_type="sound",
                label="Music",
                audio_start_seconds=8,
                audio_end_seconds=3,
            )

    def test_existing_annotation_table_migrates_additively(self):
        root = Path(self.temp.name)
        database = root / "legacy-review.sqlite3"
        MotoXStore(database, root / "legacy-daily")
        connection = sqlite3.connect(database)
        try:
            connection.execute(
                """
                CREATE TABLE review_annotations (
                    annotation_id TEXT PRIMARY KEY,
                    capture_id TEXT NOT NULL,
                    start_char INTEGER NOT NULL,
                    end_char INTEGER NOT NULL,
                    selected_text TEXT NOT NULL DEFAULT '',
                    annotation_type TEXT NOT NULL,
                    label TEXT,
                    replacement_text TEXT,
                    source TEXT NOT NULL DEFAULT 'human',
                    created_at TEXT NOT NULL,
                    reverted_at TEXT
                )
                """
            )
            connection.commit()
        finally:
            connection.close()
        MotoXReviewStore(database)
        connection = sqlite3.connect(database)
        try:
            columns = {
                row[1] for row in connection.execute("PRAGMA table_info(review_annotations)")
            }
        finally:
            connection.close()
        self.assertIn("audio_start_seconds", columns)
        self.assertIn("audio_end_seconds", columns)

    def test_rejects_unknown_labels(self):
        with self.assertRaises(ValueError):
            self.review.add_annotation(
                capture_id="clip-1",
                start_char=0,
                end_char=4,
                annotation_type="speaker",
                label="Definitely Ruby",
            )

    def test_model_proposal_remains_a_guess(self):
        self.assertEqual(
            1,
            self.review.add_proposals(
                [
                    {
                        "capture_id": "clip-1",
                        "cluster_id": "Voice group A",
                        "confidence": 0.72,
                    }
                ],
                model_name="wavlm-cluster",
                model_version="pilot-1",
            ),
        )
        candidate = self.review.candidates()[0]
        self.assertEqual("Voice group A", candidate["review_group"])
        self.assertEqual([], candidate["annotations"])


if __name__ == "__main__":
    unittest.main()
