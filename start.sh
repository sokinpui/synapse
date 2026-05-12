#!/bin/bash

CONFIG_FILE=${1:-config.yaml}

echo "Building Synapse Server (Config: $CONFIG_FILE)..."

echo "Tidying Go modules..."
go mod tidy

echo "Building binaries..."
GOWORK=off go build -o bin/server ./cmd/server
echo "Build complete. Binaries are in the 'bin' directory."

if [ -f .env ]; then
  echo "Loading environment variables from .env file..."
  set -a
  source .env
  set +a
fi

# for local deploy, use vpn proxy
export http_proxy=http://127.0.0.1:1087
export https_proxy=http://127.0.0.1:1087
export ALL_PROXY=socks5://127.0.0.1:1080

echo "Starting server..."
$PWD/bin/server -f "$CONFIG_FILE" &
SERVER_PID=$!

cleanup() {
  echo
  echo "Cleaning up server process (PID: $SERVER_PID)..."
  kill $SERVER_PID 2>/dev/null
  wait $SERVER_PID 2>/dev/null
  echo "Cleanup complete."
  exit 0
}

trap cleanup INT TERM

echo "Server is running. Press Ctrl+C to stop."

wait
