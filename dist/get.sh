#!/usr/bin/env bash

#################
# otui installer #
#################

set -e

VER="v0.09.00"
CYAN='\033[0;36m'
GREEN='\033[1;32m'
NC='\033[0m' 

echo -e "\n📦️ Installing:"

echo -e "${CYAN}
 ░▒▓██████▓▒░▒▓████████▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
 ░▒▓██████▓▒░  ░▒▓█▓▒░    ░▒▓██████▓▒░░▒▓█▓▒░ 
                                                                                         
${NC}"

echo -e "🚀️ ${GREEN}An Opinionated, Minimalist Agentic TUI${NC}\n"

USEROS=""
echo -e "🐧️ Detecting OS..."
if [[ "$OSTYPE" == "linux"* ]]; then
  USEROS="linux"
  echo -e "\n🐧️ Linux\n"
elif [[ "$OSTYPE" == "freebsd"* ]]; then
  USEROS="freebsd"
  echo -e "\n🅱️  FreeBSD\n"
elif [[ "$OSTYPE" == "darwin"* ]]; then
  USEROS="darwin"
  echo -e "\n🍎️ MacOS"
else
  echo -e "❌️ Operating System not supported... Exiting...\n"
  exit 1
fi

echo -e "💻️ Detecting CPU arch...\n"

CPUARCH=""
UNAMEM=$(uname -m)
echo -e "🏰️: $UNAMEM\n"

if [[ "$UNAMEM" == "x86_64" ]] || [[ "$UNAMEM" == "amd64" ]]; then
  CPUARCH="amd64"
elif [[ "$UNAMEM" == "arm64" ]]; then
  CPUARCH="arm64"
elif [[ "$UNAMEM" == "aarch64" ]]; then
  CPUARCH="arm64"
else
  echo -e "❌️ CPU Architecture not supported... Exiting...\n"
  exit 1
fi

echo -e "✅️ Dependencies check...\n"
if ! command -v curl >/dev/null 2>&1; then
  echo -e "\n❌️ curl is not installed on this system. Install it and run the install command again...\n"
  exit 1
fi

# SHA-256 of a file (Linux: sha256sum, macOS: shasum, FreeBSD: sha256)
# (type -P: programs only)
file_sha256() {
  if type -P sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif type -P shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif type -P sha256 >/dev/null 2>&1; then
    sha256 -q "$1"
  else
    return 1
  fi
}
if ! file_sha256 /dev/null >/dev/null 2>&1; then
  echo -e "\n❌️ No SHA-256 tool found (sha256sum, shasum or sha256) to verify the download... Exiting...\n"
  exit 1
fi

echo -e "✅️ Detecting shell...\n"

SHELLTYPE=$(basename ${SHELL})

echo -e "🐚️ shell: $SHELLTYPE"

SHELLRC="none"
SHELLPROFILE="$HOME/.config/otui/.otui_profile"

if [[ $SHELLTYPE == "sh" ]]; then
  SHELLRC="$HOME/.shrc"
fi

if [[ $SHELLTYPE == "csh" ]]; then
  SHELLRC="$HOME/.cshrc"
fi

if [[ $SHELLTYPE == "ksh" ]]; then
  SHELLRC="$HOME/.kshrc"
fi

if [[ $SHELLTYPE == "tcsh" ]]; then
  SHELLRC="$HOME/.tcshrc"
fi

if [[ $SHELLTYPE == "bash" ]]; then
  SHELLRC="$HOME/.bashrc"
fi

if [[ $SHELLTYPE == "zsh" ]]; then
  SHELLRC="$HOME/.zshrc"
fi

if [[ $SHELLTYPE == "fish" ]]; then
  SHELLRC="$HOME/.config/fish/config.fish"
fi

if [[ $SHELLRC == "none" ]]; then
  echo -e "\n❌️ Unrecognized shell... otui only supports sh, csh, ksh, tcsh, bash, zsh, and fish... exiting...\n"
  exit 1
fi

echo -e "🐚️ config: $SHELLRC\n"

echo -e "✅️ Create otui config dir if not already created...\n"
if [[ ! -d "$HOME/.config/otui" ]]; then
  mkdir -p $HOME/.config/otui
  if [[ $? -ne 0 ]] ; then
      echo -e "\n❌️ Failed to create $HOME/.config/otui... Exiting...\n"
      exit 1
  fi
fi

