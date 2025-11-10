#!/bin/bash

#
# Edit versions in all the required places for release
#

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Check if running from repo root
if [[ ! -f "main.go" ]] || [[ ! -d "scripts" ]] || [[ ! -d "dist" ]]; then
    echo -e "\n${RED}Error: This script must be run from the repository root directory.${NC}"
    echo -e "\nUsage:"
    echo -e "  ${GREEN}./scripts/release.sh v0.02.00${NC}\n"
    exit 1
fi

if [[ "$1" != "v"*  && "$1" != "help" ]]; then
    echo -e "\n${RED}Unrecognized argument.${NC}  release.sh only accepts a release version number starting with \"v\" or \"help\" as argument... Try ./release.sh help to learn more...\n"
    exit 1
fi

if [[ $1 == "help" ]]; then
  echo -e "\nOTUI automation script: ${GREEN}release.sh${NC}\n"
  echo -e "Context: This script must be run from the root of the repository.\n"
  echo -e "\t${GREEN}release.sh${NC} arguments:\n"
  echo -e "\t\t- v\[major\].\[minor\].\[test\] ~ ex. v0.01.00\n\t\t- help ~ current output\n\n"
  exit 0
fi

# Change version in main.go to $1

sed -i "s/Version = \"v[0-9]\+\.[0-9]\+\.[0-9]\+\"/Version = \"$1\"/" main.go
echo -e "✅ Updated main.go to $1"

# Change version in dist/get.sh to $1

sed -i "s/VER=\"v[0-9]\+\.[0-9]\+\.[0-9]\+\"/VER=\"$1\"/" dist/get.sh
echo -e "✅ Updated dist/get.sh to $1"

# Change version in dist/index.html to $1

sed -i "s/<div class=\"version\">v[0-9]\+\.[0-9]\+\.[0-9]\+<\/div>/<div class=\"version\">$1<\/div>/" dist/index.html
echo -e "✅ Updated dist/index.html to $1"

# Change version in dist/version.txt to $1

echo "$1" > dist/version.txt
echo -e "✅ Updated dist/version.txt to $1"
