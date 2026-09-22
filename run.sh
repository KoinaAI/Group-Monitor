#!/usr/bin/env bash
# 群哨 — build and run the backend.
set -euo pipefail
cd "$(dirname "$0")"

# Ports / paths (override via env).
# NOTE: the frontend has been removed. The backend now has NO auth gate of its
# own, so NAP_ADDR defaults to loopback. Do not bind it to a public interface
# without putting an authenticating reverse proxy in front of it.
export NAP_ADDR="${NAP_ADDR:-127.0.0.1:8787}"
export NAP_CONFIG="${NAP_CONFIG:-$PWD/config.json}"

echo "▶ building backend…"
( cd backend && go build -o napnotifier . )

echo "▶ starting backend on $NAP_ADDR (config: $NAP_CONFIG)"
exec ./backend/napnotifier
