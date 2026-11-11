#!/usr/bin/env python3
"""Filter govulncheck output so CI passes despite known vulnerabilities that
have no fix.

govulncheck `-format json` emits NDJSON (pretty-printed, one JSON object per
record). We classify each `finding` record:

  * has `fixed_version`  -> a real, fixable vulnerability (must block CI)
  * no `fixed_version`   -> an accepted risk (no fix exists upstream, e.g.
                            GO-2026-4736 GoBGP NEXT_HOP DoS) — reported but
                            does not block.

Exit code 0 when there are no fixable vulnerabilities, 1 otherwise.

Usage: python3 scripts/govulncheck_filter.py <govulncheck-json-output>
"""
import json
import sys


def load_records(path):
    text = open(path).read()
    decoder = json.JSONDecoder()
    records = []
    idx = 0
    n = len(text)
    while idx < n:
        while idx < n and text[idx] in " \n\t\r":
            idx += 1
        if idx >= n:
            break
        try:
            obj, end = decoder.raw_decode(text, idx)
            records.append(obj)
            idx = end
        except json.JSONDecodeError:
            idx += 1
    return records


def main():
    if len(sys.argv) < 2:
        print("usage: govulncheck_filter.py <input.json>", file=sys.stderr)
        return 2

    fixable = {}
    accepted = {}

    for rec in load_records(sys.argv[1]):
        if not isinstance(rec, dict):
            continue
        finding = rec.get("finding")
        if not isinstance(finding, dict):
            continue
        osv = finding.get("osv", "?")
        fixed = finding.get("fixed_version")
        if fixed:
            fixable.setdefault(osv, fixed)
        else:
            accepted.setdefault(osv, None)

    if accepted:
        print("NOTE: accepted risks (no fix available upstream, not blocking):")
        for osv in sorted(accepted):
            print(f"  - {osv}")

    if fixable:
        print("ERROR: fixable vulnerabilities found (blocking):")
        for osv, fixed in sorted(fixable.items()):
            print(f"  - {osv} (fixed in {fixed})")
        return 1

    print("OK: no fixable vulnerabilities found")
    return 0


if __name__ == "__main__":
    sys.exit(main())
