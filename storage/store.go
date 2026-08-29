package storage

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PhotoMetadata represents indexed information about an uploaded photo.
type PhotoMetadata struct {
	ID           string    `json:"id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	GuestName    string    `json:"guest_name"`
	Wish         string    `json:"wish"`
	UploadTime   time.Time `json:"upload_time"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	URL          string    `json:"url"`
	Favorite     bool      `json:"favorite"`
}

// Stats represents overall upload metrics.
type Stats struct {
	TotalPhotos    int64 `json:"total_photos"`
	TotalBytes     int64 `json:"total_bytes"`
	TotalGuests    int   `json:"total_guests"`
	FavoritePhotos int   `json:"favorite_photos"`
}

// Store handles thread-safe persistence of photo files and metadata.
type Store struct {
	uploadDir    string
	dataFile     string
	mu           sync.RWMutex
	photos       []PhotoMetadata
	diskSem      chan struct{} // limits concurrent disk I/O to prevent thrashing
	onNewPhoto   []chan PhotoMetadata
	listenerLock sync.Mutex
}

// NewStore initializes directories, loads existing metadata, and sets up concurrency bounds.
func NewStore(uploadDir, dataFile string, maxConcurrentDiskWrites int) (*Store, error) {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload dir: %w", err)
	}

	dataDir := filepath.Dir(dataFile)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	if maxConcurrentDiskWrites <= 0 {
		maxConcurrentDiskWrites = 64
	}

	s := &Store{
		uploadDir: uploadDir,
		dataFile:  dataFile,
		diskSem:   make(chan struct{}, maxConcurrentDiskWrites),
		photos:    make([]PhotoMetadata, 0),
	}

	// Load existing metadata if available
	if data, err := os.ReadFile(dataFile); err == nil {
		var loaded []PhotoMetadata
		if err := json.Unmarshal(data, &loaded); err == nil {
			s.photos = loaded
		}
	}

	return s, nil
}

// Subscribe returns a channel that receives newly uploaded photo metadata in real-time.
func (s *Store) Subscribe() chan PhotoMetadata {
	s.listenerLock.Lock()
	defer s.listenerLock.Unlock()

	ch := make(chan PhotoMetadata, 100)
	s.onNewPhoto = append(s.onNewPhoto, ch)
	return ch
}

// Unsubscribe removes a real-time listener.
func (s *Store) Unsubscribe(ch chan PhotoMetadata) {
	s.listenerLock.Lock()
	defer s.listenerLock.Unlock()

	for i, listener := range s.onNewPhoto {
		if listener == ch {
			s.onNewPhoto = append(s.onNewPhoto[:i], s.onNewPhoto[i+1:]...)
			close(ch)
			break
		}
	}
}

func (s *Store) broadcast(photo PhotoMetadata) {
	s.listenerLock.Lock()
	defer s.listenerLock.Unlock()

	for _, ch := range s.onNewPhoto {
		select {
		case ch <- photo:
		default:
			// Non-blocking if channel full
		}
	}
}

// SaveUploadedFile streams a multipart file header to disk with concurrency control and records metadata.
func (s *Store) SaveUploadedFile(fileHeader *multipart.FileHeader, guestName, wish string) (*PhotoMetadata, error) {
	// Acquire disk write semaphore
	s.diskSem <- struct{}{}
	defer func() { <-s.diskSem }()

	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open multipart file: %w", err)
	}
	defer src.Close()

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext == "" || (!strings.HasSuffix(ext, ".jpg") && !strings.HasSuffix(ext, ".jpeg") &&
		!strings.HasSuffix(ext, ".png") && !strings.HasSuffix(ext, ".webp") &&
		!strings.HasSuffix(ext, ".heic") && !strings.HasSuffix(ext, ".gif")) {
		ext = ".jpg"
	}

	id := uuid.New().String()
	timestamp := time.Now().Format("20060102_150405")
	cleanGuest := sanitizeString(guestName)
	if cleanGuest == "" {
		cleanGuest = "guest"
	}

	diskFilename := fmt.Sprintf("%s_%s_%s%s", timestamp, cleanGuest, id[:8], ext)
	diskPath := filepath.Join(s.uploadDir, diskFilename)

	dst, err := os.Create(diskPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// High speed buffered copy
	bufWriter := bufio.NewWriterSize(dst, 64*1024)
	written, err := io.Copy(bufWriter, src)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file payload: %w", err)
	}
	if err := bufWriter.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush buffer: %w", err)
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	meta := PhotoMetadata{
		ID:           id,
		Filename:     diskFilename,
		OriginalName: fileHeader.Filename,
		GuestName:    guestName,
		Wish:         wish,
		UploadTime:   time.Now(),
		Size:         written,
		ContentType:  contentType,
		URL:          "/uploads/" + diskFilename,
		Favorite:     false,
	}

	s.mu.Lock()
	s.photos = append([]PhotoMetadata{meta}, s.photos...) // prepend to keep newest first
	s.saveMetadataLocked()
	s.mu.Unlock()

	s.broadcast(meta)
	return &meta, nil
}

