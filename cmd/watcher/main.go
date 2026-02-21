package main

import (
	"log"

	"github.com/lenaxia/k8s-mendabot/internal/config"
	"github.com/lenaxia/k8s-mendabot/internal/logging"
)

// Version is embedded at build time via ldflags:
//
//	-X main.Version=sha-<commit>
//
// It defaults to "dev" for local builds.
var Version = "dev"

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		log.Fatalf("logger init failed: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	_ = logger
}
