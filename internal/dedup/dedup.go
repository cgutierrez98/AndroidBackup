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
	// Map key: "Filename|Size|Timestamp" (Simple unique key)
	// We use this to skip transfers if the file exists locally with same metadata.
	// Optimally, we'd check destination path, but if files are moved, this might miss them.
	// However, the requirement is "deduplication across different folders".
	// So we map hash -> []paths? Or just check if *any* copy exists?
	// If we find a match, we might skip downloading.

	// For "Smart Sync" (avoid re-download to same location), Path is part of key.
	// For "Global Dedup" (avoid download if ANY copy exists), we ignore path.

	// Let's implement Global Dedup based on Size + Name (weak) for now, or Size + Name + Date.
	files map[string]bool
	mu    sync.RWMutex
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	return &Registry{
		files: make(map[string]bool),
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
