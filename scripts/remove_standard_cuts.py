#!/usr/bin/env python3
"""Remove standard cuts of episodes that are also in the library as an extended version.

Episodes are identified through the metadata index opfor keeps in the library
(metadata-index.json, written by `opfor sync`), not by guessing from filenames.
Within each arc folder, a standard-cut video is removed only when a video of the
extended version of the same episode sits in that folder too. The NFO, thumbnail
and subtitle files that belong to a removed video's filename go with it, except
the episode's own metadata files, which `opfor` needs to place videos later.
Alternate versions, specials without an extended version, unknown files and
anything outside the arc folders are never touched.

Nothing is deleted without --apply; the default is a dry run that lists what
would be removed.
"""

import argparse
import json
import os
import sys
from collections import defaultdict
from pathlib import Path

VIDEO_EXTENSIONS = {".mkv", ".mp4"}
SUBTITLE_EXTENSIONS = {".srt", ".ass", ".ssa", ".vtt", ".sub", ".idx", ".sup"}
METADATA_SUFFIXES = (".nfo", "-thumb.png", "-thumb.jpg")


def config_dir() -> Path:
    """Mirror Go's os.UserConfigDir, where opfor keeps its config."""
    if sys.platform == "win32":
        return Path(os.environ["APPDATA"])
    if sys.platform == "darwin":
        return Path.home() / "Library" / "Application Support"
    return Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config")


def default_library() -> Path:
    config = config_dir() / "opforjellyfin" / "config.json"
    try:
        target = json.loads(config.read_text(encoding="utf-8")).get("target_dir")
    except (OSError, ValueError) as error:
        raise SystemExit(f"Error: could not read {config} ({error}); pass the library directory") from error
    if not target:
        raise SystemExit("Error: opfor has no library directory set; pass it as an argument")
    return Path(target)


def is_standard(entry: dict) -> bool:
    return entry.get("variant") in ("standard", "", None)


def sidecars(folder: Path, stem: str, keep_metadata: bool) -> list[Path]:
    """Files belonging to a video filename: metadata copies and subtitles."""
    found = []
    for path in folder.iterdir():
        if not path.is_file():
            continue
        name = path.name
        if name in (stem + suffix for suffix in METADATA_SUFFIXES):
            if not keep_metadata:
                found.append(path)
        elif name.startswith(stem + ".") and path.suffix.lower() in SUBTITLE_EXTENSIONS:
            found.append(path)
    return sorted(found)


def plan(library: Path, index: dict) -> list[tuple[Path, Path, list[Path]]]:
    """Return (standard video, extended video kept, sidecars) for each removal."""
    by_filename = index["by_filename"]
    entries = index["entries"]
    removals = []
    folders = sorted({arc["folder"] for arc in index["arcs"].values()})
    for folder_name in folders:
        folder = library / folder_name
        if not folder.is_dir():
            continue
        episodes = defaultdict(lambda: {"standard": [], "extended": []})
        for path in sorted(folder.iterdir()):
            if path.is_symlink() or not path.is_file() or path.suffix.lower() not in VIDEO_EXTENSIONS:
                continue
            entry_id = by_filename.get(path.stem)
            if entry_id is None:
                continue
            entry = entries[entry_id]
            # Like opfor, only count videos in their own arc folder.
            if Path(entry["nfo_path"]).parent.as_posix() != folder_name:
                continue
            key = (entry.get("kind"), entry.get("episode_id"))
            if entry.get("variant") == "extended":
                episodes[key]["extended"].append(path)
            elif is_standard(entry):
                episodes[key]["standard"].append((path, entry))
        for videos in episodes.values():
            extended = [path for path in videos["extended"] if path.stat().st_size > 0]
            if not extended:
                continue  # no usable extended version: keep the standard cut
            for path, entry in videos["standard"]:
                canonical = Path(entry["nfo_path"]).stem == path.stem
                removals.append((path, extended[0], sidecars(folder, path.stem, canonical)))
    return removals


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("library", nargs="?", type=Path,
                        help="opfor library directory (default: the one set in opfor's config)")
    parser.add_argument("--apply", action="store_true", help="delete the files instead of only listing them")
    args = parser.parse_args()

    library = (args.library or default_library()).expanduser()
    index_path = library / "metadata-index.json"
    try:
        index = json.loads(index_path.read_text(encoding="utf-8"))
        removals = plan(library, index)
    except (OSError, ValueError, KeyError) as error:
        print(f"Error: could not read {index_path} ({error}); run 'opfor sync' first", file=sys.stderr)
        return 1

    if not removals:
        print("No standard cuts with an extended version found.")
        return 0

    verb = "Removing" if args.apply else "Would remove"
    freed = 0
    failed = 0
    for video, kept, extra in removals:
        print(f"{verb} {video.relative_to(library)}")
        print(f"  kept extended: {kept.name}")
        for path in [video, *extra]:
            if path is not video:
                print(f"  {'removing' if args.apply else 'would remove'} {path.name}")
            if not args.apply:
                freed += path.stat().st_size
                continue
            try:
                size = path.stat().st_size
                path.unlink()
                freed += size
            except OSError as error:
                failed += 1
                print(f"  could not remove {path.name}: {error}", file=sys.stderr)

    gib = freed / 1024**3
    if args.apply:
        print(f"\nRemoved {len(removals)} standard cut(s), {gib:.2f} GiB.")
    else:
        print(f"\nDry run: {len(removals)} standard cut(s), {gib:.2f} GiB. Run again with --apply to delete them.")
    if failed:
        print(f"{failed} file(s) could not be removed.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
