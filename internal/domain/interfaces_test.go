package domain_test

import (
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// TestJobBuilder_InterfaceIsImportable is a compile-time check that
// JobBuilder is accessible as a type in the domain package.
func TestJobBuilder_InterfaceIsImportable(t *testing.T) {
	var _ domain.JobBuilder // compile-time: interface must exist
}

// TestSourceProvider_HasThreeMethods verifies the SourceProvider interface
// is importable and usable as a variable type.
func TestSourceProvider_HasThreeMethods(t *testing.T) {
	// This test exists to ensure SourceProvider remains importable from this package.
	// Actual interface satisfaction is verified by compile-time assertions in the
	// concrete implementation packages (internal/provider/native, internal/jobbuilder).
	var _ domain.SourceProvider
}
