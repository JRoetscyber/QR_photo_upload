package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"mime/multipart"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"webbing/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

const (
	DefaultPort       = ":5167"
	UploadsDirectory  = "./uploads"
	DataFile          = "./data/photos.json"
	DatabaseFile      = "./data/wedding.db"
	MaxBodyLimitBytes = 150 * 1024 * 1024 // 150MB per batch request
	MaxDiskWriters    = 64                // Concurrent disk writes semaphore limit
	DefaultAdminPIN   = "2026"            // Default admin access PIN
)

var (
	Port     = DefaultPort
	AdminPIN = DefaultAdminPIN
)

func main() {
	// Read environment variables if provided (for Docker / Cloudflare deployment)
	if envPort := os.Getenv("PORT"); envPort != "" {
		if !strings.HasPrefix(envPort, ":") {
			Port = ":" + envPort
		} else {
			Port = envPort
		}
	}
	if envPIN := os.Getenv("ADMIN_PIN"); envPIN != "" {
		AdminPIN = envPIN
	}

	// Initialize thread-safe photo storage engine
	store, err := storage.NewStore(UploadsDirectory, DataFile, MaxDiskWriters)
	if err != nil {
		log.Fatalf("Failed to initialize photo storage: %v", err)
	}

	// Initialize thread-safe SQLite database with goroutine worker pools
	db, err := storage.NewDB(DatabaseFile)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer db.Close()

	// Create Fiber app configured for high-concurrency Fasthttp delivery
	app := fiber.New(fiber.Config{
		BodyLimit:             MaxBodyLimitBytes,
		Concurrency:           256 * 1024,
		ReadBufferSize:        16 * 1024,
		WriteBufferSize:       16 * 1024,
		ServerHeader:          "WebbingFastServer/1.0",
		AppName:               "South African Highveld Wedding Platform",
		DisableStartupMessage: false,
	})

	// Middlewares
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-Admin-PIN, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${latency} ${method} ${path}\n",
		TimeFormat: "15:04:05",
	}))

	// API: Health & Server Status
	app.Get("/api/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"engine": "fasthttp/fiber",
			"db":     "sqlite3-wal",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// ==================== RSVP APIs (SQLite + Goroutines) ====================

	// API: Submit RSVP
	app.Post("/api/rsvp", func(c *fiber.Ctx) error {
		var req struct {
			Name             string `json:"name"`
			Email            string `json:"email"`
			Phone            string `json:"phone"`
			Attending        bool   `json:"attending"`
			GuestCount       int    `json:"guest_count"`
			AdditionalGuests string `json:"additional_guests"`
			SongRequest      string `json:"song_request"`
			Message          string `json:"message"`
		}

		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request payload",
			})
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Please provide your name",
			})
		}

		if req.Attending && req.GuestCount <= 0 {
			req.GuestCount = 1
		} else if !req.Attending {
			req.GuestCount = 0
		}

		rsvpEntry := storage.RSVP{
			ID:               uuid.New().String(),
			Name:             req.Name,
			Email:            strings.TrimSpace(req.Email),
			Phone:            strings.TrimSpace(req.Phone),
			Attending:        req.Attending,
			GuestCount:       req.GuestCount,
			AdditionalGuests: strings.TrimSpace(req.AdditionalGuests),
			SongRequest:      strings.TrimSpace(req.SongRequest),
			Message:          strings.TrimSpace(req.Message),
			SubmittedTime:    time.Now(),
		}

		// Asynchronous ingestion via Go goroutines channel queue
		db.EnqueueRSVP(rsvpEntry)

		statusMsg := "Thank you! We can't wait to celebrate under the Highveld skies with you!"
		if !req.Attending {
			statusMsg = "Thank you for letting us know. You will be dearly missed!"
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"success": true,
			"id":      rsvpEntry.ID,
			"message": statusMsg,
		})
	})

	// ==================== GUEST PHOTO UPLOAD APIs ====================

	// API: High-Concurrency Photo Upload
	app.Post("/api/upload", func(c *fiber.Ctx) error {
		form, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to parse multipart form data",
			})
		}

		files := form.File["photos"]
		if len(files) == 0 {
			if len(form.File["files"]) > 0 {
				files = form.File["files"]
			} else if len(form.File["photo"]) > 0 {
				files = form.File["photo"]
			} else {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "No photo files uploaded. Please select at least one photo.",
				})
			}
		}

		guestName := ""
		if names, ok := form.Value["guest_name"]; ok && len(names) > 0 {
			guestName = names[0]
		}
		wish := ""
		if wishes, ok := form.Value["wish"]; ok && len(wishes) > 0 {
			wish = wishes[0]
		}

		// Process uploads in parallel goroutines for maximum throughput
		type uploadResult struct {
			meta *storage.PhotoMetadata
			err  error
		}

		results := make([]uploadResult, len(files))
		var wg sync.WaitGroup
		wg.Add(len(files))

		for i, file := range files {
			go func(idx int, fh *multipart.FileHeader) {
				defer wg.Done()
				meta, saveErr := store.SaveUploadedFile(fh, guestName, wish)
				results[idx] = uploadResult{meta: meta, err: saveErr}
			}(i, file)
		}
		wg.Wait()

		var savedPhotos []*storage.PhotoMetadata
		var errors []string

		for _, res := range results {
			if res.err != nil {
				errors = append(errors, res.err.Error())
			} else if res.meta != nil {
				savedPhotos = append(savedPhotos, res.meta)
			}
		}

		if len(savedPhotos) == 0 && len(errors) > 0 {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Failed to save any photos",
				"details": errors,
			})
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"success": true,
			"count":   len(savedPhotos),
			"photos":  savedPhotos,
			"errors":  errors,
			"message": fmt.Sprintf("Successfully saved %d photo(s). Thank you %s!", len(savedPhotos), guestName),
		})
	})

	// API: Get Photo Feed (Paginated)
	app.Get("/api/photos", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 60)
		offset := c.QueryInt("offset", 0)

		photos, total := store.GetPhotos(limit, offset)
		return c.JSON(fiber.Map{
			"photos": photos,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	})

	// API: Live Server-Sent Events (SSE) for Gallery Projector
	app.Get("/api/stream", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		subChan := store.Subscribe()

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			defer store.Unsubscribe(subChan)

			// Send initial keepalive ping
			fmt.Fprintf(w, "event: ping\ndata: connected\n\n")
			_ = w.Flush()

			for photo := range subChan {
				data := fmt.Sprintf(`{"id":"%s","url":"%s","guest_name":"%s","wish":"%s","time":"%s","favorite":%t}`,
					photo.ID, photo.URL, photo.GuestName, photo.Wish, photo.UploadTime.Format("15:04"), photo.Favorite)
				fmt.Fprintf(w, "event: new_photo\ndata: %s\n\n", data)
				if err := w.Flush(); err != nil {
					return
				}
			}
		}))

		return nil
	})

	// API: Photo Upload Statistics
	app.Get("/api/stats", func(c *fiber.Ctx) error {
		stats := store.GetStats()
		return c.JSON(stats)
	})

	// ==================== ADMIN APIs ====================

	// Admin Auth Middleware Helper
	adminAuth := func(c *fiber.Ctx) error {
		pin := c.Get("X-Admin-PIN")
		if pin == "" {
			pin = c.Query("pin")
		}
		if pin == "" {
			pin = c.Cookies("wedding_admin_pin")
		}
		if pin != AdminPIN {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or missing Admin PIN",
			})
		}
		return c.Next()
	}

	// Admin: Login / Verify PIN
	app.Post("/api/admin/login", func(c *fiber.Ctx) error {
		var req struct {
			PIN string `json:"pin"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		if req.PIN == AdminPIN {
			c.Cookie(&fiber.Cookie{
				Name:     "wedding_admin_pin",
				Value:    AdminPIN,
				Expires:  time.Now().Add(30 * 24 * time.Hour),
				HTTPOnly: false,
				SameSite: "Lax",
			})
			return c.JSON(fiber.Map{
				"success": true,
				"message": "Welcome, Jonathan & Julene!",
			})
		}

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Incorrect PIN. Please try again.",
		})
	})

	// Admin: Get all RSVPs from SQLite
	app.Get("/api/admin/rsvps", adminAuth, func(c *fiber.Ctx) error {
		rsvps, err := db.GetRSVPs()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		stats, _ := db.GetRSVPStats()
		return c.JSON(fiber.Map{
			"rsvps": rsvps,
			"stats": stats,
		})
	})

	// Admin: Get RSVP Summary Stats
	app.Get("/api/admin/rsvps/stats", adminAuth, func(c *fiber.Ctx) error {
		stats, err := db.GetRSVPStats()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(stats)
	})

	// Admin: Delete RSVP
	app.Delete("/api/admin/rsvps/:id", adminAuth, func(c *fiber.Ctx) error {
		id := c.Params("id")
		if err := db.DeleteRSVP(id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "message": "RSVP removed"})
	})

	// Admin: Export RSVPs as CSV
	app.Get("/api/admin/export-rsvps", adminAuth, func(c *fiber.Ctx) error {
		csvContent, err := db.ExportRSVPsCSV()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", `attachment; filename="wedding_guest_rsvps.csv"`)
		return c.SendString(csvContent)
	})

	// Admin: Stream All Photos as ZIP
	app.Get("/api/admin/download-zip", adminAuth, func(c *fiber.Ctx) error {
		zipFilename := fmt.Sprintf("Wedding_Photos_%s.zip", time.Now().Format("2006-01-02"))
		c.Set("Content-Type", "application/zip")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipFilename))

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			_ = store.StreamAllPhotosZip(w)
			_ = w.Flush()
		}))

		return nil
	})

	// Admin: Delete a photo
	app.Delete("/api/admin/photos/:id", adminAuth, func(c *fiber.Ctx) error {
		id := c.Params("id")
		if err := store.DeletePhoto(id); err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "message": "Photo deleted"})
	})

	// Admin: Toggle Favorite / Spotlight for Projector
	app.Post("/api/admin/photos/:id/favorite", adminAuth, func(c *fiber.Ctx) error {
		id := c.Params("id")
		fav, err := store.ToggleFavorite(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "favorite": fav})
	})

	// Admin: Edit photo guest name or note
	app.Put("/api/admin/photos/:id", adminAuth, func(c *fiber.Ctx) error {
		id := c.Params("id")
		var req struct {
			GuestName string `json:"guest_name"`
			Wish      string `json:"wish"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}

		if err := store.UpdatePhoto(id, req.GuestName, req.Wish); err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "message": "Photo updated"})
	})

	// Admin: Export Guestbook as CSV
	app.Get("/api/admin/export-guestbook", adminAuth, func(c *fiber.Ctx) error {
		photos := store.GetAllPhotos()
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", `attachment; filename="wedding_guestbook_wishes.csv"`)

		var b strings.Builder
		b.WriteString("\xEF\xBB\xBF")

		writer := csv.NewWriter(&b)
		_ = writer.Write([]string{"Upload Time", "Guest Name", "Wedding Wish / Message", "Photo Filename", "Photo URL", "Favorite"})

		for _, p := range photos {
			favStr := "No"
			if p.Favorite {
				favStr = "Yes"
			}
			_ = writer.Write([]string{
				p.UploadTime.Format("2006-01-02 15:04:05"),
				p.GuestName,
				p.Wish,
				p.Filename,
				p.URL,
				favStr,
			})
		}
		writer.Flush()

		return c.SendString(b.String())
	})

	// Admin: Clear All Photos (Testing reset)
	app.Post("/api/admin/clear", adminAuth, func(c *fiber.Ctx) error {
		if err := store.ClearAllPhotos(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "message": "All test photos and metadata cleared!"})
	})

	// ==================== STATIC ROUTES & PAGES ====================

	// Serve Uploaded Files
	app.Static("/uploads", UploadsDirectory, fiber.Static{
		Compress:  false,
		ByteRange: true,
		MaxAge:    3600,
	})

	// Serve Static Frontend
	app.Static("/", "./public", fiber.Static{
		Index:    "invite.html", // Main landing page is the South African Highveld Wedding Invitation
		Compress: true,
	})

	// Route for wedding invitation & RSVP
	app.Get("/invite", func(c *fiber.Ctx) error {
		return c.SendFile("./public/invite.html")
	})

	// Route for guest photo uploader portal
	app.Get("/photos", func(c *fiber.Ctx) error {
		return c.SendFile("./public/index.html")
	})

	// Route for live projector wall
	app.Get("/gallery", func(c *fiber.Ctx) error {
		return c.SendFile("./public/gallery.html")
	})

	// Route for couple's admin panel
	app.Get("/admin", func(c *fiber.Ctx) error {
		return c.SendFile("./public/admin.html")
	})

	// Graceful Shutdown Setup
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("🌾 South African Highveld Wedding Platform is running on http://0.0.0.0%s", Port)
		log.Printf("💌 Online Wedding Invitation & RSVP: http://localhost%s/invite", Port)
		log.Printf("📸 Guest Photo Upload Portal: http://localhost%s/photos", Port)
		log.Printf("👑 Couple's Admin Suite (RSVPs & Photos): http://localhost%s/admin (PIN: %s)", Port, AdminPIN)
		log.Printf("🎥 Live Projector Wall: http://localhost%s/gallery", Port)
		if err := app.Listen(Port); err != nil {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down wedding server gracefully...")
	_ = app.Shutdown()
	log.Println("Server stopped.")
}

func init() {
	_ = strconv.Itoa(0)
}
