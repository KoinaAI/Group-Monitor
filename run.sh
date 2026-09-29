#!/usr/bin/env bash
# 讯枢 — build and run the backend.
set -euo pipefail
cd "$(dirname "$0")"

# Ports / paths (override via env).
# The backend now owns authentication (OTP to masters + optional break-glass
# password → session cookie), but NAP_ADDR still defaults to loopback to keep
# the surface local. If you expose it, keep TLS (a terminating reverse proxy)
# in front so the session cookie is sent Secure.
#   NAP_PASSWORD       break-glass password, usable only when OTP can't be sent
#                      (NapCat offline or no masters). Unset ⇒ password login off.
#   NAP_SESSION_HOURS  session lifetime in hours (default 12).
export NAP_ADDR="${NAP_ADDR:-127.0.0.1:8787}"
export NAP_CONFIG="${NAP_CONFIG:-$PWD/config.json}"

echo "▶ building backend…"
( cd backend && go build -o napnotifier . )

echo "▶ starting backend on $NAP_ADDR (config: $NAP_CONFIG)"
exec ./backend/napnotifier
