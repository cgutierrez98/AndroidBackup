package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Entry represents a single backed-up file's metadata
type Entry struct {
	OriginalPath string `json:"original_path"` // Path on the Android device
	LocalPath    string `json:"local_path"`    // Relative path in backup folder
	Size         int64  `json:"size"`
	Timestamp    string `json:"timestamp"`
}

// Manifest holds all backup entries
type Manifest struct {
	Entries []Entry `json:"entries"`
	mu      sync.Mutex
}

// New creates a new empty manifest
func New() *Manifest {
	return &Manifest{
		Entries: []Entry{},
	}
}

// Add appends an entry to the manifest (thread-safe)
func (m *Manifest) Add(original, local string, size int64, timestamp string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Entries = append(m.Entries, Entry{
		OriginalPath: original,
		LocalPath:    local,
		Size:         size,
		Timestamp:    timestamp,
	})
}

// Save writes the manifest to a JSON file
func (m *Manifest) Save(backupRoot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(backupRoot, "manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads a manifest from a backup folder
func Load(backupRoot string) (*Manifest, error) {
	path := filepath.Join(backupRoot, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
