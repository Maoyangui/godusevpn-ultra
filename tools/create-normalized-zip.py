"""Create a deterministic-audit-friendly ZIP with portable member names."""
from __future__ import annotations

import argparse
import os
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile, ZipInfo


def stamp(path: Path) -> tuple[int, int, int, int, int, int]:
    value = path.stat().st_mtime
    local = __import__("time").localtime(value)
    return (
        max(1980, local.tm_year),
        local.tm_mon,
        local.tm_mday,
        local.tm_hour,
        local.tm_min,
        local.tm_sec - local.tm_sec % 2,
    )


def entry(name: str, path: Path, directory: bool) -> ZipInfo:
    info = ZipInfo(name, stamp(path))
    info.compress_type = ZIP_DEFLATED
    info.create_system = 0
    info.external_attr = ((0o40775 if directory else 0o100644) << 16) | (0x10 if directory else 0)
    return info


def create(source: Path, destination: Path) -> None:
    source = source.resolve()
    destination.parent.mkdir(parents=True, exist_ok=True)
    root_name = source.name
    with ZipFile(destination, "w", compression=ZIP_DEFLATED, compresslevel=6, allowZip64=True) as archive:
        archive.writestr(entry(root_name + "/", source, True), b"")
        for current, directories, files in os.walk(source):
            current_path = Path(current)
            directories.sort()
            files.sort()
            for name in directories:
                path = current_path / name
                relative = path.relative_to(source).as_posix()
                archive.writestr(entry(f"{root_name}/{relative}/", path, True), b"")
            for name in files:
                path = current_path / name
                relative = path.relative_to(source).as_posix()
                with path.open("rb") as handle:
                    archive.writestr(entry(f"{root_name}/{relative}", path, False), handle.read())


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source")
    parser.add_argument("destination")
    args = parser.parse_args()
    create(Path(args.source), Path(args.destination))


if __name__ == "__main__":
    main()
