"""Inspect packaged bytecode without executing it or exposing private strings."""
import io
import json
import marshal
import os
import sys
import zipfile
from pathlib import Path
from PyInstaller.archive.readers import CArchiveReader

home = str(Path.home()).encode()
checked = 0


def inspect(data, name):
    global checked
    checked += 1
    if home + b"/" in data or home + b"\\" in data:
        raise ValueError("Owner path in packaged member: " + name)
    if data.startswith(b"PK\x03\x04"):
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            for member in archive.infolist():
                if not member.is_dir():
                    inspect(archive.read(member), name + "/" + member.filename)


for path in sys.argv[1:]:
    archive = CArchiveReader(path)
    for name, entry in archive.toc.items():
        if entry[-1] == "z":
            pyz = archive.open_embedded_archive(name)
            for module in pyz.toc:
                data = pyz.extract(module, raw=True)
                if data is not None:
                    inspect(data, module)
        else:
            inspect(archive.extract(name), name)
    for library in Path(path).parent.glob("_internal/*.zip"):
        inspect(library.read_bytes(), library.name)
print(json.dumps({"packaged_members_scanned": checked, "owner_paths_found": 0}))
