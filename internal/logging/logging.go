package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New constructs a production zap logger at the given level.
// Valid levels: "debug", "info", "warn", "error".
// Returns an error for any unrecognised level string.
func New(level string) (*zap.Logger, error) {
	if level == "" {
		return nil, fmt.Errorf("logging.New: log level must not be empty")
	}
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("logging.New: invalid log level %q: %w", level, err)
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("logging.New: build logger: %w", err)
	}
	return logger, nil
}