// GetPhotos returns paginated photos (newest first).
func (s *Store) GetPhotos(limit, offset int) ([]PhotoMetadata, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.photos)
	if offset >= total {
		return []PhotoMetadata{}, total
	}

	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}

	res := make([]PhotoMetadata, end-offset)
	copy(res, s.photos[offset:end])
	return res, total
}

// GetAllPhotos returns all stored photo metadata.
func (s *Store) GetAllPhotos() []PhotoMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]PhotoMetadata, len(s.photos))
	copy(res, s.photos)
	return res
}

// DeletePhoto removes the photo from metadata and deletes the physical file.
func (s *Store) DeletePhoto(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	var targetFile string
	for i, p := range s.photos {
		if p.ID == id {
			idx = i
			targetFile = p.Filename
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("photo with id %s not found", id)
	}

	// Remove from slice
	s.photos = append(s.photos[:idx], s.photos[idx+1:]...)
	s.saveMetadataLocked()

	// Delete from disk
	if targetFile != "" {
		filePath := filepath.Join(s.uploadDir, targetFile)
		_ = os.Remove(filePath)
	}

	return nil
}

// ToggleFavorite toggles the favorite/featured flag on a photo.
func (s *Store) ToggleFavorite(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.photos {
		if s.photos[i].ID == id {
			s.photos[i].Favorite = !s.photos[i].Favorite
			val := s.photos[i].Favorite
			s.saveMetadataLocked()
			return val, nil
		}
	}

	return false, fmt.Errorf("photo not found")
}

// UpdatePhoto edits guest name and/or wish on an existing photo.
func (s *Store) UpdatePhoto(id, guestName, wish string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.photos {
		if s.photos[i].ID == id {
			s.photos[i].GuestName = guestName
			s.photos[i].Wish = wish
			s.saveMetadataLocked()
			return nil
		}
	}

	return fmt.Errorf("photo not found")
}

// ClearAllPhotos removes all uploaded files and clears metadata (for pre-wedding testing reset).
func (s *Store) ClearAllPhotos() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove all files in uploadDir
	entries, err := os.ReadDir(s.uploadDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				_ = os.Remove(filepath.Join(s.uploadDir, entry.Name()))
			}
		}
	}

	s.photos = make([]PhotoMetadata, 0)
	s.saveMetadataLocked()
	return nil
}

// StreamAllPhotosZip streams a ZIP archive containing all photos to the provided writer.
func (s *Store) StreamAllPhotosZip(w io.Writer) error {
	s.mu.RLock()
	photosCopy := make([]PhotoMetadata, len(s.photos))
	copy(photosCopy, s.photos)
	s.mu.RUnlock()

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	for _, p := range photosCopy {
		filePath := filepath.Join(s.uploadDir, p.Filename)
		file, err := os.Open(filePath)
		if err != nil {
			continue // Skip missing files gracefully
		}

		cleanGuest := sanitizeString(p.GuestName)
		if cleanGuest == "" {
			cleanGuest = "Guest"
		}
		zipEntryName := fmt.Sprintf("%s_%s", cleanGuest, p.Filename)

		header := &zip.FileHeader{
			Name:     zipEntryName,
			Method:   zip.Store, // Images are already compressed; storing is faster
			Modified: p.UploadTime,
		}

		entryWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			file.Close()
			continue
		}

		_, _ = io.Copy(entryWriter, file)
		file.Close()
	}

	return nil
}

// GetStats returns current counts and storage totals.
func (s *Store) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	guests := make(map[string]bool)
	var totalBytes int64
	var favorites int
	for _, p := range s.photos {
		totalBytes += p.Size
		if p.Favorite {
			favorites++
		}
		if p.GuestName != "" {
			guests[strings.ToLower(strings.TrimSpace(p.GuestName))] = true
		}
	}

	return Stats{
		TotalPhotos:    int64(len(s.photos)),
		TotalBytes:     totalBytes,
		TotalGuests:    len(guests),
		FavoritePhotos: favorites,
	}
}

func (s *Store) saveMetadataLocked() {
	data, err := json.MarshalIndent(s.photos, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.dataFile, data, 0644)
}

func sanitizeString(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('_')
		}
	}
	res := b.String()
	if len(res) > 20 {
		return res[:20]
	}
	return res
}
