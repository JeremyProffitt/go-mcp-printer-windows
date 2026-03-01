package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"off", LevelOff},
		{"OFF", LevelOff},
		{"error", LevelError},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"info", LevelInfo},
		{"access", LevelAccess},
		{"debug", LevelDebug},
		{"unknown", LevelInfo},
	}

	for _, tt := range tests {
		got := ParseLogLevel(tt.input)
		if got != tt.expected {
			t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelOff, "OFF"},
		{LevelError, "ERROR"},
		{LevelWarn, "WARN"},
		{LevelInfo, "INFO"},
		{LevelAccess, "ACCESS"},
		{LevelDebug, "DEBUG"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("LogLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
		}
	}
}

func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()
	l, err := NewLogger(Config{
		LogDir:  tmpDir,
		AppName: "test",
		Level:   LevelDebug,
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer l.Close()

	l.Info("test message")

	// Verify log file was created
	matches, err := filepath.Glob(filepath.Join(tmpDir, "test-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Error("Expected log file to be created")
	}

	// Read and verify content
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test message") {
		t.Error("Expected log file to contain 'test message'")
	}
}

func TestLogLevelFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	l, err := NewLogger(Config{
		LogDir:  tmpDir,
		AppName: "test",
		Level:   LevelError,
	})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer l.Close()

	var buf bytes.Buffer
	l.SetOutput(&buf)

	l.Debug("should not appear")
	l.Info("should not appear")
	l.Warn("should not appear")
	l.Error("should appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Error("Log level filtering failed: lower-level messages were logged")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("Error message should have been logged")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"ab", "xxxab"},
		{"abcdefgh", "xxxefgh"},
		{"1234567890", "xxx7890"},
	}

	for _, tt := range tests {
		got := MaskSecret(tt.input)
		if got != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSanitizePII(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{`{"access_token": "mysecrettoken123"}`, "xxx"},
		{`{"password": "hunter2"}`, "xxx"},
	}

	for _, tt := range tests {
		got := SanitizePII(tt.input)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("SanitizePII(%q) = %q, expected to contain %q", tt.input, got, tt.contains)
		}
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`C:\Users\John\Documents\secret.pdf`, "secret.pdf"},
		{"/home/user/file.txt", "file.txt"},
		{"file.txt", "file.txt"},
	}

	for _, tt := range tests {
		got := SanitizePath(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
