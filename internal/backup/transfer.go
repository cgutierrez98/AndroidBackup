package backup

import (
	"AndroidSafeLocal/internal/adb"
	"fmt"
	"os"
	"path/filepath"
)

// TransferAgent handles the actual file transfer
type TransferAgent struct {
	Client *adb.Client
}

// Process implements the Processor interface
func (ta *TransferAgent) Process(job Job) error {
	// Ensure destination directory exists
	dir := filepath.Dir(job.DestPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Run ADB Pull
	// adb pull <remote> <local>
	_, err := ta.Client.RunCommand("pull", job.SourcePath, job.DestPath)
	if err != nil {
		return fmt.Errorf("adb pull failed for %s: %w", job.SourcePath, err)
	}

	return nil
}
