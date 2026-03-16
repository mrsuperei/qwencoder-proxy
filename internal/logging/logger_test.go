package logging

import (
	"testing"
)

func TestLogger(t *testing.T) {
	logger := NewLogger()

	// Test that we can create a logger
	if logger == nil {
		t.Error("Failed to create logger")
	}

	// Test logging functions (these should not panic)
	// Only test methods defined in the Logger interface
	logger.InfoLog("Test info log")
	logger.DebugLog("Test debug log")
	logger.ErrorLog("Test error log")
	logger.WarnLog("Test warning log")
}
