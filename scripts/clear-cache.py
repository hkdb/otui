#!/usr/bin/env python3

#
# This script clears rendering cache used in debugging markdown rendering related bugs.
#

import json
from pathlib import Path

sessions_dir = Path("~/temp/otui/sessions")
for session_file in sessions_dir.glob("*.json"):
    with open(session_file, 'r') as f:
        data = json.load(f)
    if 'messages' in data:
        for msg in data['messages']:
            if 'rendered' in msg:
                del msg['rendered']
        with open(session_file, 'w') as f:
            json.dump(data, f, indent=2)
print("✅ Cache cleared")

