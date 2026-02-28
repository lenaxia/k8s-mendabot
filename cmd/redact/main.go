package main

import (
	"io"
	"os"

	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

func run(r io.Reader, w io.Writer) error {
	input, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, domain.RedactSecrets(string(input)))
	return err
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
}
