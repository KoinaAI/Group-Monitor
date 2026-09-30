#!/usr/bin/env python3
"""Bounded, read-only Xunshu client; credentials come from the environment."""

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

MAX_RESPONSE = 2 * 1024 * 1024


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise urllib.error.HTTPError(req.full_url, code, "redirect refused", headers, fp)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("tools", help="Discover tool descriptions and schemas")
    query = commands.add_parser("query", help="Call one read-only tool")
    query.add_argument("name")
    query.add_argument("arguments", nargs="?", default="{}", help="JSON object, or - for stdin")
    args = parser.parse_args()
    key = os.environ.get("XUNSHU_API_KEY", "").strip()
    try:
        base = os.environ.get("XUNSHU_URL", "").strip().rstrip("/")
        parsed = urllib.parse.urlsplit(base)
        if (parsed.scheme not in ("http", "https") or not parsed.hostname
                or parsed.username is not None or parsed.password is not None
                or parsed.query or parsed.fragment):
            raise ValueError("XUNSHU_URL must be an HTTP(S) instance URL without credentials, query or fragment")
        if not key or "\n" in key or "\r" in key:
            raise ValueError("Set XUNSHU_API_KEY to a key from Agent access")
        data = None
        endpoint = "tools"
        if args.command == "query":
            raw = sys.stdin.read(16385) if args.arguments == "-" else args.arguments
            if len(raw.encode("utf-8")) > 16384:
                raise ValueError("Arguments too large")
            params = json.loads(raw)
            if not isinstance(params, dict):
                raise ValueError("Arguments must be a JSON object")
            data = json.dumps({"name": args.name, "arguments": params}).encode("utf-8")
            endpoint = "query"
        request = urllib.request.Request(
            f"{base}/api/agent/v1/{endpoint}", data=data,
            headers={"Authorization": f"Bearer {key}", "Accept": "application/json",
                     "Content-Type": "application/json"},
        )
        with urllib.request.build_opener(NoRedirect()).open(request, timeout=40) as response:
            body = response.read(MAX_RESPONSE + 1)
            if len(body) > MAX_RESPONSE:
                raise ValueError("Response exceeds 2 MiB")
            result = json.loads(body)
        # A compromised endpoint must not cause this helper to echo its credential.
        print(json.dumps(result, ensure_ascii=False, indent=2).replace(key, "[redacted]"))
        return 0
    except urllib.error.HTTPError as error:
        hints = {400: "Check tool arguments and account/group access", 401: "API key missing or revoked",
                 403: "Access denied", 429: "Too many concurrent requests; retry later"}
        message = hints.get(error.code, "Request failed; check the instance or proxy")
        print(f"HTTP {error.code}: {message}", file=sys.stderr)
    except (ValueError, OSError, urllib.error.URLError) as error:
        message = str(error)
        if key:
            message = message.replace(key, "[redacted]")
        print(message, file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
