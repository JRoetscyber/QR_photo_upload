#!/usr/bin/env python3
"""
F1-Grade Turbo Benchmark for Jonathan & Julene's Wedding Platform
Measures:
- Brotli / Gzip compression ratio
- ETag 304 Not Modified cache hits
- Sub-millisecond in-memory SQLite stats latency
- Concurrent RSVP ingestion throughput
"""

import sys
import time
import urllib.request
import json
from concurrent.futures import ThreadPoolExecutor

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

BASE_URL = "http://127.0.0.1:5167"

def test_compression_and_etag():
    print("\n" + "="*60)
    print(" 🏎️  1. COMPRESSION & ETAG CACHE BENCHMARK")
    print("="*60)

    # Test without compression header
    req_raw = urllib.request.Request(f"{BASE_URL}/invite")
    with urllib.request.urlopen(req_raw) as resp:
        raw_bytes = len(resp.read())
        etag_val = resp.headers.get("ETag")

    # Test with Gzip compression header
    req_gzip = urllib.request.Request(f"{BASE_URL}/invite", headers={"Accept-Encoding": "gzip"})
    with urllib.request.urlopen(req_gzip) as resp_gzip:
        gzip_bytes = len(resp_gzip.read())
        encoding = resp_gzip.headers.get("Content-Encoding", "none")

    ratio = (1 - (gzip_bytes / raw_bytes)) * 100 if raw_bytes > 0 else 0
    print(f"📄 Uncompressed HTML Size: {raw_bytes / 1024:.2f} KB")
    print(f"⚡ Compressed ({encoding}) Size: {gzip_bytes / 1024:.2f} KB ({ratio:.1f}% bandwidth reduction!)")
    print(f"🏷️  ETag Generated:        {etag_val}")

    # Test ETag 304 Cache validation
    if etag_val:
        req_etag = urllib.request.Request(f"{BASE_URL}/invite", headers={"If-None-Match": etag_val})
        try:
            with urllib.request.urlopen(req_etag) as resp_304:
                status = resp_304.status
        except urllib.error.HTTPError as e:
            status = e.code
        print(f"✨ ETag 304 Cache Revalidation: HTTP {status} (0ms round-trip payload!)")

def benchmark_stats_latency(requests_count=100):
    print("\n" + "="*60)
    print(f" 🏎️  2. IN-MEMORY ATOMIC STATS READ SPEED ({requests_count} REQUESTS)")
    print("="*60)

    latencies = []
    for _ in range(requests_count):
        start = time.perf_counter()
        req = urllib.request.Request(f"{BASE_URL}/api/admin/rsvps/stats", headers={"X-Admin-PIN": "2026"})
        with urllib.request.urlopen(req) as resp:
            resp.read()
        latencies.append((time.perf_counter() - start) * 1000)

    latencies.sort()
    p50 = latencies[int(len(latencies) * 0.50)]
    p95 = latencies[int(len(latencies) * 0.95)]
    p99 = latencies[int(len(latencies) * 0.99)]

    print(f"⚡ Min Latency:    {min(latencies):.2f} ms")
    print(f"⚡ p50 (Median):   {p50:.2f} ms")
    print(f"⚡ p95 Latency:   {p95:.2f} ms")
    print(f"⚡ p99 Latency:   {p99:.2f} ms")
    print(f"⚡ Average:       {sum(latencies)/len(latencies):.2f} ms")

def benchmark_concurrent_rsvps(concurrency=100):
    print("\n" + "="*60)
    print(f" 🏎️  3. CONCURRENT GOROUTINE RSVP INGESTION ({concurrency} CONCURRENT)")
    print("="*60)

    def submit_one(i):
        payload = json.dumps({
            "name": f"Guest Turbo #{i+1}",
            "phone": "082 111 2222",
            "email": f"turbo{i+1}@speed.co.za",
            "attending": True,
            "guest_count": 2,
            "additional_guests": f"Partner #{i+1}",
            "song_request": "Fast Lane 🏎️",
            "message": "F1 speed RSVP!"
        }).encode("utf-8")

        start = time.perf_counter()
        req = urllib.request.Request(f"{BASE_URL}/api/rsvp", data=payload, headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req) as resp:
            resp.read()
        return (time.perf_counter() - start) * 1000

    start_wall = time.perf_counter()
    with ThreadPoolExecutor(max_workers=concurrency) as pool:
        latencies = list(pool.map(submit_one, range(concurrency)))
    total_time = time.perf_counter() - start_wall

    print(f"✅ Ingested {concurrency} RSVPs in: {total_time:.2f} seconds")
    print(f"🚀 Ingestion Throughput:    {concurrency / total_time:.1f} RSVPs/sec")
    print(f"⚡ Average Ingest Latency:  {sum(latencies)/len(latencies):.1f} ms")
    print("="*60 + "\n")

if __name__ == "__main__":
    test_compression_and_etag()
    benchmark_stats_latency(100)
    benchmark_concurrent_rsvps(100)
