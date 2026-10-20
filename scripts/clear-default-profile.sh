#!/bin/bash

#
# Clear the default OTUI profile for testing
#
# Usage: clear-default-profile.sh [-y]
#   -y  delete without asking (including the OTUI SSH keys)
#

YES=false
[[ "$1" == "-y" ]] && YES=true

confirm() {
  $YES && return 0
  read -r -p "$1 [y/N] " answer
  [[ "$answer" == "y" || "$answer" == "Y" ]]
}

PROFILE=(~/.config/otui/settings.toml ~/.local/share/otui)
KEYS=(~/.ssh/otui_ed25519*)

echo "This deletes the default OTUI profile:"
for path in "${PROFILE[@]}"; do
  [[ -e "$path" ]] && echo "  $path"
done

if confirm "Delete the profile?"; then
  rm -f ~/.config/otui/settings.toml
  rm -rf ~/.local/share/otui
  echo "✅ Profile deleted"
fi

# SSH keys can't be recreated: anything encrypted with them is lost
if [[ -e "${KEYS[0]}" ]]; then
  echo
  echo "OTUI SSH keys (credentials encrypted with them can't be recovered):"
  printf '  %s\n' "${KEYS[@]}"
  if confirm "Also delete the OTUI SSH keys?"; then
    rm -f "${KEYS[@]}"
    echo "✅ SSH keys deleted"
  fi
fi
