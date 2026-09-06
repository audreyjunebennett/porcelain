import sqlite3
import tempfile
import unittest
from pathlib import Path

from build_identification_turns import merge_turns
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

    def test_duplicate_active_annotation_is_idempotent(self):
        values = {
            "capture_id": "clip-1",
            "start_char": 0,
            "end_char": 0,
            "annotation_type": "sound",
            "label": "Television",
            "audio_start_seconds": 3.25,
            "audio_end_seconds": 8.75,
        }
        first = self.review.add_annotation(**values)
        second = self.review.add_annotation(**values)
        self.assertEqual(first["annotation_id"], second["annotation_id"])
        television = [
            row for row in self.review.candidates()[0]["annotations"]
            if row["label"] == "Television"
        ]
        self.assertEqual(1, len(television))

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
        self.assertIn("speaker_turn_id", columns)

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

    def test_adaptive_identification_asks_for_an_exact_uncertain_turn(self):
        ruby_turn = "a" * 32
        lynn_turn = "b" * 32
        uncertain_turn = "c" * 32
        stored = self.review.add_speaker_turns(
            [
                {
                    "turn_id": ruby_turn,
                    "capture_id": "clip-1",
                    "conversation_id": "conversation-1",
                    "audio_start_seconds": 1.0,
                    "audio_end_seconds": 4.0,
                    "cluster_id": "report-1/SPEAKER_00",
                    "embedding": [1.0, 0.0, 0.0],
                    "quality": 0.9,
                },
                {
                    "turn_id": lynn_turn,
                    "capture_id": "clip-1",
                    "conversation_id": "conversation-1",
                    "audio_start_seconds": 5.0,
                    "audio_end_seconds": 8.0,
                    "cluster_id": "report-1/SPEAKER_01",
                    "embedding": [0.0, 1.0, 0.0],
                    "quality": 0.9,
                },
                {
                    "turn_id": uncertain_turn,
                    "capture_id": "clip-1",
                    "conversation_id": "conversation-1",
                    "audio_start_seconds": 10.25,
                    "audio_end_seconds": 14.25,
                    "cluster_id": "report-1/SPEAKER_02",
                    "embedding": [1.0, 1.0, 0.0],
                    "quality": 1.0,
                },
            ],
            embedding_model="test-embedding",
            embedding_version="1",
            diarization_model="test-diarization",
            source_id="report-1",
        )
        self.assertEqual(3, stored)
        ruby = self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=0,
            annotation_type="speaker",
            label="Ruby",
            audio_start_seconds=0,
            audio_end_seconds=0.5,
            speaker_turn_id=ruby_turn,
        )
        self.assertEqual((1.0, 4.0), (ruby["audio_start_seconds"], ruby["audio_end_seconds"]))
        self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=0,
            annotation_type="speaker",
            label="Lynn",
            speaker_turn_id=lynn_turn,
        )

        candidates = self.review.identification_candidates()
        self.assertEqual(1, len(candidates))
        question = candidates[0]["identification"]
        self.assertEqual(uncertain_turn, question["turn_id"])
        self.assertEqual("Ruby or Lynn?", question["prompt"])
        self.assertEqual((10.25, 14.25), (
            question["audio_start_seconds"], question["audio_end_seconds"]
        ))
        self.assertEqual({"Ruby": 1, "Lynn": 1, "Raven": 0}, question["profile_examples"])

    def test_identification_turn_answer_is_reversible_and_reenters_queue(self):
        turn_id = "d" * 32
        self.review.add_speaker_turns(
            [{
                "turn_id": turn_id,
                "capture_id": "clip-1",
                "audio_start_seconds": 2.0,
                "audio_end_seconds": 6.0,
                "cluster_id": "report-2/SPEAKER_00",
                "embedding": [0.5, 0.5],
                "quality": 0.8,
            }],
            embedding_model="test-embedding",
            embedding_version="1",
            diarization_model="test-diarization",
            source_id="report-2",
        )
        self.assertEqual(turn_id, self.review.identification_candidates()[0]["identification"]["turn_id"])
        annotation = self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=0,
            annotation_type="speaker",
            label="Not sure",
            speaker_turn_id=turn_id,
        )
        self.assertEqual([], self.review.identification_candidates())
        self.assertTrue(self.review.revert_annotation(annotation["annotation_id"]))
        self.assertEqual(turn_id, self.review.identification_candidates()[0]["identification"]["turn_id"])

    def test_sound_or_overlap_annotation_handles_matching_identification_turn(self):
        turn_id = "e" * 32
        self.review.add_speaker_turns(
            [{
                "turn_id": turn_id,
                "capture_id": "clip-1",
                "audio_start_seconds": 2.0,
                "audio_end_seconds": 6.0,
                "cluster_id": "report-3/SPEAKER_00",
                "embedding": [0.5, 0.5],
                "quality": 0.8,
            }],
            embedding_model="test-embedding",
            embedding_version="1",
            diarization_model="test-diarization",
            source_id="report-3",
        )
        annotation = self.review.add_annotation(
            capture_id="clip-1",
            start_char=0,
            end_char=0,
            annotation_type="sound",
            label="Television",
            audio_start_seconds=2.0,
            audio_end_seconds=6.0,
        )
        self.assertEqual([], self.review.identification_candidates())
        self.assertTrue(self.review.revert_annotation(annotation["annotation_id"]))
        self.assertEqual(turn_id, self.review.identification_candidates()[0]["identification"]["turn_id"])

    def test_identification_batch_uses_each_capture_once(self):
        self.review.add_speaker_turns(
            [
                {
                    "turn_id": "f" * 32,
                    "capture_id": "clip-1",
                    "audio_start_seconds": 2.0,
                    "audio_end_seconds": 5.0,
                    "cluster_id": "report-4/SPEAKER_00",
                    "embedding": [1.0, 0.0],
                    "quality": 0.9,
                },
                {
                    "turn_id": "0" * 32,
                    "capture_id": "clip-1",
                    "audio_start_seconds": 7.0,
                    "audio_end_seconds": 10.0,
                    "cluster_id": "report-4/SPEAKER_01",
                    "embedding": [0.0, 1.0],
                    "quality": 0.8,
                },
            ],
            embedding_model="test-embedding",
            embedding_version="1",
            diarization_model="test-diarization",
            source_id="report-4",
        )
        candidates = self.review.identification_candidates(limit=10)
        self.assertEqual(1, len(candidates))
        self.assertEqual("clip-1", candidates[0]["capture_id"])
        self.assertEqual(
            [],
            self.review.identification_candidates(exclude_capture_ids={"clip-1"}),
        )

    def test_repeated_television_marks_contaminate_a_source_report(self):
        turns = [
            {"capture_id": f"clip-{index}", "source_id": "tv-report"}
            for index in range(1, 5)
        ]
        annotations = [
            {
                "capture_id": f"clip-{index}",
                "annotation_type": "sound",
                "label": "Television",
            }
            for index in range(1, 4)
        ]
        annotations.append({
            "capture_id": "clip-4",
            "annotation_type": "speaker",
            "label": "Ruby",
        })
        self.assertEqual(
            {"tv-report"},
            self.review._television_contaminated_sources(turns, annotations),
        )

    def test_diarization_fragments_merge_into_question_sized_turns(self):
        turns = merge_turns(
            [
                {"capture_id": "clip-1", "speaker_cluster": "A", "source_start": 1.0, "source_end": 2.0},
                {"capture_id": "clip-1", "speaker_cluster": "A", "source_start": 2.2, "source_end": 4.5},
                {"capture_id": "clip-1", "speaker_cluster": "B", "source_start": 5.0, "source_end": 5.4},
                {"capture_id": "clip-1", "speaker_cluster": "A", "source_start": 6.0, "source_end": 18.0},
            ],
            min_seconds=1.5,
            max_seconds=8.0,
            merge_gap=0.35,
        )
        self.assertEqual(2, len(turns))
        self.assertEqual((1.0, 4.5), (turns[0]["source_start"], turns[0]["source_end"]))
        self.assertEqual(4.0, turns[1]["source_end"] - turns[1]["source_start"])


if __name__ == "__main__":
    unittest.main()
