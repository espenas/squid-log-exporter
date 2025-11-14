package position

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Position represents the current reading position in a log file
type Position struct {
	Filename    string    `json:"filename"`
	Position    int64     `json:"position"`
	Inode       uint64    `json:"inode"`
	LastUpdated time.Time `json:"last_updated"`
}

// Tracker manages the position tracking for log files
type Tracker struct {
	positionFile string
	position     Position
	mu           sync.RWMutex
}

// NewTracker creates a new position tracker
func NewTracker(positionFile string) *Tracker {
	return &Tracker{
		positionFile: positionFile,
	}
}

// Load reads the position from disk
func (t *Tracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := os.ReadFile(t.positionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read position file: %w", err)
	}

	if err := json.Unmarshal(data, &t.position); err != nil {
		return fmt.Errorf("failed to unmarshal position: %w", err)
	}

	return nil
}

// Save writes the current position to disk atomically
func (t *Tracker) Save(filename string, pos int64, inode uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.position = Position{
		Filename:    filename,
		Position:    pos,
		Inode:       inode,
		LastUpdated: time.Now(),
	}

	data, err := json.MarshalIndent(t.position, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal position: %w", err)
	}

	// Ensure target directory exists
	dir := filepath.Dir(t.positionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create position directory: %w", err)
	}

	// Write to temp file in same directory as destination (guaranteed atomic rename)
	tmpFile := t.positionFile + ".tmp"

	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp position file: %w", err)
	}

	// Atomic rename to final location
	if err := os.Rename(tmpFile, t.positionFile); err != nil {
		os.Remove(tmpFile) // Cleanup temp file on failure
		return fmt.Errorf("failed to rename position file: %w", err)
	}

	return nil
}

// GetPosition returns the last saved position and inode
func (t *Tracker) GetPosition() (int64, uint64) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.position.Position, t.position.Inode
}

// GetFileInode returns the current inode of a file
func GetFileInode(filename string) (uint64, error) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("failed to get file stat")
	}

	return stat.Ino, nil
}
