package logger

import (
	"os"
	"strings"
	"testing"
)

// TestNewLogger ensures the log file is created where it should be.
func TestNewLogger(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	dir := t.TempDir()

	// Initialise the logger with an unwritable log file.
	unwritableDir := dir + "/unwritable"
	// Make the dir unwritable.
	err := os.Mkdir(unwritableDir, 0400)
	if err != nil {
		t.Fatal(err)
	}
	unwritableLog := unwritableDir + "/pinner.log"
	_, _, err = NewLogger("trace", unwritableLog)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("Expected 'permission denied', got '%s'", err)
	}

	// Initialise the logger with a writable log file.
	writableLog := dir + "/pinner.log"
	_, _, err = NewLogger("trace", writableLog)
	if err != nil {
		t.Fatal(err)
	}
	// Ensure the log file is created.
	f, err := os.Open(writableLog)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}
}
