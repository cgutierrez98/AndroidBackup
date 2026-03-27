package sorter

import (
	"AndroidSafeLocal/internal/device"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// DocumentExtensions is the set of file extensions considered documents.
var DocumentExtensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".txt": true, ".rtf": true, ".odt": true,
	".ods": true, ".odp": true, ".csv": true, ".md": true,
}

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

// exifExtensions is the set of image formats that commonly embed EXIF metadata.
var exifExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".heic": true, ".heif": true,
	".tiff": true, ".tif": true,
}

// GetDestinationWithEXIF returns the relative destination path using EXIF
// DateTimeOriginal when available, falling back to GetDestination otherwise.
// localPath is the already-downloaded copy of the file on the local disk.
func (s *Sorter) GetDestinationWithEXIF(file device.File, localPath string) string {
	ext := strings.ToLower(filepath.Ext(file.Path))
	if exifExtensions[ext] {
		if year, month, ok := exifDate(localPath); ok {
			return filepath.Join(year, month, filepath.Base(file.Path))
		}
	}
	return s.GetDestination(file)
}

// exifDate reads the EXIF DateTimeOriginal from a local image file.
func exifDate(localPath string) (year, month string, ok bool) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", "", false
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return "", "", false
	}
	dt, err := x.DateTime()
	if err != nil {
		return "", "", false
	}
	return fmt.Sprintf("%d", dt.Year()), fmt.Sprintf("%02d", int(dt.Month())), true
}

// IsDocument returns true if filePath has a document extension.
func IsDocument(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return DocumentExtensions[ext]
}

// GetDocumentDestination returns the backup destination for a document file.
// Structure: Documents/<path-relative-to-sourceRoot>
// This preserves directory structure and avoids collisions across folders.
func (s *Sorter) GetDocumentDestination(file device.File, sourceRoot string) string {
	src := strings.TrimSuffix(sourceRoot, "/")
	if strings.HasPrefix(file.Path, src+"/") {
		rel := file.Path[len(src)+1:]
		return filepath.Join("Documents", filepath.FromSlash(rel))
	}
	return filepath.Join("Documents", filepath.Base(file.Path))
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
