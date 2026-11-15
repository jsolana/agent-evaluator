#!/usr/bin/env python3
"""Upload the suite, run an evaluation and wait for it. Stdlib only.

Usage: run_eval.py SUITE.yaml   (EVALUATOR_URL, EVAL_LABEL optional)
Exit code: 0 all scenarios passed, 1 failures, 2 evaluation did not finish.
"""
import json
import os
import sys
import time
import urllib.request

BASE = os.environ.get("EVALUATOR_URL", "http://localhost:8080").rstrip("/")


def call(method, path, body=None, ctype="application/json"):
    req = urllib.request.Request(BASE + path, body, {"Content-Type": ctype}, method=method)
    with urllib.request.urlopen(req) as r:
        return json.load(r)


def main(path):
    with open(path, "rb") as f:
        suite = call("POST", "/api/suites", f.read(), "application/yaml")
    ev = call("POST", "/api/evaluations", json.dumps(
        {"suiteId": suite["id"], "label": os.environ.get("EVAL_LABEL", "recipe-agent"), "concurrency": 2}).encode())
    print(f"suite {suite['id']} -> evaluation {ev['id']}", flush=True)
    while True:
        report = call("GET", f"/api/evaluations/{ev['id']}")
        status = report["evaluation"]["status"]
        if status in ("completed", "failed"):
            break
        time.sleep(2)
    if status == "failed":
        print("evaluation failed:", report["evaluation"].get("error"))
        return 2
    for sc in call("GET", f"/api/evaluations/{ev['id']}/scenarios"):
        print(f"  {sc['status']:<16} {sc['key']:<24} {sc['totalTokens']:>5} tok {sc['totalMs']:>6} ms")
    s = report["summary"]
    c = s["scenarios"]
    print(f"passed {c['passed']}/{c['total']}  failed {c['failed']}  errors {c['executionError']}  "
          f"tokens {s['tokens']}  judge tokens {s['judgeTokens']}")
    return 0 if c["passed"] == c["total"] else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1]))
