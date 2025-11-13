#!/bin/bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "
${GREEN}
*********************************************

 ░▒▓██████▓▒░▒▓████████▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
░▒▓█▓▒░░▒▓█▓▒░ ░▒▓█▓▒░   ░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░ 
 ░▒▓██████▓▒░  ░▒▓█▓▒░    ░▒▓██████▓▒░░▒▓█▓▒░ 
                                                                                            
********** Container Build Script ***********
${NC}
"

if [[ "$1" == "" ]]; then
  echo -e "\n${RED}ERROR:${NC}You must specify a build version to run this script. Try \"./build.sh --help\" to learn more\n"
  exit 1
fi

if [[ $1 == "--help" ]]; then
  echo -e "\n${GREEN}OTUI${NC} production containers build script.\n"
  echo -e "\tBuild arguments:\n"
  echo -e "\t\t- Build Version (ie. v0.01.00)\n"
  exit 0
fi

read -p "Build Docker image? (Y/n) " bconfirm
if [[ "$bconfirm" == "" ]]; then
  bconfirm="Y"
fi
if [[ "$bconfirm" == "Y" ]] || [[ "$bconfirm" == "y" ]]; then
  echo ""
  echo ""
  echo -e "\n📦 Build Docker Image...\n"
  docker build --no-cache -t ghcr.io/hkdb/otui:$1 .
fi
echo ""
echo ""
read -p "Do you want to push this image to the registry? (Y/n) " ans
if [[ "$ans" == "" ]]; then
  ans="Y"
fi
if [[ "$ans" == "Y" ]] || [[ "$ans" == "y" ]]; then
  echo -e "\n🚀 Push...\n"
  docker push ghcr.io/hkdb/otui:$1
else
  exit 0
fi
echo ""
echo ""
read -p "Tag image as latest? (Y/n) " tconfirm
if [[ "$tconfirm" == "" ]]; then
  tconfirm="Y"
fi
if [[ "$tconfirm" == "Y" ]] || [[ "$tconfirm" == "y" ]]; then
  echo -e "\n🏷️ Tagging...\n"
  docker tag ghcr.io/hkdb/otui:$1 ghcr.io/hkdb/otui:latest
else
  exit 0
fi
echo ""
echo ""
read -p "Do you want to push this image to the registry? (Y/n) " lconfirm
if [[ "$lconfirm" == "" ]]; then
  lconfirm="Y"
fi
if [[ "$lconfirm" == "Y" ]] || [[ "$lconfirm" == "y" ]]; then
  echo -e "\n🚀 Push...\n"
  docker push ghcr.io/hkdb/otui:latest
fi

echo -e "\n💫 COMPLETED 💫\n"
