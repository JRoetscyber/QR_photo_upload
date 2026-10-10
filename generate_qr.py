#!/usr/bin/env python3
"""
Wedding QR Code & Table Stand Generator.

Production (default) - QR opens the guest photo upload page (/photos):
    python generate_qr.py
    python generate_qr.py --url https://jjwed.co.za

Local / venue Wi-Fi testing:
    python generate_qr.py --local [--ip 192.168.1.20] [--port 5167]

Outputs (in --out-dir, default ./qr_output):
    wedding_qr.png         print-resolution QR with champagne frame
    wedding_qr.svg         vector QR (scales infinitely for print shops)
    table_stand_card.html  self-contained printable card (QR embedded)
"""

import argparse
import base64
import html
import io
import os
import socket
import sys
from urllib.parse import urlparse

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")

try:
    import qrcode
    import qrcode.image.svg
    from PIL import Image, ImageDraw
except ImportError:
    sys.exit("Missing dependencies. Install with: pip install qrcode pillow")

UPLOAD_PATH = "/photos"  # guest photo upload portal route (see main.go); "/" is the invitation page
DEFAULT_URL = os.environ.get("WEDDING_URL", "https://jjwed.co.za")
DARK = "#2b2d42"


def get_local_ip():
    """Best LAN IPv4 address (used only with --local)."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.settimeout(0.5)
            s.connect(("8.8.8.8", 80))
            ip = s.getsockname()[0]
            if ip and not ip.startswith("127."):
                return ip
    except OSError:
        pass
    try:
        for ip in socket.gethostbyname_ex(socket.gethostname())[2]:
            if not ip.startswith(("127.", "169.254.")):
                return ip
    except OSError:
        pass
    return "127.0.0.1"


def normalize_url(raw, allow_insecure=False):
    """Validate and normalise the target URL."""
    raw = raw.strip()
    if "://" not in raw:
        raw = "https://" + raw
    parsed = urlparse(raw)
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        sys.exit(f"Invalid URL: {raw!r}")
    if parsed.scheme == "http" and not allow_insecure:
        sys.exit("Refusing http:// for a production QR code (phone cameras warn on it). "
                 "Use https://, or pass --local for LAN testing.")
    host = parsed.hostname or ""
    if not allow_insecure and (host in ("localhost", "example.com") or host.startswith(("127.", "192.168.", "10."))
                               or host.endswith("yourdomain.com")):
        sys.exit(f"{host!r} is not a public production host. Pass --url with the real domain.")
    # A bare domain means "the upload page": guests must land on the photo portal, not the invitation.
    return raw.rstrip("/") + UPLOAD_PATH if not parsed.path.strip("/") else raw.rstrip("/")


def build_qr(url, error_correction=qrcode.constants.ERROR_CORRECT_Q, box_size=20, border=4):
    qr = qrcode.QRCode(version=None, error_correction=error_correction, box_size=box_size, border=border)
    qr.add_data(url)
    qr.make(fit=True)
    return qr


def write_png(url, path):
    """High-resolution PNG (quiet zone preserved) inside a subtle champagne frame."""
    qr = build_qr(url)
    img = qr.make_image(fill_color=DARK, back_color="#ffffff").convert("RGBA")
    pad = 48
    w, h = img.size
    size = (w + pad * 2, h + pad * 2)
    card = Image.new("RGBA", size, (255, 255, 255, 255))
    d = ImageDraw.Draw(card)
    d.rectangle([10, 10, size[0] - 11, size[1] - 11], outline="#d4af37", width=5)
    d.rectangle([22, 22, size[0] - 23, size[1] - 23], outline="#f3e5ab", width=2)
    card.paste(img, (pad, pad))
    card.save(path, dpi=(300, 300), optimize=True)
    return path


def write_svg(url, path):
    qr = build_qr(url, box_size=10)
    img = qr.make_image(image_factory=qrcode.image.svg.SvgPathImage)
    img.save(path)
    return path


def png_data_uri(path):
    with open(path, "rb") as f:
        return "data:image/png;base64," + base64.b64encode(f.read()).decode("ascii")


CARD_TEMPLATE = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Table Card - Share Your Wedding Memories</title>
<style>
  @page {{ size: 4in 6in; margin: 0; }}
  :root {{ --gold: #c59b27; --gold-light: #f5e6be; --charcoal: #2d3142; --cream: #faf8f5; }}
  * {{ box-sizing: border-box; margin: 0; padding: 0; }}
  body {{
    font-family: 'Montserrat', 'Segoe UI', Helvetica, Arial, sans-serif;
    background: #eef1f6; color: var(--charcoal);
    display: flex; flex-direction: column; align-items: center;
    min-height: 100vh; padding: 24px;
    -webkit-print-color-adjust: exact; print-color-adjust: exact;
  }}
  .actions {{ margin-bottom: 24px; display: flex; gap: 12px; flex-wrap: wrap; justify-content: center; }}
  .btn {{
    background: var(--gold); color: #fff; border: 0; padding: 12px 24px;
    font-size: 15px; font-weight: 600; border-radius: 30px; cursor: pointer;
    text-decoration: none;
  }}
  .btn.secondary {{ background: #fff; color: var(--charcoal); border: 1px solid #ddd; }}
  .card {{
    width: 4in; max-width: 100%; aspect-ratio: 2 / 3; background: var(--cream);
    border: 1px solid #e0d8cc; border-radius: 12px;
    padding: 0.35in 0.3in; text-align: center; position: relative; overflow: hidden;
    display: flex; flex-direction: column; align-items: center; justify-content: center;
  }}
  .card::before {{ content: ''; position: absolute; inset: 0.12in; border: 1.5px solid var(--gold); border-radius: 8px; }}
  .card::after {{ content: ''; position: absolute; inset: 0.16in; border: 0.5px solid var(--gold-light); border-radius: 6px; }}
  h1 {{ font-family: 'Cormorant Garamond', Georgia, serif; font-size: 30px; font-weight: 600; letter-spacing: 1px; margin-bottom: 4px; }}
  .names {{ font-family: 'Cormorant Garamond', Georgia, serif; font-style: italic; font-size: 18px; color: var(--gold); margin-bottom: 18px; }}
  .qr {{ background: #fff; padding: 10px; border-radius: 10px; border: 1px solid #eee; margin-bottom: 16px; }}
  .qr img {{ display: block; width: min(2.1in, 52vw); height: auto; aspect-ratio: 1; image-rendering: pixelated; }}
  .instructions {{ font-size: 13px; line-height: 1.6; color: #555; margin-bottom: 12px; }}
  .url {{ max-width: 100%; overflow-wrap: anywhere; display: inline-block; background: #f0ebe1; padding: 5px 14px; border-radius: 20px; font-size: 12px; font-weight: 600; letter-spacing: .5px; }}
  @media (max-width: 480px) {{
    body {{ padding: 16px 12px; }}
    .btn {{ flex: 1 1 100%; text-align: center; padding: 14px 20px; }}
    h1 {{ font-size: clamp(22px, 7vw, 30px); }}
    .instructions {{ font-size: 12px; }}
  }}
  @media print {{
    body {{ background: #fff; padding: 0; min-height: 0; display: block; }}
    .actions {{ display: none; }}
    .card {{ width: 4in; height: 6in; aspect-ratio: auto; border: 0; border-radius: 0; margin: 0; page-break-inside: avoid; }}
    .qr img {{ width: 2.1in; }}
  }}
</style>
</head>
<body>
  <div class="actions">
    <button class="btn" onclick="window.print()">Print table card</button>
    <a class="btn secondary" href="{url}" target="_blank" rel="noopener">Open upload portal</a>
  </div>
  <div class="card">
    <h1>Capture the Love</h1>
    <div class="names">{names}</div>
    <div class="qr"><img src="{qr_data}" alt="QR code linking to {url}"></div>
    <p class="instructions">Open your phone camera, scan the code,<br>and share your photos from today.</p>
    <div class="url">{display_url}</div>
  </div>
</body>
</html>
"""


