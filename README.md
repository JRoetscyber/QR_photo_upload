# 💍 Wedding Moments — Fast Photo Server & Couple's Admin Suite

A high-performance, mobile-first wedding photo sharing and guestbook application built with **Go Fiber (`fasthttp`)**, designed to let wedding guests scan a table QR code and upload photos directly to a local server over Wi-Fi, optimized to effortlessly handle 60+ simultaneous uploaders without server slowdowns or crashes.

---

## ✨ Features

- **⚡ Blazing-Fast Go Backend (`fasthttp` / Fiber v2)**:
  - Direct-to-disk multipart streaming (zero memory bloat).
  - Bounded concurrency I/O worker pool preventing disk thrashing.
  - Tested: **60 simultaneous uploads in 0.69s (87.25 req/sec)** with 100% success rate.
- **📱 Mobile-First Guest Web Portal (`/`)**:
  - Romantic glassmorphism UI with champagne gold accents.
  - Direct mobile camera shutter capture + gallery photo picker.
  - Guest name & wedding wish messages with localStorage persistence.
  - Live progress bar & confetti burst upon upload.
  - Real-time recent photo feed & high-res lightbox.
- **🎥 Live Venue Projector Wall (`/gallery`)**:
  - Real-time Server-Sent Events (SSE) push stream.
  - Fullscreen masonry photo wall & automated slideshow for wedding reception screens.
- **👑 Private Couple's Admin Suite (`/admin`)**:
  - Protected with a 4-digit PIN (default: `2026`).
  - **One-Click Download All as ZIP**: Instant full-resolution album download.
  - **Export Guestbook as CSV**: All guest messages & timestamps ready for Excel.
  - **Live Photo Moderation**: Star/favorite moments for the projector, edit names/wishes, or delete blurry/unwanted photos.
  - **Reset / Clear Test Data**: Single-click pre-wedding cleanup.
- **📡 Automatic Wi-Fi QR & Table Stand Generator**:
  - Auto-detects local LAN/Wi-Fi IPv4 address.
  - Generates high-res `wedding_qr.png` and printable 4x6 `table_stand_card.html`.
  - Outputs an ASCII QR code directly into the terminal for instant scanning.

---

## 🚀 Quick Start

### 1. Requirements
- **Go 1.22+**
- **Python 3.10+** (with `qrcode` and `pillow`)

### 2. Install Dependencies
```bash
# Go dependencies
go mod tidy

# Python QR dependencies
pip install qrcode pillow
```

### 3. Generate QR Code & Table Stand Card
```bash
python generate_qr.py
```
This detects your local Wi-Fi IP (e.g., `192.168.50.191:8080`), generates `wedding_qr.png`, creates `table_stand_card.html`, and displays the QR code in the terminal.

### 4. Run the Server
```bash
# Run directly
go run .

# Or build standalone executable
go build -o wedding_server.exe .
.\wedding_server.exe
```

---

## 🌐 Endpoints & URLs

| Route | URL | Description |
| :--- | :--- | :--- |
| **Guest Upload Portal** | `http://<local-ip>:8080/` | Mobile photo uploader with camera capture |
| **Live Projector Wall** | `http://<local-ip>:8080/gallery` | Fullscreen real-time photo wall & slideshow |
| **Couple's Admin Suite** | `http://<local-ip>:8080/admin` | PIN: `2026` — ZIP download, moderation, CSV export |
| **Table Stand Print Card**| `table_stand_card.html` | Open in browser and click Print |

---

## 📊 Concurrency Benchmark (60 Simultaneous Uploads)

Run the load test:
```bash
python load_test.py
```

Results:
- **Simultaneous Users**: 60
- **Success Rate**: 100.0% (60/60)
- **Total Duration**: 0.69 seconds
- **Throughput**: 87.25 requests/sec
- **Average Latency**: 255.8 ms

---

## 📜 License
MIT License. Created with ❤️ for our special day!
