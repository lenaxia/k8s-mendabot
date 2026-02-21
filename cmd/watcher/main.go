package main

// Version is embedded at build time via ldflags:
//
//	-X main.Version=sha-<commit>
//
// It defaults to "dev" for local builds.
var Version = "dev"

func main() {
}
