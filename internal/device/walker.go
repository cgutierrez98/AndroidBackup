package device

import (
	"AndroidSafeLocal/internal/adb"
	"bufio"
	"fmt"
	"path"
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

		// Parse line
		// drwxrwx--x 3 root sdcard_rw 4096 2024-01-01 10:00 Camera
		// parts: [perms, links, user, group, size, date, time, name...]
		// On some android versions, links might be missing or user/group might be missing?
		// Toybox ls -l:
		// perms, links, owner, group, size, date, time, name

		parts := strings.Fields(line)

		// Heuristic parsing.
		// Perms always start with - or d or l
		if len(parts) < 6 {
			// Malformed or unknown line
			continue
		}

		if !strings.HasPrefix(parts[0], "-") && !strings.HasPrefix(parts[0], "d") && !strings.HasPrefix(parts[0], "l") {
			continue
		}

		isDir := strings.HasPrefix(parts[0], "d")

		// We need to find where the date/time starts to isolate the name.
		// Standard format: ... size date time name
		// Name is the LAST part, but can contain spaces.
		// Date/Time are usually fields -3 and -2 from the name?

		// Let's assume standard toybox columns.
		// 0: perms
		// 1: links? or user?
		// ...
		// We can look for the date pattern YYYY-MM-DD

		dateIdx := -1
		for i, p := range parts {
			// Check for YYYY-MM-DD
			if len(p) == 10 && strings.Count(p, "-") == 2 {
				// verify it looks like a date?
				// Must start with a digit (e.g. 2024...)
				if p[0] >= '0' && p[0] <= '9' {
					dateIdx = i
					break
				}
			}
		}

		if dateIdx == -1 {
			// Fallback: maybe format is different (e.g. older Android).
			// Let's just assume last parts are name.
			continue
		}

		// Assuming: ... size date time name
		// size is dateIdx - 1
		// time is dateIdx + 1
		// name starts at dateIdx + 2

		if dateIdx+2 >= len(parts) {
			continue
		}

		sizeStr := parts[dateIdx-1]
		size, _ := strconv.ParseInt(sizeStr, 10, 64)

		timeStr := parts[dateIdx] + " " + parts[dateIdx+1]

		nameParts := parts[dateIdx+2:]
		name := strings.Join(nameParts, " ")

		if name == "." || name == ".." {
			continue
		}

		// Full path
		fullPath := path.Join(currentDir, name)

		files = append(files, File{
			Path:      fullPath,
			Size:      size,
			Timestamp: timeStr,
			IsDir:     isDir,
		})
	}

	return files, nil
}
