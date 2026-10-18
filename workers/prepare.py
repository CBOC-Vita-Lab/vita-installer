"""Bounded data inspection and full Galaxy cooking, without device access."""

import argparse
import contextlib
import hashlib
import json
import multiprocessing
import os
from pathlib import Path
import sys
import shutil

import pycdlib

VENDOR = Path(__file__).resolve().parent / "vendor" / "galaxy"
sys.path[:0] = [str(VENDOR / "cook"), str(VENDOR)]


def inspect_infected(path):
    path = Path(path)
    if path.is_symlink() or not path.is_file():
        raise ValueError("invalid_disc")
    image = pycdlib.PyCdlib()
    opened = False
    try:
        before = path.stat()
        image.open(str(path))
        opened = True
        digest = hashlib.sha256()
        size = 0
        with image.open_file_from_iso(iso_path="/PSP_GAME/SYSDIR/BOOT.BIN;1") as source:
            while chunk := source.read(1024 * 1024):
                size += len(chunk)
                if size > 64 * 1024 * 1024:
                    raise ValueError("invalid_disc")
                digest.update(chunk)
        after = path.stat()
        if (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (
            after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns
        ) or path.is_symlink() or not size:
            raise ValueError("source_changed")
        return {"bootSha256": digest.hexdigest(), "bootBytes": size}
    except (OSError, pycdlib.pycdlibexception.PyCdlibException) as error:
        raise ValueError("invalid_disc") from error
    finally:
        if opened:
            image.close()


def cook_galaxy(source, output):
    import cook
    import vita_layout

    source, output = Path(source), Path(output)
    if not source.is_dir() or source.is_symlink() or output.exists() or output.is_symlink():
        raise ValueError("unsafe_conversion_path")
    output.mkdir(parents=False)
    try:
        count = 0
        for path in source.rglob("*"):
            count += 1
            if path.is_symlink() or count > 100000 or not (path.is_file() or path.is_dir()):
                raise ValueError("unsafe_conversion_input")
        game = source if (source / "sys" / "fst.bin").is_file() else source / "DATA"
        for _directory, name, _a, _b in vita_layout.read_fst(game / "sys" / "fst.bin"):
            if name in (b".", b"..") or any(c in name for c in (b"/", b"\\", b":", b"\0")):
                raise ValueError("unsafe_conversion_input")
        with contextlib.redirect_stdout(sys.stderr):
            status = cook.convert(str(source), str(output), jobs=min(4, os.cpu_count() or 1), with_movies=True, vita_layout=True)
            if status or vita_layout.main(["vita_layout.py", str(output), "--verify"]):
                raise ValueError("galaxy_conversion_failed")
        return {"prepared": True, "movies": True, "layoutVerified": True}
    except BaseException as error:
        shutil.rmtree(output)
        if isinstance(error, (KeyboardInterrupt, SystemExit)):
            raise ValueError("galaxy_conversion_failed") from error
        raise


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    inspect = commands.add_parser("infected-inspect")
    inspect.add_argument("image", type=Path)
    galaxy = commands.add_parser("galaxy-cook")
    galaxy.add_argument("source", type=Path)
    galaxy.add_argument("output", type=Path)
    commands.add_parser("version")
    args = parser.parse_args()
    try:
        if args.command == "version":
            result = {"worker": "0.1.0", "pycdlib": "1.22.0"}
        elif args.command == "galaxy-cook":
            result = cook_galaxy(args.source, args.output)
        else:
            result = inspect_infected(args.image)
        print(json.dumps(result, separators=(",", ":")))
        return 0
    except ValueError as error:
        print(json.dumps({"error": str(error)}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    multiprocessing.freeze_support()
    sys.exit(main())
