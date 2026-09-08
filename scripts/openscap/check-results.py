#!/usr/bin/env python3
"""Gate an OpenSCAP XCCDF results.xml against a list of accepted findings.

Exits 0 if every "fail" result corresponds to an accepted finding (by rule
id substring match), exits 1 and lists the offending rule ids otherwise.
"""
import re
import sys

RESULT_RE = re.compile(
    r'<[^:>]*:?rule-result idref="([^"]+)"[^>]*>.*?<[^:>]*:?result>([a-z]+)</[^:>]*:?result>',
    re.S,
)


def load_accepted(path):
    accepted = set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.split("#", 1)[0].strip()
            if line:
                accepted.add(line)
    return accepted


def load_rule_results(path):
    with open(path, encoding="utf-8") as f:
        content = f.read()
    return RESULT_RE.findall(content)


def main(argv):
    if len(argv) != 3:
        print(f"usage: {argv[0]} <results.xml> <accepted-findings.txt>", file=sys.stderr)
        return 1

    results_path, accepted_path = argv[1], argv[2]
    accepted = load_accepted(accepted_path)
    rules = load_rule_results(results_path)

    failed = [rule_id for rule_id, result in rules if result == "fail"]
    unexpected = [rid for rid in failed if not any(a in rid for a in accepted)]

    print(f"Total rules evaluated: {len(rules)}")
    print(f"Failed: {len(failed)} (accepted: {len(failed) - len(unexpected)}, unexpected: {len(unexpected)})")

    if unexpected:
        print("\nUnexpected OpenSCAP findings (not in accepted-findings.txt):", file=sys.stderr)
        for rid in unexpected:
            print(f"  - {rid}", file=sys.stderr)
        return 1

    print("\nAll findings are documented and accepted. Scan passed.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
