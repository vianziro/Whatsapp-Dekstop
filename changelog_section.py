#!/usr/bin/env python3
"""Extracts a single version's section from CHANGELOG.md.

Release notes on GitHub are generated from CHANGELOG.md so that the published
release and the repository can never disagree about what changed. release.sh
calls this before publishing.

    python3 changelog_section.py 1.5.9.7 --notes-file /tmp/notes.md

Writes the version's section body to --notes-file and prints the version's
tagline (the leading "> ..." line, if any) to stdout, which release.sh appends
to the release title.

Exits non-zero when the version has no section. Publishing a release with
placeholder notes is worse than not publishing it, so release.sh treats a
missing section as fatal.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

CHANGELOG = Path(__file__).resolve().parent / "CHANGELOG.md"
HEADING = re.compile(r"^##\s+\[([^\]]+)\]")


def extract(text: str, version: str) -> tuple[str, str] | None:
    """Return (tagline, body) for `version`, or None when absent."""
    lines = text.splitlines()

    # Indices of every "## [...]" heading, in document order. The bracketed
    # version is compared in full, so 1.5.9 can never match 1.5.9.7.
    headings = [i for i, line in enumerate(lines) if HEADING.match(line)]

    for position, index in enumerate(headings):
        if HEADING.match(lines[index]).group(1).strip() != version:
            continue
        after = headings[position + 1] if position + 1 < len(headings) else len(lines)
        block = lines[index + 1:after]
        break
    else:
        return None

    # Drop the section separators and surrounding blank lines.
    while block and block[-1].strip() in ("", "---"):
        block.pop()
    while block and block[0].strip() == "":
        block.pop(0)

    # An optional blockquote right under the heading becomes the tagline used
    # in the release title.
    tagline = ""
    if block and block[0].startswith(">"):
        tagline = block[0].lstrip(">").strip()
        block.pop(0)
        while block and block[0].strip() == "":
            block.pop(0)

    if not block:
        return None
    return tagline, "\n".join(block).rstrip() + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="version without the leading v, e.g. 1.5.9.7")
    parser.add_argument("--notes-file", help="write the section body here")
    args = parser.parse_args()

    if not CHANGELOG.is_file():
        print(f"error: {CHANGELOG} not found", file=sys.stderr)
        return 1

    result = extract(CHANGELOG.read_text(encoding="utf-8"), args.version)
    if result is None:
        print(
            f"error: CHANGELOG.md has no '## [{args.version}]' section.\n"
            f"Add one before releasing — the published notes are generated from it.",
            file=sys.stderr,
        )
        return 1

    tagline, body = result
    if args.notes_file:
        Path(args.notes_file).write_text(body, encoding="utf-8")
    print(tagline)
    return 0


if __name__ == "__main__":
    sys.exit(main())
