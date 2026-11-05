#!/usr/bin/env python3
"""Bounded, dependency-free load smoke test for an already running stack."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import statistics
import time
import urllib.error
import urllib.request


def request_json(url: str, method: str = "GET", token: str = "", body: dict | None = None) -> tuple[int, object]:
    payload = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(url, data=payload, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return response.status, json.loads(response.read() or b"{}")
    except urllib.error.HTTPError as error:
        return error.code, json.loads(error.read() or b"{}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.getenv("LOAD_BASE_URL", "http://127.0.0.1:18088"))
    parser.add_argument("--requests", type=int, default=100)
    parser.add_argument("--concurrency", type=int, default=10)
    parser.add_argument("--p95-ms", type=float, default=1500)
    args = parser.parse_args()
    if not 1 <= args.requests <= 10_000 or not 1 <= args.concurrency <= 100:
        parser.error("requests must be 1-10000 and concurrency must be 1-100")

    username = os.getenv("LOAD_USERNAME", "teacher")
    password = os.getenv("LOAD_PASSWORD", "demo123")
    status, login = request_json(
        f"{args.base_url}/api/v1/auth/login",
        "POST",
        body={"username": username, "password": password},
    )
    if status != 200 or not isinstance(login, dict) or not login.get("token"):
        print(json.dumps({"ok": False, "stage": "login", "status": status}, ensure_ascii=False))
        return 1
    token = str(login["token"])
    class_id = str(login.get("user", {}).get("classId", ""))

    def one(index: int) -> tuple[int, float]:
        path = "/health" if index % 5 == 0 else f"/api/v1/classes/{class_id}/materials"
        started = time.perf_counter()
        response_status, _ = request_json(f"{args.base_url}{path}", token=token)
        return response_status, (time.perf_counter() - started) * 1000

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        results = list(executor.map(one, range(args.requests)))
    latencies = sorted(latency for _, latency in results)
    failures = sum(1 for status_code, _ in results if status_code != 200)
    p95_index = max(0, min(len(latencies) - 1, round(len(latencies) * 0.95) - 1))
    report = {
        "ok": failures == 0 and latencies[p95_index] <= args.p95_ms,
        "requests": args.requests,
        "concurrency": args.concurrency,
        "failures": failures,
        "latencyMs": {
            "mean": round(statistics.fmean(latencies), 2),
            "p95": round(latencies[p95_index], 2),
            "max": round(max(latencies), 2),
        },
        "p95BudgetMs": args.p95_ms,
    }
    print(json.dumps(report, ensure_ascii=False))
    return 0 if report["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
