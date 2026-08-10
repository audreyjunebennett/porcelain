import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, patch

# The PC-side unit-test environment need not install the phone runtime's
# requests dependency. Tests replace its network calls below.
sys.modules.setdefault("requests", SimpleNamespace(post=None, get=None))

import record_v2


class RecordV2QueueTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.original_queue = record_v2.QUEUE_DIR
        self.original_log = record_v2.RECORD_LOG
        self.original_lock = record_v2.PROCESS_LOCK
        record_v2.QUEUE_DIR = self.temp.name
        record_v2.RECORD_LOG = str(Path(self.temp.name) / "record.log")
        record_v2.PROCESS_LOCK = str(Path(self.temp.name) / "record.lock")

    def tearDown(self):
        record_v2.QUEUE_DIR = self.original_queue
        record_v2.RECORD_LOG = self.original_log
        record_v2.PROCESS_LOCK = self.original_lock
        self.temp.cleanup()

    def test_process_lock_refuses_second_recorder(self):
        self.assertTrue(record_v2.acquire_process_lock())
        self.assertFalse(record_v2.acquire_process_lock())
        record_v2.release_process_lock()
        self.assertFalse(Path(record_v2.PROCESS_LOCK).exists())

    def test_recover_orphan_audio_creates_ready_metadata(self):
        capture_id = "2026-07-19_12-00-00_deadbeef"
        audio = Path(self.temp.name) / f"{capture_id}.aac"
        audio.write_bytes(b"x" * 2000)

        record_v2.recover_orphan_audio()

        metadata = json.loads(audio.with_suffix(".json").read_text(encoding="utf-8"))
        self.assertEqual(capture_id, metadata["capture_id"])
        self.assertEqual("2026-07-19_12-00-00", metadata["captured_at"])
        self.assertTrue(metadata["recovered"])

    @patch("record_v2.requests.post")
    def test_successful_delivery_sends_identity_then_removes_queue_pair(self, post):
        capture_id = "2026-07-19_12-00-00_deadbeef"
        audio = Path(self.temp.name) / f"{capture_id}.aac"
        metadata = audio.with_suffix(".json")
        audio.write_bytes(b"x" * 2000)
        record_v2.write_ready_metadata(
            str(metadata),
            {
                "capture_id": capture_id,
                "captured_at": "2026-07-19_12-00-00",
                "speaker": "Ruby",
            },
        )
        post.return_value = Mock(status_code=200, text="ok")

        delivered = record_v2.deliver(str(metadata))

        self.assertTrue(delivered)
        sent = post.call_args.kwargs["data"]
        self.assertEqual(capture_id, sent["capture_id"])
        self.assertEqual("2026-07-19_12-00-00", sent["captured_at"])
        self.assertEqual(
            (record_v2.CONNECT_TIMEOUT, record_v2.UPLOAD_TIMEOUT),
            post.call_args.kwargs["timeout"],
        )
        self.assertFalse(audio.exists())
        self.assertFalse(metadata.exists())

    @patch("record_v2.requests.post")
    def test_status_reports_ready_queue_depth(self, post):
        (Path(self.temp.name) / "first.json").write_text("{}", encoding="utf-8")
        (Path(self.temp.name) / "second.json").write_text("{}", encoding="utf-8")
        record_v2.LAST_CYCLE_SECONDS = 30.5

        record_v2.post_status()

        payload = post.call_args.kwargs["json"]
        self.assertEqual(2, payload["queue_depth"])
        self.assertEqual(30.5, payload["cycle_seconds"])


if __name__ == "__main__":
    unittest.main()
