#!/usr/bin/env python3
"""
Wedding Server High-Concurrency Load Test
Simulates N simultaneous guests uploading photos at the exact same moment.
Measures latency, throughput, success rate, and verifies server resilience.
"""

import sys
import io
import time
import argparse
import urllib.request
import urllib.parse
import json

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

from concurrent.futures import ThreadPoolExecutor, as_completed
from PIL import Image

SERVER_URL = "http://127.0.0.1:8080"
UPLOAD_URL = f"{SERVER_URL}/api/upload"

GUEST_NAMES = [
    "David & Sarah", "Jessica Miller", "Uncle Bob", "Aunt Clara", "Emily & James",
    "Liam Johnson", "Noah & Emma", "Olivia Davis", "Sophia & Jackson", "Lucas Walker",
    "Mason & Mia", "Ethan Hall", "Harper Allen", "Evelyn Young", "Alexander King",
    "Benjamin Wright", "Charlotte Scott", "Daniel Green", "Henry Adams", "Ella Baker",
    "Chloe Nelson", "Sebastian Carter", "Grace Mitchell", "Jack Perez", "Victoria Roberts",
    "Logan Turner", "Hannah Phillips", "Samuel Campbell", "Lily Parker", "Ryan Evans",
    "Zoey Edwards", "Nathan Collins", "Penelope Stewart", "Isaac Sanchez", "Riley Morris",
    "Andrew Rogers", "Nora Reed", "Joshua Cook", "Hazel Morgan", "Christopher Bell",
    "Stella Murphy", "Julian Bailey", "Ellie Rivera", "Caleb Cooper", "Paisley Richardson",
    "Hunter Cox", "Audrey Howard", "Christian Ward", "Skylar Torres", "Aaron Peterson",
    "Lucy Gray", "Eli Ramirez", "Anna James", "Landon Watson", "Samantha Brooks",
    "Jonathan Kelly", "Elena Sanders", "Nolan Price", "Maya Bennett", "Wyatt Wood"
]

WISHES = [
    "Congratulations to the wonderful couple! 🎉",
    "Wishing you a lifetime of love and laughter! 💕",
    "What an incredible night! Best party ever! 🥂",
    "May your love grow stronger each passing day.",
    "Cheers to the newlyweds! 🍾",
    "So honored to celebrate with you both! ✨",
    "Dancing the night away! Incredible wedding!",
    "Love you guys so much! Beautiful ceremony.",
    "Such a romantic evening! Enjoy forever together.",
    "Here's to the new Mr. & Mrs.! 🥂"
]

def create_synthetic_image(user_id):
    """Creates a sample JPEG image in memory with distinct colors."""
    img = Image.new("RGB", (800, 600), color=(
        (user_id * 37) % 255,
        (user_id * 73) % 255,
        (user_id * 109) % 255
    ))
    buf = io.BytesIO()
    img.save(buf, format="JPEG", quality=85)
    return buf.getvalue()

