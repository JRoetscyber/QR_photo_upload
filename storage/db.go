package storage

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// RSVP represents a guest RSVP entry in SQLite
type RSVP struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Email            string    `json:"email"`
	Phone            string    `json:"phone"`
	Attending        bool      `json:"attending"`
	GuestCount       int       `json:"guest_count"`
	AdditionalGuests string    `json:"additional_guests"`
	SongRequest      string    `json:"song_request"`
	Message          string    `json:"message"`
	HasUploaded      bool      `json:"has_uploaded"`
	UploadCount      int       `json:"upload_count"`
	LastRemindedAt   *time.Time `json:"last_reminded_at"`
	SubmittedTime    time.Time `json:"submitted_time"`
}

// RSVPStats represents summary counts for the couple
type RSVPStats struct {
	TotalResponses int `json:"total_responses"`
	TotalAttending int `json:"total_attending"` // Sum of all attending seats/guests
	TotalDeclined  int `json:"total_declined"`
	AttendingCount int `json:"attending_count"` // Number of positive RSVP submissions
	UploadedCount  int `json:"uploaded_count"`  // Guests who have uploaded photos
	PendingUploads int `json:"pending_uploads"` // Attending guests who haven't uploaded yet
}

// PushSubscription represents a Web Push subscriber
type PushSubscription struct {
	ID        string    `json:"id"`
	Endpoint  string    `json:"endpoint"`
	P256dh    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	GuestName string    `json:"guest_name"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationLog tracks broadcast and reminder dispatches
type NotificationLog struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Message      string    `json:"message"`
	Type         string    `json:"type"` // "broadcast" or "photo_reminder"
	SentCount    int       `json:"sent_count"`
	SkippedCount int       `json:"skipped_count"`
	SentAt       time.Time `json:"sent_at"`
}

// DB handles thread-safe SQLite operations with WAL mode, MMAP, and in-memory atomic caching
type DB struct {
	db        *sql.DB
	mu        sync.RWMutex
	rsvpQueue chan RSVP

	// In-memory atomic cache for sub-millisecond responses
	cachedTotalResponses atomic.Int64
	cachedTotalAttending atomic.Int64
	cachedTotalDeclined  atomic.Int64
	cachedAttendingCount atomic.Int64
	cachedUploadedCount  atomic.Int64
}

// NewDB initializes the SQLite database with high-performance PRAGMAs: WAL mode, 256MB MMAP, 64MB Cache
func NewDB(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Supercharged F1-grade SQLite connection string with WAL and MMAP
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// High concurrency connection pool
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(time.Hour)

	s := &DB{
		db:        db,
		rsvpQueue: make(chan RSVP, 1000),
	}

	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Warm up in-memory stats cache from disk
	s.warmupCache()

	// Dedicated worker goroutines for high-throughput async processing
	for i := 0; i < 4; i++ {
		go s.rsvpWorker()
	}

	return s, nil
}

func (s *DB) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS rsvps (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT,
		phone TEXT,
		attending INTEGER NOT NULL,
		guest_count INTEGER NOT NULL DEFAULT 1,
		additional_guests TEXT,
		song_request TEXT,
		message TEXT,
		has_uploaded INTEGER NOT NULL DEFAULT 0,
		upload_count INTEGER NOT NULL DEFAULT 0,
		last_reminded_at DATETIME,
		submitted_time DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS subscriptions (
		id TEXT PRIMARY KEY,
		endpoint TEXT UNIQUE NOT NULL,
		p256dh TEXT NOT NULL,
		auth TEXT NOT NULL,
		guest_name TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS notifications (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		type TEXT NOT NULL,
		sent_count INTEGER NOT NULL,
		skipped_count INTEGER NOT NULL,
		sent_at DATETIME NOT NULL
	);
	`
	if _, err := s.db.Exec(query); err != nil {
		return err
	}

	// Safe auto-migration for existing tables:
	_, _ = s.db.Exec("ALTER TABLE rsvps ADD COLUMN has_uploaded INTEGER NOT NULL DEFAULT 0;")
	_, _ = s.db.Exec("ALTER TABLE rsvps ADD COLUMN upload_count INTEGER NOT NULL DEFAULT 0;")
	_, _ = s.db.Exec("ALTER TABLE rsvps ADD COLUMN last_reminded_at DATETIME;")

	// Create indexes
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rsvps_attending ON rsvps(attending);")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rsvps_has_uploaded ON rsvps(has_uploaded);")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rsvps_time ON rsvps(submitted_time DESC);")
	_, _ = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_sub_endpoint ON subscriptions(endpoint);")

	return nil
}

