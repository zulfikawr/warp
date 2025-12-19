package server

import (
	"strings"
	"testing"
)

// FuzzSanitizeFilename tests filename sanitization with random inputs
func FuzzSanitizeFilename(f *testing.F) {
	// Seed corpus with interesting test cases
	f.Add("normal.txt")
	f.Add("../../../etc/passwd")
	f.Add("file\x00name.txt")
	f.Add(".")
	f.Add("..")
	f.Add(strings.Repeat("a", 300))
	f.Add("file/with/path.txt")
	f.Add("file\\windows\\path.txt")
	f.Add("   ")
	f.Add("\r\n\t")
	f.Add("file\x01\x02\x03.txt")
	f.Add("CON") // Windows reserved name
	f.Add("PRN")
	f.Add("AUX")
	f.Add("NUL")

	f.Fuzz(func(t *testing.T, input string) {
		result, err := sanitizeFilename(input)

		if err == nil {
			// If accepted, verify it's safe
			if strings.Contains(result, "..") {
				t.Errorf("Accepted directory traversal: input=%q, result=%q", input, result)
			}
			if strings.ContainsAny(result, "/\\") {
				t.Errorf("Accepted path separator: input=%q, result=%q", input, result)
			}
			if strings.Contains(result, "\x00") {
				t.Errorf("Accepted null byte: input=%q, result=%q", input, result)
			}
			if len(result) > 255 {
				t.Errorf("Accepted overlong filename: input=%q, result=%q (len=%d)", input, result, len(result))
			}
			if result == "" || result == "." || result == ".." {
				t.Errorf("Accepted dangerous name: input=%q, result=%q", input, result)
			}
			// Check for control characters
			for _, r := range result {
				if r < 32 || r == 0x7F {
					t.Errorf("Accepted control character: input=%q, result=%q", input, result)
					break
				}
			}
			// Verify result is only whitespace
			if strings.TrimSpace(result) == "" {
				t.Errorf("Accepted whitespace-only filename: input=%q, result=%q", input, result)
			}
		} else {
			// If rejected, error message should be informative
			if err.Error() == "" {
				t.Errorf("Empty error message for input=%q", input)
			}
		}
	})
}

// TestSanitizeFilename_KnownGood tests valid filenames that should pass
func TestSanitizeFilename_KnownGood(t *testing.T) {
	validNames := []string{
		"file.txt",
		"document.pdf",
		"my-file_123.jpg",
		"README.md",
		"config.yaml",
		"2024-12-19.log",
		"file with spaces.txt",
		"αβγδε.txt", // Unicode
		"文件.txt",    // Chinese
		"файл.txt",  // Cyrillic
	}

	for _, name := range validNames {
		result, err := sanitizeFilename(name)
		if err != nil {
			t.Errorf("Rejected valid filename %q: %v", name, err)
		}
		if result != name {
			t.Errorf("Modified valid filename: input=%q, output=%q", name, result)
		}
	}
}

// TestSanitizeFilename_KnownBad tests invalid filenames that should fail
func TestSanitizeFilename_KnownBad(t *testing.T) {
	invalidNames := map[string]string{
		"":                       "empty",
		".":                      "dot",
		"..":                     "dotdot",
		"../etc/passwd":          "traversal",
		"../../file.txt":         "traversal",
		"/etc/passwd":            "absolute path",
		"C:\\Windows\\file.txt":  "windows path",
		"file/with/slash.txt":    "slash",
		"file\\with\\back.txt":   "backslash",
		"file\x00name.txt":       "null byte",
		"file\x01name.txt":       "control char",
		"   ":                    "whitespace only",
		"\t\n\r":                 "whitespace only",
		strings.Repeat("a", 256): "too long",
		strings.Repeat("x", 500): "way too long",
	}

	for name, reason := range invalidNames {
		result, err := sanitizeFilename(name)
		if err == nil {
			t.Errorf("Accepted invalid filename (%s): input=%q, output=%q", reason, name, result)
		}
	}
}

// TestSanitizeFilename_Normalization tests normalization attack detection
func TestSanitizeFilename_Normalization(t *testing.T) {
	// These should fail because they change after normalization
	attackNames := []string{
		"file/./name.txt",
		"file/../name.txt",
		"./file.txt",
		"../file.txt",
	}

	for _, name := range attackNames {
		result, err := sanitizeFilename(name)
		if err == nil {
			t.Errorf("Accepted normalization attack: input=%q, output=%q", name, result)
		}
	}
}
