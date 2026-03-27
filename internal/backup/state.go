package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateFileName = "backup-state.json"

// BackupState tracks which source paths have been successfully transferred.
// Written periodically during a backup so it can be resumed after a disconnect.
type BackupState struct {
	// CompletedPaths is the set of device source paths already transferred.
	CompletedPaths map[string]bool `json:"completed_paths"`
}

// StateExists returns true if a resume state file exists in destRoot.
func StateExists(destRoot string) bool {
	_, err := os.Stat(filepath.Join(destRoot, stateFileName))
	return err == nil
}

// LoadState reads the state file from destRoot.
func LoadState(destRoot string) (*BackupState, error) {
	data, err := os.ReadFile(filepath.Join(destRoot, stateFileName))
	if err != nil {
		return nil, err
	}
	var s BackupState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.CompletedPaths == nil {
		s.CompletedPaths = make(map[string]bool)
	}
	return &s, nil
}

// Save writes the state file atomically (write to temp, rename).
func (s *BackupState) Save(destRoot string) error {
	if err := os.MkdirAll(destRoot, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(destRoot, stateFileName)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// ClearState deletes the state file from destRoot (called on successful backup completion).
func ClearState(destRoot string) error {
	err := os.Remove(filepath.Join(destRoot, stateFileName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// NewBackupState creates an empty state.
func NewBackupState() *BackupState {
	return &BackupState{CompletedPaths: make(map[string]bool)}
}
