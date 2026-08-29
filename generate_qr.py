#!/usr/bin/env python3
"""
Wedding QR Code & Table Stand Generator
Detects the local Wi-Fi IP address, generates a high-resolution QR code PNG,
creates a printable table-stand card HTML file, and outputs an ASCII QR code to the terminal.
"""

import sys
import os
import socket
import argparse

# Ensure UTF-8 output on Windows consoles
if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

import qrcode
from PIL import Image, ImageDraw

def get_local_ip():
    """Finds the best local LAN/Wi-Fi IPv4 address."""
    # First try connecting to a public DNS IP to determine the outbound Wi-Fi adapter
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.settimeout(0.5)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        if ip and not ip.startswith("127."):
            return ip
    except Exception:
        pass

    # Fallback to hostname lookup
    try:
        hostname = socket.gethostname()
        for ip in socket.gethostbyname_ex(hostname)[2]:
            if not ip.startswith("127.") and not ip.startswith("169.254."):
                return ip
    except Exception:
        pass

    return "127.0.0.1"

def generate_styled_qr(url, output_png="wedding_qr.png"):
    """Generates a high-quality styled QR code with subtle elegant styling."""
    qr = qrcode.QRCode(
        version=1,
        error_correction=qrcode.constants.ERROR_CORRECT_H,
        box_size=12,
        border=3,
    )
    qr.add_data(url)
    qr.make(fit=True)

    # Base QR Code
    img = qr.make_image(fill_color="#2b2d42", back_color="#ffffff").convert("RGBA")
    
    # Add a delicate gold/champagne outer frame
    border_width = 24
    w, h = img.size
    card_size = (w + border_width * 2, h + border_width * 2)
    card = Image.new("RGBA", card_size, (255, 255, 255, 255))
    
    draw = ImageDraw.Draw(card)
    # Subtle champagne border
    draw.rectangle([6, 6, card_size[0] - 7, card_size[1] - 7], outline="#d4af37", width=3)
    draw.rectangle([12, 12, card_size[0] - 13, card_size[1] - 13], outline="#f3e5ab", width=1)
    
    # Paste QR code in center
    card.paste(img, (border_width, border_width), img)
    card.save(output_png)
    print(f"✅ Generated high-resolution QR code PNG: {output_png}")
    return output_png

def generate_table_card_html(url, ip, port, output_html="table_stand_card.html"):
    """Generates a printable, elegant wedding table card."""
    html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Share Your Wedding Memories - Table Card</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,600;1,400&family=Montserrat:wght@300;400;600&display=swap" rel="stylesheet">
  <style>
    :root {{
      --gold-primary: #c59b27;
      --gold-light: #f5e6be;
      --charcoal: #2d3142;
      --cream: #faf8f5;
    }}
    * {{
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }}
    body {{
      font-family: 'Montserrat', sans-serif;
      background: #eef1f6;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      padding: 24px;
      color: var(--charcoal);
    }}
    .print-actions {{
      margin-bottom: 24px;
      display: flex;
      gap: 12px;
    }}
    .btn {{
      background: var(--gold-primary);
      color: white;
      border: none;
      padding: 12px 24px;
      font-size: 15px;
      font-weight: 600;
      border-radius: 30px;
      cursor: pointer;
      box-shadow: 0 4px 14px rgba(197, 155, 39, 0.35);
      transition: all 0.2s ease;
    }}
    .btn:hover {{
      transform: translateY(-2px);
      box-shadow: 0 6px 20px rgba(197, 155, 39, 0.45);
    }}
    .btn-secondary {{
      background: white;
      color: var(--charcoal);
      border: 1px solid #ddd;
    }}

    /* Printable Card (4x6 / 5x7 ratio) */
    .card {{
      width: 380px;
      min-height: 540px;
      background: var(--cream);
      border: 1px solid #e0d8cc;
      border-radius: 16px;
      box-shadow: 0 16px 40px rgba(0, 0, 0, 0.1);
      padding: 36px 28px;
      text-align: center;
      position: relative;
      overflow: hidden;
    }}
    .card::before {{
      content: '';
      position: absolute;
      inset: 12px;
      border: 1.5px solid var(--gold-primary);
      border-radius: 10px;
      pointer-events: none;
    }}
    .card::after {{
      content: '';
      position: absolute;
      inset: 16px;
      border: 0.5px solid var(--gold-light);
      border-radius: 8px;
      pointer-events: none;
    }}
    .rings-icon {{
      font-size: 32px;
      margin-bottom: 6px;
      color: var(--gold-primary);
    }}
    h1 {{
      font-family: 'Cormorant Garamond', Georgia, serif;
      font-size: 32px;
      font-weight: 600;
      color: var(--charcoal);
      margin-bottom: 6px;
      letter-spacing: 1px;
    }}
    .subtitle {{
      font-family: 'Cormorant Garamond', Georgia, serif;
      font-style: italic;
      font-size: 18px;
      color: var(--gold-primary);
      margin-bottom: 20px;
    }}
    .qr-container {{
      background: white;
      padding: 14px;
      border-radius: 12px;
      box-shadow: 0 4px 16px rgba(0,0,0,0.06);
      display: inline-block;
      margin-bottom: 18px;
      border: 1px solid #eee;
    }}
    .qr-container img {{
      display: block;
      width: 190px;
      height: 190px;
    }}
    .instructions {{
      font-size: 13px;
      line-height: 1.6;
      color: #555;
      margin-bottom: 12px;
    }}
    .instructions strong {{
      color: var(--charcoal);
    }}
    .url-badge {{
      display: inline-block;
      background: #f0ebe1;
      padding: 5px 14px;
      border-radius: 20px;
      font-size: 11.5px;
      font-weight: 600;
      color: var(--charcoal);
      letter-spacing: 0.5px;
    }}
    .wifi-note {{
      font-size: 11px;
      color: #888;
      margin-top: 14px;
    }}

    @media print {{
      body {{
        background: white;
        padding: 0;
      }}
      .print-actions {{
        display: none;
      }}
      .card {{
        box-shadow: none;
        border: 1px solid #ccc;
        margin: auto;
        page-break-inside: avoid;
      }}
    }}
  </style>