def upload_worker(user_idx):
    guest = GUEST_NAMES[user_idx % len(GUEST_NAMES)]
    wish = WISHES[user_idx % len(WISHES)]
    img_data = create_synthetic_image(user_idx)
    filename = f"photo_guest_{user_idx+1}.jpg"

    boundary = f"----WebKitFormBoundary{int(time.time() * 1000)}_{user_idx}"
    
    # Build multipart/form-data payload
    body = bytearray()
    
    # Field: guest_name
    body.extend(f"--{boundary}\r\n".encode("utf-8"))
    body.extend(f'Content-Disposition: form-data; name="guest_name"\r\n\r\n'.encode("utf-8"))
    body.extend(f"{guest} #{user_idx+1}\r\n".encode("utf-8"))
    
    # Field: wish
    body.extend(f"--{boundary}\r\n".encode("utf-8"))
    body.extend(f'Content-Disposition: form-data; name="wish"\r\n\r\n'.encode("utf-8"))
    body.extend(f"{wish}\r\n".encode("utf-8"))
    
    # Field: photos
    body.extend(f"--{boundary}\r\n".encode("utf-8"))
    body.extend(f'Content-Disposition: form-data; name="photos"; filename="{filename}"\r\n'.encode("utf-8"))
    body.extend(b"Content-Type: image/jpeg\r\n\r\n")
    body.extend(img_data)
    body.extend(b"\r\n")
    
    # End boundary
    body.extend(f"--{boundary}--\r\n".encode("utf-8"))

    req = urllib.request.Request(
        UPLOAD_URL,
        data=bytes(body),
        headers={
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "User-Agent": f"WeddingLoadTester/1.0 (Guest-{user_idx+1})"
        },
        method="POST"
    )

    start_time = time.time()
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            elapsed = time.time() - start_time
            status = resp.status
            resp.read()
            return {
                "user_id": user_idx + 1,
                "guest": guest,
                "status": status,
                "latency_ms": int(elapsed * 1000),
                "success": status in (200, 201),
                "error": None
            }
    except Exception as e:
        elapsed = time.time() - start_time
        return {
            "user_id": user_idx + 1,
            "guest": guest,
            "status": 0,
            "latency_ms": int(elapsed * 1000),
            "success": False,
            "error": str(e)
        }

def run_test(concurrent_users=300):
    print(f"🚀 Starting High-Concurrency Stress Test: {concurrent_users} simultaneous photo uploads...")
    print(f"🎯 Target Endpoint: {UPLOAD_URL}")

    start_wall = time.time()
    results = []

    with ThreadPoolExecutor(max_workers=concurrent_users) as executor:
        futures = [executor.submit(upload_worker, i) for i in range(concurrent_users)]
        for future in as_completed(futures):
            results.append(future.result())

    total_time = time.time() - start_wall
    successes = [r for r in results if r["success"]]
    failures = [r for r in results if not r["success"]]
    latencies = sorted([r["latency_ms"] for r in results])

    p50 = latencies[int(len(latencies) * 0.50)] if latencies else 0
    p95 = latencies[int(len(latencies) * 0.95)] if latencies else 0
    p99 = latencies[int(len(latencies) * 0.99)] if latencies else 0

    print("\n" + "="*58)
    print(f" 📊 STRESS TEST RESULTS ({concurrent_users} CONCURRENT UPLOADS)")
    print("="*58)
    print(f"Total Requests Dispatched:  {len(results)}")
    print(f"Successful Uploads:         {len(successes)} / {len(results)} ({len(successes)/len(results)*100:.1f}%)")
    print(f"Failed Uploads:             {len(failures)}")
    print(f"Total Duration:             {total_time:.2f} seconds")
    print(f"Throughput:                 {len(results)/total_time:.2f} requests/sec")
    if latencies:
        print(f"Min Latency:                {min(latencies)} ms")
        print(f"p50 (Median) Latency:       {p50} ms")
        print(f"p95 Latency:                {p95} ms")
        print(f"p99 Latency:                {p99} ms")
        print(f"Max Latency:                {max(latencies)} ms")
        print(f"Average Latency:            {sum(latencies)/len(latencies):.1f} ms")
    print("="*58)

    if failures:
        print("\n⚠️ Failure Details:")
        for f in failures[:5]:
            print(f"  Guest {f['user_id']} ({f['guest']}): {f['error']}")

    # Check stats endpoint
    try:
        with urllib.request.urlopen(f"{SERVER_URL}/api/stats") as resp:
            stats = json.loads(resp.read().decode())
            print(f"\n📦 Verified Server Stats:")
            print(f"  Total Photos in Disk Store: {stats.get('total_photos')}")
            print(f"  Total Unique Guests:        {stats.get('total_guests')}")
            print(f"  Total Storage Used:         {stats.get('total_bytes') / (1024*1024):.2f} MB")
    except Exception as e:
        print(f"Could not query stats: {e}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Wedding Server Load Test")
    parser.add_argument("--count", type=int, default=300, help="Number of concurrent uploads (default: 300)")
    args = parser.parse_args()
    run_test(args.count)
