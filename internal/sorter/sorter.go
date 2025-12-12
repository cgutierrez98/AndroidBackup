package sorter

import (
	"AndroidSafeLocal/internal/device"
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

// Sorter determines the destination path for a file
type Sorter struct{}

var (
	// YYYYMMDD
	regexYMDCompact = regexp.MustCompile(`(20\d{2})(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])`)
	// YYYY-MM-DD or YYYY_MM_DD
	regexYMDSeperated = regexp.MustCompile(`(20\d{2})[-_](0[1-9]|1[0-2])[-_](0[1-9]|[12]\d|3[01])`)
)

// NewSorter creates a new Sorter
func NewSorter() *Sorter {
	return &Sorter{}
}

// GetDestination returns the relative destination path (Year/Month/Filename)
func (s *Sorter) GetDestination(file device.File) string {
	fileName := filepath.Base(file.Path)
	year, month := s.extractDate(fileName)

	// Fallback to file timestamp if filename parsing failed
	if year == "" {
		// Timestamp format from walker: "2024-01-01 10:00"
		// Parse it
		t, err := time.Parse("2006-01-02 15:04", file.Timestamp)
		if err == nil {
			year = fmt.Sprintf("%d", t.Year())
			month = fmt.Sprintf("%02d", t.Month())
		} else {
			// Ultimate fallback
			year = "Unknown_Date"
			month = "Misc"
		}
	}

	return filepath.Join(year, month, fileName)
}

func (s *Sorter) extractDate(filename string) (string, string) { // year, month
	// Check standard formats
	// IMG_20240101_...
	// VID-20240101-...

	// Try separated first (2024-01-01)
	if matches := regexYMDSeperated.FindStringSubmatch(filename); len(matches) > 3 {
		return matches[1], matches[2]
	}

	// Try compact (20240101)
	if matches := regexYMDCompact.FindStringSubmatch(filename); len(matches) > 3 {
		return matches[1], matches[2]
	}

	return "", ""
}
