#!/usr/bin/env python3
"""Validate the structural fields used by the lightweight delivery gate."""

import json
import re
import sys


def object_without_duplicate_keys(pairs):
    document = {}
    for key, value in pairs:
        if key in document:
            raise ValueError("duplicate JSON key")
        document[key] = value
    return document


def main() -> int:
    if len(sys.argv) != 4:
        return 64
    evidence_path, expected_card, expected_commit = sys.argv[1:]
    if not re.fullmatch(r"AUR-[0-9]{3}", expected_card):
        return 64
    if not re.fullmatch(r"[0-9a-f]{40}", expected_commit):
        return 64
    try:
        with open(evidence_path, "r", encoding="utf-8") as stream:
            document = json.load(stream, object_pairs_hook=object_without_duplicate_keys)
    except (OSError, UnicodeDecodeError, ValueError, json.JSONDecodeError):
        return 1
    if not isinstance(document, dict):
        return 1
    if document.get("schema") != "aurum.delivery-record":
        return 1
    if type(document.get("version")) is not int or document.get("version") != 1:
        return 1
    if document.get("card") != expected_card:
        return 1
    if document.get("commit") != expected_commit:
        return 1
    if document.get("review") != "approved" or document.get("validation") != "passed":
        return 1
    validator_run = document.get("validator_run")
    if not isinstance(validator_run, dict):
        return 1
    if type(validator_run.get("exit_code")) is not int or validator_run["exit_code"] != 0:
        return 1
    if not isinstance(validator_run.get("command"), str) or not validator_run["command"]:
        return 1
    if not isinstance(validator_run.get("raw_output"), str):
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
