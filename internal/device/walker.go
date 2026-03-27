package device

import (
	"AndroidSafeLocal/internal/adb"
	"bufio"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// File represents a file on the Android device
type File struct {
	Path      string
	Size      int64
	Timestamp string
	IsDir     bool
}

// Walker handles file system traversal
type Walker struct {
	client *adb.Client
}

// reLsLine parses a single ls -l line; compiled once at package level for performance.
// Matches: perms links owner group size date time name
var reLsLine = regexp.MustCompile(`^([dl-][rwxst-]{9})\s+\d+\s+\S+\s+\S+\s+(\d+)\s+(20\d{2}-\d{2}-\d{2})\s+(\d{2}:\d{2})\s+(.+)$`)

// NewWalker creates a new Walker
func NewWalker(client *adb.Client) *Walker {
	return &Walker{client: client}
}

// Walk recursively lists files starting from rootPath using 'ls -R -l'
// This is more robust than 'find' on some minimalist Android shells for metadata.
func (w *Walker) Walk(rootPath string) ([]File, error) {
	// Execute ls -R -l.
	// -R: recursive
	// -l: long format (perms, user, group, size, date, time, name)
	// -n: numeric uid/gid (easier to parse, keeps column count consistent?) - standard Android ls often doesn't show user/group names anyway or shows 'root' 'sdcard_rw'.
	// Let's stick to 'ls -R -l'

	cmdOut, err := w.client.RunCommand("shell", "ls", "-R", "-l", rootPath)
	if err != nil {
		if cmdOut == "" {
			return nil, fmt.Errorf("failed to list files: %w", err)
		}
		// If we have output, it might be partial success (e.g. Permission Denied on some subdirs)
		// We'll proceed but maybe we should log it?
		// For now, we return valid files we found.
		// fmt.Printf("Warning: ls encountered errors but produced output: %v\n", err)
	}

	return parseLsR(cmdOut, rootPath)
}

func parseLsR(output string, rootPath string) ([]File, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))

	var files []File
	var currentDir string = rootPath

	// The first block in ls -R is usually the root dir contents, but sometimes it starts with "path:"
	// Android toybox ls -R output format:
	//
	// /sdcard/DCIM:
	// total 16
	// drwxrwx--x 3 root sdcard_rw 4096 2024-01-01 10:00 Camera
	// -rw-rw---- 1 root sdcard_rw  123 2024-01-01 10:00 file.txt
	//
	// /sdcard/DCIM/Camera:
	// ...

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Check if it's a directory header
		if strings.HasSuffix(line, ":") && strings.HasPrefix(line, "/") {
			currentDir = strings.TrimSuffix(line, ":")
			continue
		}

		if strings.HasPrefix(line, "total ") {
			continue
		}

		// Try to parse using regex for typical ls -l format to better handle filenames with spaces.
		// Example: -rw-rw---- 1 root sdcard_rw 1234 2024-05-20 15:30 image name.jpg
		if m := reLsLine.FindStringSubmatch(line); m != nil {
			perms := m[1]
			sizeStr := m[2]
			datePart := m[3]
			timePart := m[4]
			name := m[5]

			isDir := strings.HasPrefix(perms, "d")
			size, err := strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				// skip entries with malformed size
				continue
			}
			if name == "." || name == ".." {
				continue
			}
			fullPath := path.Join(currentDir, name)
			files = append(files, File{Path: fullPath, Size: size, Timestamp: datePart + " " + timePart, IsDir: isDir})
			continue
		}

		// Fallback heuristic (previous behavior), attempt to split and parse date token
		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}
		if !strings.HasPrefix(parts[0], "-") && !strings.HasPrefix(parts[0], "d") && !strings.HasPrefix(parts[0], "l") {
			continue
		}
		isDir := strings.HasPrefix(parts[0], "d")
		dateIdx := -1
		for i, p := range parts {
			if len(p) == 10 && strings.Count(p, "-") == 2 && p[0] >= '0' && p[0] <= '9' {
				dateIdx = i
				break
			}
		}
		if dateIdx == -1 || dateIdx+2 >= len(parts) {
			continue
		}
		sizeStr := parts[dateIdx-1]
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			continue
		}
		timeStr := parts[dateIdx] + " " + parts[dateIdx+1]
		nameParts := parts[dateIdx+2:]
		name := strings.Join(nameParts, " ")
		if name == "." || name == ".." {
			continue
		}
		fullPath := path.Join(currentDir, name)
		files = append(files, File{Path: fullPath, Size: size, Timestamp: timeStr, IsDir: isDir})
	}

	return files, nil
}
