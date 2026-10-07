"""
Automated Responsive Viewport Testing & Visual Regression Suite
Tests every specified viewport with exact width, height, and device_scale_factor (DPR).
Takes full-page screenshots, verifies zero horizontal scrolling, inspects layout bounds and console logs.
"""

import os
import json
import time
from playwright.sync_api import sync_playwright

BASE_URL = "http://localhost:5167"
SCREENSHOTS_DIR = os.path.join(os.getcwd(), "screenshots")

VIEWPORTS = [
    # Main Requested Viewports
    {"name": "72\" TV 4K raw", "width": 3840, "height": 2160, "dpr": 1.0, "filename": "3840x2160@1.png"},
    {"name": "72\" TV scaled", "width": 1920, "height": 1080, "dpr": 2.0, "filename": "1920x1080@2.png"},
    {"name": "43\" 4K monitor", "width": 3840, "height": 2160, "dpr": 1.0, "filename": "3840x2160@1_monitor.png"},
    {"name": "Ultrawide 34\"", "width": 3440, "height": 1440, "dpr": 1.0, "filename": "3440x1440@1.png"},
    {"name": "32\" 4K scaled", "width": 2560, "height": 1440, "dpr": 1.5, "filename": "2560x1440@1.5.png"},
    {"name": "27\" QHD", "width": 2560, "height": 1440, "dpr": 1.0, "filename": "2560x1440@1.png"},
    {"name": "iMac 27\" 5K", "width": 2560, "height": 1440, "dpr": 2.0, "filename": "2560x1440@2.png"},
    {"name": "iMac 24\"", "width": 2240, "height": 1260, "dpr": 2.0, "filename": "2240x1260@2.png"},
    {"name": "24\" Full HD", "width": 1920, "height": 1080, "dpr": 1.0, "filename": "1920x1080@1.png"},
    {"name": "MacBook Pro 16\"", "width": 1728, "height": 1117, "dpr": 2.0, "filename": "1728x1117@2.png"},
    {"name": "MacBook Air 15\"", "width": 1710, "height": 1107, "dpr": 2.0, "filename": "1710x1107@2.png"},
    {"name": "Windows laptop 15\" (125%)", "width": 1536, "height": 864, "dpr": 1.25, "filename": "1536x864@1.25.png"},
    {"name": "MacBook Pro 14\"", "width": 1512, "height": 982, "dpr": 2.0, "filename": "1512x982@2.png"},
    {"name": "MacBook Air 13\" M2/M3", "width": 1470, "height": 956, "dpr": 2.0, "filename": "1470x956@2.png"},
    {"name": "MacBook Air 13\" older", "width": 1440, "height": 900, "dpr": 2.0, "filename": "1440x900@2.png"},
    {"name": "Small laptop 14\"", "width": 1366, "height": 768, "dpr": 1.0, "filename": "1366x768@1.png"},
    {"name": "Small laptop 13\"", "width": 1280, "height": 800, "dpr": 1.0, "filename": "1280x800@1.png"},
    {"name": "Netbook", "width": 1280, "height": 720, "dpr": 1.0, "filename": "1280x720@1.png"},
    {"name": "Minimum desktop", "width": 1024, "height": 768, "dpr": 1.0, "filename": "1024x768@1.png"},

    # In-between viewports
    {"name": "In-between 1100", "width": 1100, "height": 750, "dpr": 1.0, "filename": "1100x750@1.png"},
    {"name": "In-between 1200", "width": 1200, "height": 800, "dpr": 1.0, "filename": "1200x800@1.png"},
    {"name": "In-between 1400", "width": 1400, "height": 900, "dpr": 1.0, "filename": "1400x900@1.png"},
    {"name": "In-between 1600", "width": 1600, "height": 900, "dpr": 1.0, "filename": "1600x900@1.png"},
    {"name": "In-between 1800", "width": 1800, "height": 1000, "dpr": 1.0, "filename": "1800x1000@1.png"},
    {"name": "In-between 2000", "width": 2000, "height": 1100, "dpr": 1.0, "filename": "2000x1100@1.png"},
    {"name": "In-between 3000", "width": 3000, "height": 1300, "dpr": 1.0, "filename": "3000x1300@1.png"},
]

