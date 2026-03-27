package dedup

import (
	"AndroidSafeLocal/internal/device"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Registry tracks files that have already been backed up
type Registry struct {
	// Map key: "Filename|Size" (fast pre-check based on filesystem walk)
	files map[string]bool
	// hashes: xxHash hex strings from verified transfers (stronger identity)
	hashes map[string]bool
	mu     sync.RWMutex
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	return &Registry{
		files:  make(map[string]bool),
		hashes: make(map[string]bool),
	}
}

// Load scans the local backup directory and populates the registry
func (r *Registry) Load(rootPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable
		}
		if info.IsDir() {
			return nil
		}

		// Key: Filename + Size.
		// Timestamp is tricky because Android fs time vs Windows fs time might drift or be set differently.
		// Filename + Size is a strong enough heuristic for personal photos.
		key := makeKey(info.Name(), info.Size())
		r.files[key] = true
		return nil
	})
}

// Exists checks if a file is already in the registry
func (r *Registry) Exists(file device.File) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// extracting basic name from path
	name := filepath.Base(file.Path)
	key := makeKey(name, file.Size)
	return r.files[key]
}

// Add adds a file to the registry (after successful download)
func (r *Registry) Add(file device.File) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := filepath.Base(file.Path)
	key := makeKey(name, file.Size)
	r.files[key] = true
}

func makeKey(name string, size int64) string {
	// "IMG_2024.jpg|1024"
	return fmt.Sprintf("%s|%d", name, size)
}

// AddByHash records a verified xxHash so future runs can detect already-transferred files.
func (r *Registry) AddByHash(hash string) {
	if hash == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hashes[hash] = true
}

// LoadHashes seeds the registry with a set of previously stored hashes
// (typically loaded from the manifest's HashSet).
func (r *Registry) LoadHashes(hs map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for h := range hs {
		r.hashes[h] = true
	}
}
