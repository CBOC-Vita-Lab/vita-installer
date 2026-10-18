"""Rooted removable-volume writes. No discovery, formatting or app promotion."""
import argparse
from contextlib import contextmanager, ExitStack
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys


def parts_for(relative):
    parts = relative.split("/")
    if len(relative) > 1024 or any(
        not p or p in (".", "..") or p.endswith((".", " "))
        or re.search(r'[\\:\x00-\x1f\x7f;*?"<>|]', p)
        or re.match(r"^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)", p, re.I)
        for p in parts
    ):
        raise ValueError("unsafe_destination")
    return parts


def check_root(root, expected):
    try:
        info = os.lstat(root)
    except OSError as error:
        raise ValueError("device_changed") from error
    if not stat.S_ISDIR(info.st_mode) or (info.st_dev, info.st_ino) != expected:
        raise ValueError("device_changed")


def windows_open(path, directory=False, read=False):
    import ctypes
    from ctypes import wintypes
    import msvcrt

    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    create = kernel.CreateFileW
    create.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, ctypes.c_void_p,
                       wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE]
    create.restype = wintypes.HANDLE
    # A held directory handle denies rename/delete and reparse-point mutation.
    handle = create(str(path), 0x80 if directory else (0x80000000 if read else 0x40000000), 1 if directory or read else 0,
                    None, 3 if directory or read else 1, 0x00200000 | (0x02000000 if directory else 0x80), None)
    if handle == wintypes.HANDLE(-1).value:
        error = ctypes.get_last_error()
        if error in (80, 183):
            raise FileExistsError(errno.EEXIST, "destination_conflict")
        raise ctypes.WinError(error)
    fd = msvcrt.open_osfhandle(handle, os.O_RDONLY if directory or read else os.O_WRONLY | os.O_BINARY)
    try:
        info = os.fstat(fd)
        if getattr(info, "st_file_attributes", 0) & 0x400 or not (stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode)):
            raise ValueError("unsafe_destination")
        return fd
    except BaseException:
        os.close(fd)
        raise


@contextmanager
def anchored_parent(root, expected, relative, create):
    root = Path(root)
    parts = parts_for(relative)
    check_root(root, expected)
    with ExitStack() as stack:
        def hold(fd):
            stack.callback(os.close, fd)
            return fd

        if os.name == "nt":
            parent = root
            fd = hold(windows_open(parent, directory=True))
            if (os.fstat(fd).st_dev, os.fstat(fd).st_ino) != expected:
                raise ValueError("device_changed")
            for name in parts[:-1]:
                check_root(root, expected)
                parent = parent / name
                if create:
                    try:
                        parent.mkdir()
                    except FileExistsError:
                        pass
                hold(windows_open(parent, directory=True))
            yield parent / parts[-1], None
        else:
            flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW
            fd = hold(os.open(root, flags))
            if (os.fstat(fd).st_dev, os.fstat(fd).st_ino) != expected:
                raise ValueError("device_changed")
            for name in parts[:-1]:
                check_root(root, expected)
                if create:
                    try:
                        os.mkdir(name, 0o700, dir_fd=fd)
                    except FileExistsError:
                        pass
                fd = hold(os.open(name, flags, dir_fd=fd))
                if os.fstat(fd).st_dev != expected[0]:
                    raise ValueError("device_changed")
            yield parts[-1], fd


def write_volume(root, expected, relative, source):
    with anchored_parent(root, expected, relative, True) as (name, parent):
        check_root(root, expected)
        fd = windows_open(name) if os.name == "nt" else os.open(
            name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=parent)
        try:
            total = 0
            read = getattr(source, "read1", source.read)
            while chunk := read(1024 * 1024):
                check_root(root, expected)
                offset = 0
                while offset < len(chunk):
                    written = os.write(fd, chunk[offset:])
                    if not written:
                        raise OSError("write_failed")
                    offset += written
                total += len(chunk)
            check_root(root, expected)
            os.fsync(fd)
            return total
        finally:
            os.close(fd)


def remove_volume(root, expected, relative):
    with anchored_parent(root, expected, relative, False) as (name, parent):
        check_root(root, expected)
        info = os.lstat(name) if parent is None else os.stat(name, dir_fd=parent, follow_symlinks=False)
        if not stat.S_ISREG(info.st_mode):
            raise ValueError("unsafe_destination")
        if parent is None:
            os.unlink(name)
        else:
            os.unlink(name, dir_fd=parent)


def move_volume(root, expected, source, destination, expected_hash):
    with anchored_parent(root, expected, source, False) as (name, parent), anchored_parent(root, expected, destination, True) as (target, target_parent):
        fd = windows_open(name, read=True) if parent is None else os.open(name, os.O_RDONLY | os.O_NOFOLLOW, dir_fd=parent)
        try:
            before = os.fstat(fd)
            if not stat.S_ISREG(before.st_mode):
                raise ValueError("unsafe_destination")
            digest = hashlib.sha256()
            while chunk := os.read(fd, 1024 * 1024):
                check_root(root, expected)
                digest.update(chunk)
            if digest.hexdigest() != expected_hash:
                raise ValueError("source_changed")
        finally:
            os.close(fd)
        current = os.lstat(name) if parent is None else os.stat(name, dir_fd=parent, follow_symlinks=False)
        if (current.st_dev, current.st_ino, current.st_size, current.st_mtime_ns) != (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns):
            raise ValueError("source_changed")
        check_root(root, expected)
        if os.name == "nt":
            os.rename(name, target)  # Windows rename fails if the destination exists.
        elif sys.platform == "darwin":
            import ctypes
            libc = ctypes.CDLL(None, use_errno=True)
            rename_exclusive = libc.renameatx_np
            rename_exclusive.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
            rename_exclusive.restype = ctypes.c_int
            if rename_exclusive(parent, os.fsencode(name), target_parent, os.fsencode(target), 0x4):
                code = ctypes.get_errno()
                raise OSError(code, os.strerror(code))
            os.fsync(parent)
            os.fsync(target_parent)
        else:
            raise ValueError("platform_not_supported")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=["write", "remove", "move"])
    parser.add_argument("root", type=Path)
    parser.add_argument("device", type=int)
    parser.add_argument("inode", type=int)
    parser.add_argument("relative")
    parser.add_argument("destination", nargs="?")
    parser.add_argument("sha256", nargs="?")
    args = parser.parse_args()
    try:
        expected = (args.device, args.inode)
        if args.operation == "write":
            result = {"bytes": write_volume(args.root, expected, args.relative, sys.stdin.buffer)}
        elif args.operation == "move":
            if not args.destination or not args.sha256 or not re.fullmatch(r"[a-f0-9]{64}", args.sha256):
                raise ValueError("invalid_move")
            move_volume(args.root, expected, args.relative, args.destination, args.sha256)
            result = {"moved": True}
        else:
            remove_volume(args.root, expected, args.relative)
            result = {"removed": True}
        print(json.dumps(result), flush=True)
        return 0
    except (OSError, ValueError) as error:
        code = "destination_conflict" if isinstance(error, FileExistsError) else (
            errno.errorcode.get(error.errno, "volume_io_failed") if isinstance(error, OSError) else str(error))
        print(json.dumps({"error": code}), flush=True)
        return 1


if __name__ == "__main__":
    sys.exit(main())