func (s *DB) warmupCache() {
	var total, attCount, totalAtt, dec, uploaded int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM rsvps").Scan(&total)
	_ = s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(guest_count), 0) FROM rsvps WHERE attending = 1").Scan(&attCount, &totalAtt)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM rsvps WHERE attending = 0").Scan(&dec)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM rsvps WHERE attending = 1 AND has_uploaded = 1").Scan(&uploaded)

	s.cachedTotalResponses.Store(int64(total))
	s.cachedAttendingCount.Store(int64(attCount))
	s.cachedTotalAttending.Store(int64(totalAtt))
	s.cachedTotalDeclined.Store(int64(dec))
	s.cachedUploadedCount.Store(int64(uploaded))
}

func (s *DB) rsvpWorker() {
	for r := range s.rsvpQueue {
		s.insertRSVPSync(r)
	}
}

func (s *DB) insertRSVPSync(r RSVP) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attendingInt := 0
	if r.Attending {
		attendingInt = 1
	}

	query := `
	INSERT INTO rsvps (id, name, email, phone, attending, guest_count, additional_guests, song_request, message, has_uploaded, upload_count, submitted_time)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?);
	`
	_, err := s.db.Exec(query,
		r.ID,
		r.Name,
		r.Email,
		r.Phone,
		attendingInt,
		r.GuestCount,
		r.AdditionalGuests,
		r.SongRequest,
		r.Message,
		r.SubmittedTime,
	)
	if err != nil {
		log.Printf("[SQLite Error] failed to insert RSVP %s: %v", r.ID, err)
	}
}

// EnqueueRSVP pushes an RSVP into the high-speed goroutine worker queue and immediately updates in-memory atomic cache
func (s *DB) EnqueueRSVP(r RSVP) {
	s.cachedTotalResponses.Add(1)
	if r.Attending {
		s.cachedAttendingCount.Add(1)
		s.cachedTotalAttending.Add(int64(r.GuestCount))
	} else {
		s.cachedTotalDeclined.Add(1)
	}

	s.rsvpQueue <- r
}

// RecordGuestUpload marks a guest as having uploaded photos so they are excluded from post-wedding reminders
func (s *DB) RecordGuestUpload(guestName string) {
	guestName = strings.TrimSpace(guestName)
	if guestName == "" || strings.EqualFold(guestName, "Anonymous Guest") || strings.EqualFold(guestName, "Guest") {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update the matching RSVP entry by exact (case-insensitive, trimmed) name match.
	// A substring match here would also match unrelated guests sharing a short
	// name fragment (e.g. "Jo" matching "Jonathan", "Joanna", ...).
	query := `
	UPDATE rsvps
	SET has_uploaded = 1, upload_count = upload_count + 1
	WHERE LOWER(TRIM(name)) = LOWER(?);
	`
	res, err := s.db.Exec(query, guestName)
	if err == nil {
		if rows, _ := res.RowsAffected(); rows > 0 {
			s.warmupCache()
		}
	}
}

// GetRSVPs returns all RSVPs ordered by newest first
func (s *DB) GetRSVPs() ([]RSVP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, name, email, phone, attending, guest_count, additional_guests, song_request, message, has_uploaded, upload_count, last_reminded_at, submitted_time
	FROM rsvps
	ORDER BY submitted_time DESC;
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]RSVP, 0, 64)
	for rows.Next() {
		var r RSVP
		var attendingInt, hasUploadedInt int
		var subTime time.Time
		var remindedAt sql.NullTime

		if err := rows.Scan(
			&r.ID,
			&r.Name,
			&r.Email,
			&r.Phone,
			&attendingInt,
			&r.GuestCount,
			&r.AdditionalGuests,
			&r.SongRequest,
			&r.Message,
			&hasUploadedInt,
			&r.UploadCount,
			&remindedAt,
			&subTime,
		); err != nil {
			continue
		}
		r.Attending = (attendingInt == 1)
		r.HasUploaded = (hasUploadedInt == 1)
		if remindedAt.Valid {
			r.LastRemindedAt = &remindedAt.Time
		}
		r.SubmittedTime = subTime
		list = append(list, r)
	}

	return list, nil
}

// GetUnuploadedAttendingGuests retrieves guests who confirmed attendance but have NOT uploaded photos yet
func (s *DB) GetUnuploadedAttendingGuests() ([]RSVP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, name, email, phone, attending, guest_count, additional_guests, song_request, message, has_uploaded, upload_count, last_reminded_at, submitted_time
	FROM rsvps
	WHERE attending = 1 AND has_uploaded = 0
	ORDER BY submitted_time ASC;
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]RSVP, 0, 32)
	for rows.Next() {
		var r RSVP
		var attendingInt, hasUploadedInt int
		var subTime time.Time
		var remindedAt sql.NullTime

		if err := rows.Scan(
			&r.ID,
			&r.Name,
			&r.Email,
			&r.Phone,
			&attendingInt,
			&r.GuestCount,
			&r.AdditionalGuests,
			&r.SongRequest,
			&r.Message,
			&hasUploadedInt,
			&r.UploadCount,
			&remindedAt,
			&subTime,
		); err != nil {
			continue
		}
		r.Attending = (attendingInt == 1)
		r.HasUploaded = false
		if remindedAt.Valid {
			r.LastRemindedAt = &remindedAt.Time
		}
		r.SubmittedTime = subTime
		list = append(list, r)
	}

	return list, nil
}

