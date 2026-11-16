#!/usr/bin/env python3
"""Check an image's Node version against published security releases only."""

import hashlib
import json
import re
import sys
import urllib.parse
import urllib.request


INDEX_URL = "https://nodejs.org/dist/index.json"
MAX_INDEX_BYTES = 5 * 1024 * 1024


def version_tuple(value):
    if not isinstance(value, str):
        raise ValueError("Invalid Node version")
    component = r"(0|[1-9][0-9]*)"
    match = re.fullmatch(r"v?" + component + r"\." + component + r"\." + component,
                         value.strip())
    if match is None:
        raise ValueError("Invalid stable Node version")
    return tuple(map(int, match.groups()))


def security_floor(releases, installed):
    installed = version_tuple(installed)
    if not isinstance(releases, list):
        raise ValueError("Invalid Node release index")
    candidates = []
    for release in releases:
        if not isinstance(release, dict):
            raise ValueError("Invalid Node release record")
        value = release.get("version")
        # The official index also contains alpha/RC/nightly entries. They are
        # not published stable security floors, including within this major.
        if isinstance(value, str) and re.fullmatch(
                r"v?[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*",
                value.strip()):
            continue
        version = version_tuple(value)
        if version[0] != installed[0]:
            continue
        if type(release.get("security")) is not bool:
            raise ValueError("Invalid Node security marker")
        if release["security"]:
            candidates.append(version)
    if not candidates:
        raise ValueError("No published security release for this Node line")
    return max(candidates)


def fetch_index():
    with urllib.request.urlopen(INDEX_URL, timeout=30) as response:
        destination = urllib.parse.urlsplit(response.geturl())
        if destination.scheme != "https" or destination.hostname != "nodejs.org":
            raise ValueError("Unexpected Node metadata destination")
        payload = response.read(MAX_INDEX_BYTES + 1)
    if len(payload) > MAX_INDEX_BYTES:
        raise ValueError("Node release index is too large")
    return json.loads(payload), hashlib.sha256(payload).hexdigest()


def main(arguments):
    if len(arguments) != 1:
        print("usage: check-node-security-release.py INSTALLED_NODE_VERSION", file=sys.stderr)
        return 2
    try:
        installed = version_tuple(arguments[0])
        releases, digest = fetch_index()
        required = security_floor(releases, arguments[0])
        passed = installed >= required
        print(json.dumps({
            "installed": "v" + ".".join(map(str, installed)),
            "published_security_floor": "v" + ".".join(map(str, required)),
            "source": INDEX_URL,
            "source_sha256": digest,
            "meets_published_security_floor": passed,
        }, sort_keys=True))
        return 0 if passed else 1
    except (OSError, ValueError, TypeError) as error:
        print("Cannot verify published Node security releases: " + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
