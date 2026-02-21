package logging_test

import (
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/logging"
)

func TestNew_ValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := logging.New(level)
			if err != nil {
				t.Fatalf("New(%q) unexpected error: %v", level, err)
			}
			if logger == nil {
				t.Fatalf("New(%q) returned nil logger", level)
			}
			logger.Sync() //nolint:errcheck
		})
	}
}

func TestNew_InvalidLevel(t *testing.T) {
	_, err := logging.New("verbose")
	if err == nil {
		t.Fatal("New(\"verbose\") expected error, got nil")
	}
}

func TestNew_EmptyLevel(t *testing.T) {
	_, err := logging.New("")
	if err == nil {
		t.Fatal("New(\"\") expected error, got nil")
	}
}
