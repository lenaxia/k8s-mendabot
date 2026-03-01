package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

func run(r io.Reader, w io.Writer) error {
	var extras []string
	if raw := os.Getenv("EXTRA_REDACT_PATTERNS"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				extras = append(extras, p)
			}
		}
	}

	// Skip invalid patterns with a warning — never crash the agent Job.
	var validExtras []string
	for _, p := range extras {
		if _, err := regexp.Compile(p); err != nil {
			fmt.Fprintf(os.Stderr, "[redact] WARNING: skipping invalid pattern %q: %v\n", p, err)
			continue
		}
		validExtras = append(validExtras, p)
	}

	redactor, err := domain.New(validExtras)
	if err != nil {
		return fmt.Errorf("[redact] failed to build redactor: %w", err)
	}

	input, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, redactor.Redact(string(input)))
	return err
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[redact] ERROR: %v\n", err)
		os.Exit(1)
	}
}
