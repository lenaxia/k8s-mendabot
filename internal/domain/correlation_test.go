package domain

import (
	"testing"
)

func TestNewCorrelationGroupID_Length(t *testing.T) {
	id := NewCorrelationGroupID()
	if len(id) != 12 {
		t.Errorf("NewCorrelationGroupID: expected 12 chars, got %d (%q)", len(id), id)
	}
}

func TestNewCorrelationGroupID_HexChars(t *testing.T) {
	id := NewCorrelationGroupID()
	for i, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("NewCorrelationGroupID: non-hex character %q at position %d in %q", c, i, id)
		}
	}
}

func TestNewCorrelationGroupID_Unique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id := NewCorrelationGroupID()
		if _, exists := seen[id]; exists {
			t.Errorf("NewCorrelationGroupID: duplicate ID %q after %d iterations", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestCorrelationConstants(t *testing.T) {
	if CorrelationGroupIDLabel == "" {
		t.Error("CorrelationGroupIDLabel must not be empty")
	}
	if CorrelationGroupRoleLabel == "" {
		t.Error("CorrelationGroupRoleLabel must not be empty")
	}
	if CorrelationRolePrimary == "" {
		t.Error("CorrelationRolePrimary must not be empty")
	}
	if CorrelationRoleCorrelated == "" {
		t.Error("CorrelationRoleCorrelated must not be empty")
	}
	if NodeNameAnnotation == "" {
		t.Error("NodeNameAnnotation must not be empty")
	}
}
