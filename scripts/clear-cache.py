#!/usr/bin/env python3

#
# Clears the cached markdown rendering of every session (used when debugging
# markdown rendering bugs). OTUI re-renders messages on the next load.
#
# Usage: clear-cache.py [data directory]
#   Default: data_directory from ~/.config/otui/settings.toml,
#   else ~/.local/share/otui
#

import json
import os
import re
import sys
import tempfile
from pathlib import Path


def data_directory():
    if len(sys.argv) > 1:
        return Path(sys.argv[1]).expanduser()
    settings = Path("~/.config/otui/settings.toml").expanduser()
    if settings.exists():
        match = re.search(r'^\s*data_directory\s*=\s*"([^"]*)"', settings.read_text(), re.MULTILINE)
        if match:
            return Path(match.group(1)).expanduser()
    return Path("~/.local/share/otui").expanduser()


sessions_dir = data_directory() / "sessions"
if not sessions_dir.is_dir():
    sys.exit(f"❌ No sessions directory at {sessions_dir}")

cleared = 0
for session_file in sorted(sessions_dir.glob("*.json")):
    with open(session_file) as f:
        data = json.load(f)
    messages = [m for m in data.get("messages") or [] if "rendered" in m]
    if not messages:
        continue
    for msg in messages:
        del msg["rendered"]
    cleared += len(messages)

    # Write atomically (a session file must never be left half-written)
    fd, tmp = tempfile.mkstemp(dir=sessions_dir, prefix=".clear-cache-")
    with os.fdopen(fd, "w") as f:
        json.dump(data, f, indent=2)
    os.chmod(tmp, 0o600)
    os.replace(tmp, session_file)

print(f"✅ Cache cleared: {cleared} messages in {sessions_dir}")