// MarkGuestsReminded updates the last_reminded_at timestamp for a list of RSVP IDs
func (s *DB) MarkGuestsReminded(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, id := range ids {
		_, _ = s.db.Exec("UPDATE rsvps SET last_reminded_at = ? WHERE id = ?", now, id)
	}
	return nil
}

// GetRSVPStats returns aggregate numbers instantly from in-memory atomic cache
func (s *DB) GetRSVPStats() (RSVPStats, error) {
	attCount := int(s.cachedAttendingCount.Load())
	uploaded := int(s.cachedUploadedCount.Load())
	pending := attCount - uploaded
	if pending < 0 {
		pending = 0
	}

	return RSVPStats{
		TotalResponses: int(s.cachedTotalResponses.Load()),
		TotalAttending: int(s.cachedTotalAttending.Load()),
		TotalDeclined:  int(s.cachedTotalDeclined.Load()),
		AttendingCount: attCount,
		UploadedCount:  uploaded,
		PendingUploads: pending,
	}, nil
}

// DeleteRSVP removes an RSVP by ID and recalculates cache
func (s *DB) DeleteRSVP(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM rsvps WHERE id = ?", id)
	if err == nil {
		s.warmupCache()
	}
	return err
}

// SavePushSubscription saves or updates a Web Push subscriber
func (s *DB) SavePushSubscription(sub PushSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT INTO subscriptions (id, endpoint, p256dh, auth, guest_name, created_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(endpoint) DO UPDATE SET
		p256dh = excluded.p256dh,
		auth = excluded.auth,
		guest_name = COALESCE(NULLIF(excluded.guest_name, ''), subscriptions.guest_name);
	`
	_, err := s.db.Exec(query, sub.ID, sub.Endpoint, sub.P256dh, sub.Auth, sub.GuestName, sub.CreatedAt)
	return err
}

// GetPushSubscriptions returns all registered push subscribers
func (s *DB) GetPushSubscriptions() ([]PushSubscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, endpoint, p256dh, auth, guest_name, created_at FROM subscriptions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PushSubscription
	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.ID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &sub.GuestName, &sub.CreatedAt); err == nil {
			list = append(list, sub)
		}
	}
	return list, nil
}

// DeletePushSubscription removes an endpoint that expired or failed
func (s *DB) DeletePushSubscription(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM subscriptions WHERE endpoint = ?", endpoint)
	return err
}

// RecordNotificationLog saves a history entry of dispatched notifications
func (s *DB) RecordNotificationLog(logEntry NotificationLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT INTO notifications (id, title, message, type, sent_count, skipped_count, sent_at)
	VALUES (?, ?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.Exec(query, logEntry.ID, logEntry.Title, logEntry.Message, logEntry.Type, logEntry.SentCount, logEntry.SkippedCount, logEntry.SentAt)
	return err
}

// GetNotificationLogs returns history of all sent broadcasts and reminders
func (s *DB) GetNotificationLogs() ([]NotificationLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, title, message, type, sent_count, skipped_count, sent_at FROM notifications ORDER BY sent_at DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []NotificationLog
	for rows.Next() {
		var n NotificationLog
		if err := rows.Scan(&n.ID, &n.Title, &n.Message, &n.Type, &n.SentCount, &n.SkippedCount, &n.SentAt); err == nil {
			list = append(list, n)
		}
	}
	return list, nil
}

// ExportRSVPsCSV generates a clean CSV with UTF-8 BOM for Microsoft Excel
func (s *DB) ExportRSVPsCSV() (string, error) {
	rsvps, err := s.GetRSVPs()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("\xEF\xBB\xBF") // UTF-8 BOM for Excel

	writer := csv.NewWriter(&b)
	_ = writer.Write([]string{
		"Submission Date",
		"Guest Name",
		"Status",
		"Total Seats",
		"Additional Guests",
		"Phone",
		"Email",
		"Photos Uploaded",
		"Song Request",
		"Message / Note",
	})

	for _, r := range rsvps {
		status := "Attending"
		if !r.Attending {
			status = "Regretfully Declining"
		}
		uploadedStatus := "No"
		if r.HasUploaded {
			uploadedStatus = fmt.Sprintf("Yes (%d photos)", r.UploadCount)
		}
		_ = writer.Write([]string{
			r.SubmittedTime.Format("2006-01-02 15:04"),
			r.Name,
			status,
			fmt.Sprintf("%d", r.GuestCount),
			r.AdditionalGuests,
			r.Phone,
			r.Email,
			uploadedStatus,
			r.SongRequest,
			r.Message,
		})
	}

	writer.Flush()
	return b.String(), nil
}

// Close closes the database connection cleanly
func (s *DB) Close() error {
	close(s.rsvpQueue)
	return s.db.Close()
}
