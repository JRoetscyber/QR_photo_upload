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
	SubmittedTime    time.Time `json:"submitted_time"`
}

// RSVPStats represents summary counts for the couple
type RSVPStats struct {
	TotalResponses int `json:"total_responses"`
	TotalAttending int `json:"total_attending"` // Sum of all attending seats/guests
	TotalDeclined  int `json:"total_declined"`
	AttendingCount int `json:"attending_count"` // Number of positive RSVP submissions
}

// DB handles thread-safe SQLite operations with WAL mode
type DB struct {
	db        *sql.DB
	mu        sync.RWMutex
	rsvpQueue chan RSVP
}

// NewDB initializes the SQLite database at dbPath with WAL mode and background worker goroutines
func NewDB(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Optimize connection pool for concurrent Fiber handlers
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	s := &DB{
		db:        db,
		rsvpQueue: make(chan RSVP, 500),
	}

	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Start background worker goroutines for high-performance async processing
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
		submitted_time DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_rsvps_attending ON rsvps(attending);
	CREATE INDEX IF NOT EXISTS idx_rsvps_time ON rsvps(submitted_time DESC);
	`
	_, err := s.db.Exec(query)
	return err
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
	INSERT INTO rsvps (id, name, email, phone, attending, guest_count, additional_guests, song_request, message, submitted_time)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
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

// EnqueueRSVP pushes an RSVP into the high-speed goroutine worker queue
func (s *DB) EnqueueRSVP(r RSVP) {
	s.rsvpQueue <- r
}

// GetRSVPs returns all RSVPs ordered by newest first
func (s *DB) GetRSVPs() ([]RSVP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, name, email, phone, attending, guest_count, additional_guests, song_request, message, submitted_time
	FROM rsvps
	ORDER BY submitted_time DESC;
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RSVP
	for rows.Next() {
		var r RSVP
		var attendingInt int
		var subTime time.Time
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
			&subTime,
		); err != nil {
			continue
		}
		r.Attending = (attendingInt == 1)
		r.SubmittedTime = subTime
		list = append(list, r)
	}

	if list == nil {
		list = []RSVP{}
	}
	return list, nil
}

// GetRSVPStats returns aggregate numbers for confirmed attendees and regrets
func (s *DB) GetRSVPStats() (RSVPStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats RSVPStats

	// Total responses
	row := s.db.QueryRow("SELECT COUNT(*) FROM rsvps")
	_ = row.Scan(&stats.TotalResponses)

	// Attending submissions count and total guest seats
	rowAttending := s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(guest_count), 0) FROM rsvps WHERE attending = 1")
	_ = rowAttending.Scan(&stats.AttendingCount, &stats.TotalAttending)

	// Declined count
	rowDeclined := s.db.QueryRow("SELECT COUNT(*) FROM rsvps WHERE attending = 0")
	_ = rowDeclined.Scan(&stats.TotalDeclined)

	return stats, nil
}

// DeleteRSVP removes an RSVP by ID
func (s *DB) DeleteRSVP(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM rsvps WHERE id = ?", id)
	return err
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
		"Song Request",
		"Message / Note",
	})

	for _, r := range rsvps {
		status := "Attending"
		if !r.Attending {
			status = "Regretfully Declining"
		}
		_ = writer.Write([]string{
			r.SubmittedTime.Format("2006-01-02 15:04"),
			r.Name,
			status,
			fmt.Sprintf("%d", r.GuestCount),
			r.AdditionalGuests,
			r.Phone,
			r.Email,
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
