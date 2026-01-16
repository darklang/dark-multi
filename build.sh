#!/bin/bash
# Build and install multi

set -e

GO=/home/stachu/go-sdk/go/bin/go

# Build
$GO build -o multi .

# Kill any running multi process
pkill -9 multi 2>/dev/null || true
sleep 0.5

# Install
cp multi ~/.local/bin/

echo "Installed multi"