def write_card(url, png_path, path, names):
    display = urlparse(url)
    display_url = (display.netloc + display.path).rstrip("/")
    with open(path, "w", encoding="utf-8") as f:
        f.write(CARD_TEMPLATE.format(
            url=html.escape(url, quote=True),
            display_url=html.escape(display_url),
            names=html.escape(names),
            qr_data=png_data_uri(png_path),
        ))
    return path


def print_terminal_qr(url):
    qr = qrcode.QRCode(error_correction=qrcode.constants.ERROR_CORRECT_L, box_size=1, border=2)
    qr.add_data(url)
    qr.make(fit=True)
    print()
    qr.print_ascii(invert=True)
    print(f"\n  {url}\n")


def verify_decodes(png_path, url):
    """Best-effort round-trip check; silently skipped if no decoder is installed."""
    try:
        import cv2
    except ImportError:
        return None
    img = cv2.imread(png_path)
    if img is None:
        return False
    data, _, _ = cv2.QRCodeDetector().detectAndDecode(img)
    return data == url


def main():
    p = argparse.ArgumentParser(description="Generate the wedding upload QR code and table card.")
    p.add_argument("--url", help=f"Public URL to encode (default: $WEDDING_URL or {DEFAULT_URL})")
    p.add_argument("--local", action="store_true", help="Use http://<LAN-IP>:<port> for venue/LAN testing")
    p.add_argument("--ip", help="LAN IP for --local (auto-detected by default)")
    p.add_argument("--port", type=int, default=5167, help="Port for --local (default: 5167)")
    p.add_argument("--names", default="Jonathan & Julene", help="Names shown on the card")
    p.add_argument("--out-dir", default="qr_output", help="Output directory (default: qr_output)")
    p.add_argument("--no-terminal", action="store_true", help="Do not print the ASCII QR")
    args = p.parse_args()

    if args.local:
        url = f"http://{args.ip or get_local_ip()}:{args.port}{UPLOAD_PATH}"
        print("WARNING: --local QR codes only work on the same network. Do not print these for the event.")
    else:
        url = normalize_url(args.url or DEFAULT_URL)

    os.makedirs(args.out_dir, exist_ok=True)
    png = write_png(url, os.path.join(args.out_dir, "wedding_qr.png"))
    svg = write_svg(url, os.path.join(args.out_dir, "wedding_qr.svg"))
    card = write_card(url, png, os.path.join(args.out_dir, "table_stand_card.html"), args.names)

    print(f"Target URL: {url}")
    for path in (png, svg, card):
        print(f"Wrote {path}")

    ok = verify_decodes(png, url)
    if ok is True:
        print("Verified: PNG decodes back to the target URL.")
    elif ok is False:
        sys.exit("ERROR: generated PNG did not decode to the target URL.")

    if not args.no_terminal:
        print_terminal_qr(url)


if __name__ == "__main__":
    main()
