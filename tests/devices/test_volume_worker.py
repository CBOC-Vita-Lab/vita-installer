import io
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import sys
import subprocess
import stat
from types import SimpleNamespace

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "workers"))
import volume


class VolumeWorkerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve() / "CARD"
        self.root.mkdir()
        self.expected = (self.root.stat().st_dev, self.root.stat().st_ino)

    def tearDown(self):
        self.temp.cleanup()

    def write(self, path="data/job/part.bin", content=b"payload"):
        return volume.write_volume(self.root, self.expected, path, io.BytesIO(content))

    def test_new_file_is_flushed_and_existing_file_is_never_overwritten(self):
        self.write()
        self.assertEqual((self.root / "data/job/part.bin").read_bytes(), b"payload")
        with self.assertRaises(FileExistsError):
            self.write(content=b"replacement")

    def test_worker_translates_node_windows_ids_without_truncating_later_guards(self):
        info = SimpleNamespace(st_mode=stat.S_IFDIR, st_dev=0x12345678ABCDEF01,
                               st_ino=0x112233445566778899AABBCCDDEEFF00)
        with patch.object(volume.os, "name", "nt"), patch.object(volume.os, "lstat", return_value=info):
            expected = volume.worker_identity(self.root, (0xABCDEF01, 0x99AABBCCDDEEFF00))
            self.assertEqual(expected, (0x12345678ABCDEF01, 0x112233445566778899AABBCCDDEEFF00))
            volume.check_root(self.root, expected)
            info.st_dev = 0x87654321ABCDEF01
            with self.assertRaisesRegex(ValueError, "device_changed"):
                volume.check_root(self.root, expected)
            info.st_dev = expected[0]
            info.st_ino = 0xFFEEDDCCBBAA009999AABBCCDDEEFF00
            with self.assertRaisesRegex(ValueError, "device_changed"):
                volume.check_root(self.root, expected)

    def test_worker_rejects_a_different_windows_root_before_writing(self):
        info = SimpleNamespace(st_mode=stat.S_IFDIR, st_dev=0x12345678ABCDEF01,
                               st_ino=0x112233445566778899AABBCCDDEEFF00)
        with patch.object(volume.os, "name", "nt"), patch.object(volume.os, "lstat", return_value=info):
            for expected in [(0xABCDEF02, 0x99AABBCCDDEEFF00), (0xABCDEF01, 0x99AABBCCDDEEFF01)]:
                with self.assertRaisesRegex(ValueError, "device_changed"):
                    volume.worker_identity(self.root, expected)
            info.st_mode = stat.S_IFLNK
            with self.assertRaisesRegex(ValueError, "device_changed"):
                volume.worker_identity(self.root, (0xABCDEF01, 0x99AABBCCDDEEFF00))

    def test_symlink_parent_and_changed_root_are_rejected(self):
        outside = self.root.parent / "outside"
        outside.mkdir()
        (self.root / "data").symlink_to(outside, target_is_directory=True)
        with self.assertRaises((OSError, ValueError)):
            self.write()
        self.assertEqual(list(outside.iterdir()), [])
        self.root.rename(self.root.parent / "old")
        self.root.mkdir()
        with self.assertRaisesRegex(ValueError, "device_changed"):
            self.write()

    @unittest.skipIf(os.name == "nt", "POSIX descriptor race; Windows needs native runner")
    def test_parent_swap_cannot_redirect_a_write(self):
        parent = self.root / "data/job"
        parent.mkdir(parents=True)
        outside = self.root.parent / "outside"
        outside.mkdir()
        real_open = os.open

        def swapped(path, flags, *args, **kwargs):
            if path == "part.bin":
                parent.rename(self.root / "data/old-job")
                parent.symlink_to(outside, target_is_directory=True)
            return real_open(path, flags, *args, **kwargs)

        with patch.object(volume.os, "open", swapped):
            self.write()
        self.assertEqual(list(outside.iterdir()), [])
        self.assertEqual((self.root / "data/old-job/part.bin").read_bytes(), b"payload")

    def test_full_disk_and_failed_flush_never_report_success(self):
        with patch.object(volume.os, "write", side_effect=OSError("disk full")):
            with self.assertRaisesRegex(OSError, "disk full"):
                self.write()
        (self.root / "data/job/part.bin").unlink()
        with patch.object(volume.os, "fsync", side_effect=OSError("flush failed")):
            with self.assertRaisesRegex(OSError, "flush failed"):
                self.write()

    def test_atomic_move_preserves_an_existing_destination_and_checks_source_hash(self):
        import hashlib
        self.write()
        digest = hashlib.sha256(b"payload").hexdigest()
        with self.assertRaisesRegex(ValueError, "source_changed"):
            volume.move_volume(self.root, self.expected, "data/job/part.bin", "data/final.bin", "0" * 64)
        volume.move_volume(self.root, self.expected, "data/job/part.bin", "data/final.bin", digest)
        self.assertEqual((self.root / "data/final.bin").read_bytes(), b"payload")
        self.write(content=b"payload")
        with self.assertRaises(FileExistsError):
            volume.move_volume(self.root, self.expected, "data/job/part.bin", "data/final.bin", digest)
        self.assertEqual((self.root / "data/job/part.bin").read_bytes(), b"payload")

    def test_binary_bytes_are_unchanged_during_move_verification(self):
        import hashlib
        content = bytes(range(256)) + b"\r\n\x1a\r\n" + bytes(range(255, -1, -1))
        self.write(content=content)
        volume.move_volume(self.root, self.expected, "data/job/part.bin", "data/final.bin",
                           hashlib.sha256(content).hexdigest())
        self.assertEqual((self.root / "data/final.bin").read_bytes(), content)

    @unittest.skipUnless(os.environ.get("VITA_TEST_VOLUME_WORKER"), "packaged worker not selected")
    def test_packaged_worker_writes_without_python_on_path(self):
        command_identity = self.expected
        if os.name == "nt":
            command_identity = (self.expected[0] & 0xFFFFFFFF, self.expected[1] & 0xFFFFFFFFFFFFFFFF)
        command = [os.environ["VITA_TEST_VOLUME_WORKER"], "write", str(self.root),
                   *(str(n) for n in command_identity), "data/packaged/part.bin"]
        result = subprocess.run(command, input=b"packaged payload", capture_output=True,
                                timeout=20, env={"PATH": "", "SystemRoot": os.environ.get("SystemRoot", "")})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((self.root / "data/packaged/part.bin").read_bytes(), b"packaged payload")
        rejected = subprocess.run(command, input=b"replacement", capture_output=True, timeout=20)
        self.assertNotEqual(rejected.returncode, 0)
        import hashlib
        moved = subprocess.run([command[0], "move", *command[2:6], "data/packaged/final.bin",
                                hashlib.sha256(b"packaged payload").hexdigest()], capture_output=True, timeout=20)
        self.assertEqual(moved.returncode, 0, moved.stdout + moved.stderr)
        self.assertEqual((self.root / "data/packaged/final.bin").read_bytes(), b"packaged payload")


if __name__ == "__main__":
    unittest.main()