echo -e "✅️ Making sure there's a $HOME/.local/bin...\n"
if [[ ! -d "$HOME/.local/bin" ]]; then
  mkdir -p $HOME/.local/bin
  if [[ $? -ne 0 ]] ; then
      echo -e "\n❌️ Failed to create $HOME/.local/bin... Exiting...\n"
      exit 1
  fi
fi

echo -e "✅️ Making sure $HOME/.local/bin is in PATH...\n"
if [[ -f $SHELLPROFILE ]]; then
  PCHECK=$(grep ".local/bin" $SHELLPROFILE)
  if [[ "$PCHECK" == "" ]]; then
    echo -e "\nif [ -d \"$HOME/.local/bin\" ]; then\n\tPATH=\"$HOME/.local/bin:\$PATH\"\nfi" >> $SHELLPROFILE
    echo -e "\n# Added by otui (https://github.com/hkdb/otui) installation\nsource $SHELLPROFILE" >> $SHELLRC
  fi
else
    if [[ $SHELLTYPE == "fish" ]]; then
      echo -e "if test -d \"$HOME/.local/bin\"\n   set -U fish_user_paths $HOME/.local/bin \$PATH\nend" >> $SHELLPROFILE
    else
      echo -e "\nif [ -d \"$HOME/.local/bin\" ]; then\n\tPATH=\"$HOME/.local/bin:\$PATH\"\nfi" >> $SHELLPROFILE
    fi
    echo -e "\n# Added by otui (https://github.com/hkdb/otui) installation\nsource $SHELLPROFILE" >> $SHELLRC
fi

echo -e "⏳️ Downloading otui binary...\n"

# Release downloads (OTUI_DOWNLOAD_BASE overrides the location for testing)
BINARY="otui-$USEROS-$CPUARCH"
BASE="${OTUI_DOWNLOAD_BASE:-https://github.com/hkdb/otui/releases/download}"
CURL_OPTS=(-fsSL --retry 3)
if [[ -z "$OTUI_DOWNLOAD_BASE" ]]; then
  CURL_OPTS+=(--proto '=https' --tlsv1.2)
fi

# Next to the install location, so the final mv is an atomic rename
TMPDIR_OTUI="$(mktemp -d "$HOME/.local/bin/.otui-download.XXXXXX")"
trap 'rm -rf "$TMPDIR_OTUI"' EXIT

# -f: an HTTP error (404, rate limit...) fails instead of saving the error page
if ! curl "${CURL_OPTS[@]}" -o "$TMPDIR_OTUI/$BINARY" "$BASE/$VER/$BINARY"; then
    echo -e "\n❌️ Failed to download $BINARY ($VER)... Exiting...\n"
    exit 1
fi
if ! curl "${CURL_OPTS[@]}" -o "$TMPDIR_OTUI/SHA256SUMS" "$BASE/$VER/SHA256SUMS"; then
    echo -e "\n❌️ Failed to download the checksums (SHA256SUMS) to verify $BINARY... Exiting...\n"
    exit 1
fi

echo -e "🔐️ Verifying checksum...\n"
EXPECTED="$(awk -v f="$BINARY" '$2 == f || $2 == "*" f {print $1}' "$TMPDIR_OTUI/SHA256SUMS")"
ACTUAL="$(file_sha256 "$TMPDIR_OTUI/$BINARY")"
if [[ -z "$EXPECTED" ]]; then
    echo -e "\n❌️ SHA256SUMS has no checksum for $BINARY... Exiting...\n"
    exit 1
fi
if [[ "$EXPECTED" != "$ACTUAL" ]]; then
    echo -e "\n❌️ Checksum mismatch for $BINARY: the download is corrupted or was tampered with. Nothing was installed... Exiting...\n"
    exit 1
fi

echo -e " Installing otui binary...\n"
chmod +x "$TMPDIR_OTUI/$BINARY"
# Replace the binary in one step: an existing install is never left half-written
if ! mv -f "$TMPDIR_OTUI/$BINARY" "$HOME/.local/bin/otui"; then
    echo -e "\n❌️ Failed to install otui to $HOME/.local/bin... Exiting...\n"
    exit 1
fi

echo -e "\n${GREEN}**************"
echo -e " 💯️ COMPLETED"
echo -e "**************${NC}\n"

echo -e "⚠️  You may need to close and reopen your existing terminal windows for otui to work as expected...\n"

