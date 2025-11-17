#!/bin/bash

set -e 


echo -e "\n🚀 Compiling..."
go build

size=$(du -sh otui | awk '{print $1}')
echo -e "\n⚓ Size: ${size}"

echo -e "\n💫 COMPLETED 💫\n"