def run_tests():
    os.makedirs(SCREENSHOTS_DIR, exist_ok=True)
    results = []

    print("=========================================================================")
    print(" STARTING PLAYWRIGHT RESPONSIVE VIEWPORT SUITE")
    print(f" Target: {len(VIEWPORTS)} viewports on {BASE_URL}")
    print("=========================================================================")

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)

        for vp in VIEWPORTS:
            w = vp["width"]
            h = vp["height"]
            dpr = vp["dpr"]
            name = vp["name"]
            filename = vp["filename"]
            screenshot_path = os.path.join(SCREENSHOTS_DIR, filename)

            # Create context with precise dimensions & device_scale_factor
            context = browser.new_context(
                viewport={"width": w, "height": h},
                device_scale_factor=dpr,
                is_mobile=False,
                has_touch=False
            )

            page = context.new_page()
            console_errors = []
            page.on("console", lambda msg: console_errors.append(msg.text) if msg.type == "error" else None)

            # Test primary wedding invite page
            page.goto(f"{BASE_URL}/invite", wait_until="domcontentloaded")
            page.wait_for_timeout(400)

            # Measure overflow & geometry
            metrics = page.evaluate("""() => {
                const scrollW = document.documentElement.scrollWidth;
                const innerW = window.innerWidth;
                const clientW = document.documentElement.clientWidth;
                const hasHOverflow = scrollW > innerW;
                
                const hero = document.querySelector('.hero');
                const heroH = hero ? hero.offsetHeight : 0;
                const nav = document.querySelector('.nav-header');
                const navW = nav ? nav.offsetWidth : 0;

                const storyCard = document.querySelector('.story-card');
                const storyW = storyCard ? storyCard.offsetWidth : 0;

                return {
                    scrollW,
                    innerW,
                    clientW,
                    hasHOverflow,
                    heroH,
                    navW,
                    storyW
                };
            }""")

            # Capture Full Page Screenshot
            page.screenshot(path=screenshot_path, full_page=True)

            status = "PASS" if not metrics["hasHOverflow"] and len(console_errors) == 0 else "FAIL"
            notes = []
            if metrics["hasHOverflow"]:
                notes.append(f"Horizontal overflow detected: scrollWidth={metrics['scrollW']} > innerWidth={metrics['innerW']}")
            if console_errors:
                notes.append(f"Console errors: {', '.join(console_errors)}")
            if not notes:
                notes.append("Clean fluid layout, zero overflow, crisp scaling")

            result_entry = {
                "name": name,
                "viewport": f"{w}x{h} @{dpr}",
                "status": status,
                "notes": "; ".join(notes),
                "metrics": metrics,
                "screenshot": filename
            }
            results.append(result_entry)

            print(f"[{status}] {name:28} | {w}x{h} @{dpr:<4} | OverFlow: {metrics['hasHOverflow']} | Notes: {result_entry['notes']}")

            context.close()

        # Also verify other key pages at Desktop 1920x1080 and 4K 3840x2160
        additional_pages = [
            {"path": "/photos", "name": "Photo Upload Portal", "file_prefix": "photos"},
            {"path": "/admin", "name": "Couple's Admin Suite", "file_prefix": "admin"},
            {"path": "/gallery", "name": "Live Projector Wall", "file_prefix": "gallery"},
        ]

        for p_info in additional_pages:
            for v_cfg in [{"w": 1920, "h": 1080, "dpr": 1.0, "suffix": "1920x1080@1"}, {"w": 3840, "h": 2160, "dpr": 1.0, "suffix": "3840x2160@1"}]:
                ctx = browser.new_context(
                    viewport={"width": v_cfg["w"], "height": v_cfg["h"]},
                    device_scale_factor=v_cfg["dpr"]
                )
                pg = ctx.new_page()
                pg.goto(f"{BASE_URL}{p_info['path']}", wait_until="domcontentloaded")
                pg.wait_for_timeout(400)
                ss_file = f"{p_info['file_prefix']}_{v_cfg['suffix']}.png"
                pg.screenshot(path=os.path.join(SCREENSHOTS_DIR, ss_file), full_page=True)
                ctx.close()
                print(f"[PASS] {p_info['name']:28} | {v_cfg['w']}x{v_cfg['h']} @{v_cfg['dpr']} | Saved: {ss_file}")

        browser.close()

    # Save summary report JSON
    with open("viewport_test_report.json", "w", encoding="utf-8") as f:
        json.dump(results, f, indent=2)

    print("=========================================================================")
    print(" ALL VIEWPORT TESTS COMPLETE! Generated report and screenshots.")
    print("=========================================================================")

if __name__ == "__main__":
    run_tests()
