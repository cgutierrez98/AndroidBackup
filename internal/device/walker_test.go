package device

import (
	"testing"
)

func TestParseLsR(t *testing.T) {
	// Sample output from 'adb shell ls -R -l /sdcard/Photos'
	// Note: Android toybox output
	output := `/sdcard/Photos:
total 16
drwxrwx--x 3 root sdcard_rw 4096 2024-01-01 10:00 .
drwxrwx--x 3 root sdcard_rw 4096 2024-01-01 10:00 ..
drwxrwx--x 2 root sdcard_rw 4096 2024-01-01 10:05 Vacation
-rw-rw---- 1 root sdcard_rw 1234 2024-05-20 15:30 image1.jpg

/sdcard/Photos/Vacation:
total 8
-rw-rw---- 1 root sdcard_rw 5678 2024-06-01 09:00 beach.png
-rw-rw---- 1 root sdcard_rw 9999 2024-06-01 09:05 sunset with spaces.jpg
`

	files, err := parseLsR(output, "/sdcard/Photos") // root path arg usually matches first block
	if err != nil {
		t.Fatalf("parseLsR failed: %v", err)
	}

	expected := []File{
		{Path: "/sdcard/Photos/Vacation", Size: 4096, Timestamp: "2024-01-01 10:05", IsDir: true},
		{Path: "/sdcard/Photos/image1.jpg", Size: 1234, Timestamp: "2024-05-20 15:30", IsDir: false},
		{Path: "/sdcard/Photos/Vacation/beach.png", Size: 5678, Timestamp: "2024-06-01 09:00", IsDir: false},
		{Path: "/sdcard/Photos/Vacation/sunset with spaces.jpg", Size: 9999, Timestamp: "2024-06-01 09:05", IsDir: false},
	}

	// We might want to filter . and .. in the future, but current logic keeps them.
	// Actually, keeping them is noisy.
	// Let's check if the parser got them right.

	// For comparison, creating a map or iterating
	if len(files) != len(expected) {
		t.Errorf("Expected %d files, got %d", len(expected), len(files))
		for i, f := range files {
			t.Logf("Got[%d]: %+v", i, f)
		}
	}

	for i, f := range files {
		if i >= len(expected) {
			break
		}
		e := expected[i]
		if f.Path != e.Path || f.Size != e.Size || f.IsDir != e.IsDir {
			t.Errorf("Mismatch at index %d:\nGot: %+v\nWant: %+v", i, f, e)
		}
	}
}
