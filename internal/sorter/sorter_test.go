package sorter

import (
	"AndroidSafeLocal/internal/device"
	"path/filepath"
	"testing"
)

func TestGetDestination(t *testing.T) {
	s := NewSorter()

	tests := []struct {
		name     string
		file     device.File
		expected string
	}{
		{
			name: "Filename Compact",
			file: device.File{
				Path: "/sdcard/DCIM/IMG_20231225_120000.jpg",
			},
			expected: filepath.Join("2023", "12", "IMG_20231225_120000.jpg"),
		},
		{
			name: "Filename Separated",
			file: device.File{
				Path: "/sdcard/Documents/Report-2024-01-15.pdf",
			},
			expected: filepath.Join("2024", "01", "Report-2024-01-15.pdf"),
		},
		{
			name: "Filename Separated Underscore",
			file: device.File{
				Path: "/sdcard/Documents/Report_2024_02_15.pdf",
			},
			expected: filepath.Join("2024", "02", "Report_2024_02_15.pdf"),
		},
		{
			name: "Fallback Timestamp",
			file: device.File{
				Path:      "/sdcard/Other/random_file.txt",
				Timestamp: "2022-11-20 09:30",
			},
			expected: filepath.Join("2022", "11", "random_file.txt"),
		},
		{
			name: "Fallback Unknown",
			file: device.File{
				Path:      "/sdcard/Other/mystery.bin",
				Timestamp: "invalid-date",
			},
			expected: filepath.Join("Unknown_Date", "Misc", "mystery.bin"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.GetDestination(tt.file)
			if got != tt.expected {
				t.Errorf("GetDestination() = %v, want %v", got, tt.expected)
			}
		})
	}
}
