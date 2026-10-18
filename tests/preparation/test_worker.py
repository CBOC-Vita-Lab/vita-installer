import hashlib
import io
import json
import os
from pathlib import Path
import sys
import subprocess
import tempfile
import unittest

import pycdlib

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "workers"))
from prepare import inspect_infected as inspect_source, cook_galaxy


def inspect_infected(path):
    worker = os.environ.get("VITA_TEST_WORKER")
    if not worker:
        return inspect_source(path)
    result = subprocess.run([worker, "infected-inspect", str(path)], capture_output=True, text=True, timeout=20)
    if result.returncode:
        raise ValueError("invalid_disc")
    return json.loads(result.stdout)


def create_iso(path, boot):
    image = pycdlib.PyCdlib()
    image.new(interchange_level=3)
    image.add_directory(iso_path="/PSP_GAME")
    image.add_directory(iso_path="/PSP_GAME/SYSDIR")
    image.add_fp(io.BytesIO(boot), len(boot), iso_path="/PSP_GAME/SYSDIR/BOOT.BIN;1")
    image.write(str(path))
    image.close()


class IsoInspectionTests(unittest.TestCase):
    def test_hashes_actual_boot_content_without_extracting_or_changing_image(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "random.iso"
            create_iso(path, b"synthetic executable")
            original = path.read_bytes()
            result = inspect_infected(path)
            self.assertEqual(result["bootSha256"], hashlib.sha256(b"synthetic executable").hexdigest())
            self.assertEqual(result["bootBytes"], 20)
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(len(list(Path(directory).iterdir())), 1)

    def test_corrupt_iso_and_missing_boot_do_not_pass_as_game_data(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "Infected.iso"
            path.write_bytes(b"not an ISO")
            with self.assertRaises(ValueError):
                inspect_infected(path)
            image = pycdlib.PyCdlib()
            image.new()
            image.write(str(path))
            image.close()
            with self.assertRaises(ValueError):
                inspect_infected(path)

    def test_incomplete_galaxy_disc_cannot_leave_a_ready_partial_conversion(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, output = root / "disc", root / "cooked"
            (source / "sys").mkdir(parents=True)
            (source / "files" / "MovieData").mkdir(parents=True)
            (source / "sys" / "main.dol").write_bytes(b"synthetic, incomplete executable")
            (source / "sys" / "fst.bin").write_bytes(bytes.fromhex("010000000000000000000001"))
            (source / "files" / "MovieData" / "fixture.bin").write_bytes(b"test movie placeholder")
            worker = os.environ.get("VITA_TEST_WORKER")
            if worker:
                result = subprocess.run([worker, "galaxy-cook", str(source), str(output)], capture_output=True, text=True, timeout=60)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("galaxy_conversion_failed", result.stderr)
            else:
                with self.assertRaisesRegex(ValueError, "galaxy_conversion_failed"):
                    cook_galaxy(source, output)
            self.assertFalse(output.exists())
            self.assertEqual((source / "files" / "MovieData" / "fixture.bin").read_bytes(), b"test movie placeholder")


if __name__ == "__main__":
    unittest.main()
