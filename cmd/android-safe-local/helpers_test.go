package main

import "testing"

// ---- parseExcludePatterns ----

func TestParseExcludePatterns(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{", ,  ", nil},
		{".tmp, .jpg", []string{".tmp", ".jpg"}},
		{"  .TMP , /sdcard/ ", []string{".tmp", "/sdcard/"}},
	}
	for _, c := range cases {
		got := parseExcludePatterns(c.raw)
		if len(got) != len(c.want) {
			t.Errorf("parseExcludePatterns(%q) len=%d, want %d (got %v)", c.raw, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseExcludePatterns(%q)[%d] = %q, want %q", c.raw, i, got[i], c.want[i])
			}
		}
	}
}

// ---- isExcluded — substring ----

func TestIsExcludedSubstring(t *testing.T) {
	patterns := []string{".thumbnails", "/android/data/"}
	tests := []struct {
		path    string
		want    bool
		comment string
	}{
		{"/sdcard/DCIM/.thumbnails/photo.jpg", true, ".thumbnails match"},
		{"/sdcard/Android/data/com.app/cache/file.jpg", true, "/Android/data/ match (case-insensitive)"},
		{"/sdcard/DCIM/photo.jpg", false, "no match"},
	}
	for _, tc := range tests {
		got := isExcluded(tc.path, patterns)
		if got != tc.want {
			t.Errorf("isExcluded(%q) = %v, want %v (%s)", tc.path, got, tc.want, tc.comment)
		}
	}
}

// ---- isExcluded — glob ----

func TestIsExcludedGlob(t *testing.T) {
	patterns := []string{"*.tmp", "thumb*"}
	tests := []struct {
		path    string
		want    bool
		comment string
	}{
		{"/sdcard/Download/session.tmp", true, "*.tmp glob on basename"},
		{"/sdcard/DCIM/thumbnail_001.jpg", true, "thumb* glob on basename"},
		{"/sdcard/DCIM/photo.jpg", false, "no glob match"},
		{"/sdcard/.tmp/photo.jpg", false, "glob matches basename not directory"},
	}
	for _, tc := range tests {
		got := isExcluded(tc.path, patterns)
		if got != tc.want {
			t.Errorf("isExcluded(%q) = %v, want %v (%s)", tc.path, got, tc.want, tc.comment)
		}
	}
}

// ---- shouldBackupFile ----

func TestShouldBackupFile(t *testing.T) {
	if !shouldBackupFile("/sdcard/DCIM/photo.jpg", false) {
		t.Error("expected .jpg to be backed up")
	}
	if shouldBackupFile("/sdcard/note.txt", false) {
		t.Error("expected .txt NOT backed up without includeDocs")
	}
	if !shouldBackupFile("/sdcard/note.txt", true) {
		t.Error("expected .txt backed up WITH includeDocs")
	}
	if shouldBackupFile("/sdcard/unknown.xyz", true) {
		t.Error("expected unknown extension not backed up")
	}
}

// ---- formatETA ----

func TestFormatETA(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{0, "0m00s"},
		{59, "0m59s"},
		{90, "1m30s"},
		{3600, "1h00m"},
		{3661, "1h01m"},
		{-5, "0m00s"},
	}
	for _, c := range cases {
		got := formatETA(c.secs)
		if got != c.want {
			t.Errorf("formatETA(%.0f) = %q, want %q", c.secs, got, c.want)
		}
	}
}
