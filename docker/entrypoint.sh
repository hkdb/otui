#!/bin/bash

fix_vol_owner() {
  local dir="$1"
  current_uid=$(stat -c '%u' "$dir" 2>/dev/null)
  if [ "$current_uid" != "1000" ]; then
    echo "Fixing ownership of ${dir}..."
    chown 1000:1000 "$dir"
  fi
}

data=/home/otui/.local/share/otui
conf=/home/otui/.config/otui
keys=/home/otui/.ssh

fix_vol_owner "$data"
fix_vol_owner "$conf"
fix_vol_owner "$keys"

exec sudo -u otui -H \
  PATH="/home/otui/.nvm/versions/node/v22.21.1/bin:$PATH" \
  NVM_DIR="/home/otui/.nvm" \
  OTUI_EDITOR="${OTUI_EDITOR}" \
  OTUI_OLLAMA_HOST="${OTUI_OLLAMA_HOST}" \
  OTUI_OLLAMA_MODEL="${OTUI_OLLAMA_MODEL}" \
  OTUI_DATA_DIR="${OTUI_DATA_DIR}" \
  /home/otui/.local/bin/otui