</head>
<body>

  <div class="print-actions">
    <button class="btn" onclick="window.print()">🖨️ Print Table Stand Card</button>
    <a href="{url}" target="_blank" style="text-decoration:none;">
      <button class="btn btn-secondary">🌐 Open Web Upload Portal</button>
    </a>
  </div>

  <div class="card">
    <div class="rings-icon">💍</div>
    <h1>Capture the Love</h1>
    <div class="subtitle">Share your photos from tonight</div>

    <div class="qr-container">
      <img src="wedding_qr.png" alt="Wedding Upload QR Code">
    </div>

    <p class="instructions">
      Open your phone camera, scan the QR code,<br>
      and upload all your candid moments!
    </p>

    <div class="url-badge">{url}</div>

    <p class="wifi-note">
      📶 Connect to the venue Wi-Fi to upload instantly!
    </p>
  </div>

</body>
</html>
"""
    with open(output_html, "w", encoding="utf-8") as f:
        f.write(html_content)
    print(f"✅ Generated printable table-stand HTML card: {output_html}")
    return output_html

def print_terminal_qr(url):
    """Prints a beautiful ASCII QR code directly into the terminal."""
    print("\n" + "="*50)
    print(" 📸 WEDDING PHOTO UPLOAD - SCAN WITH YOUR PHONE")
    print("="*50 + "\n")
    
    qr = qrcode.QRCode(
        version=1,
        error_correction=qrcode.constants.ERROR_CORRECT_L,
        box_size=1,
        border=2,
    )
    qr.add_data(url)
    qr.make(fit=True)
    qr.print_ascii(invert=True)
    
    print("\n" + "="*50)
    print(f" 🌐 Access URL: {url}")
    print("="*50 + "\n")

def main():
    parser = argparse.ArgumentParser(description="Generate Wedding Wi-Fi QR Code")
    parser.add_argument("--ip", type=str, default=None, help="Explicit IP address (defaults to auto-detected Wi-Fi IP)")
    parser.add_argument("--port", type=int, default=8080, help="Server port (default: 8080)")
    args = parser.parse_args()

    ip = args.ip if args.ip else get_local_ip()
    port = args.port
    url = f"http://{ip}:{port}"

    print(f"📡 Detected local Wi-Fi network address: {ip}")
    print(f"🔗 Target Wedding Upload URL: {url}")

    # Generate PNG
    generate_styled_qr(url, "wedding_qr.png")
    
    # Generate Printable Card
    generate_table_card_html(url, ip, port, "table_stand_card.html")

    # Output terminal QR
    print_terminal_qr(url)

if __name__ == "__main__":
    main()
