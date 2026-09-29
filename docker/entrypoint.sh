#!/bin/sh
set -eu

config_dir=$(dirname "${NAP_CONFIG:-/data/config.json}")
mkdir -p "$config_dir"

/usr/local/bin/napnotifier &
backend_pid=$!

nginx -g 'daemon off;' &
nginx_pid=$!

cleanup() {
    kill "$backend_pid" "$nginx_pid" 2>/dev/null || true
}
trap cleanup TERM INT EXIT

while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$nginx_pid" 2>/dev/null; do
    sleep 1
done

exit 1
