package domain_test

import (
	"context"
	"testing"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	"github.com/lenaxia/k8s-mechanic/internal/domain"
)

// Compile-time interface satisfaction check.
var _ domain.SinkCloser = domain.NoopSinkCloser{}

func TestNoopSinkCloser_AlwaysNil(t *testing.T) {
	t.Parallel()
	closer := domain.NoopSinkCloser{}
	ctx := context.Background()

	tests := []struct {
		name string
		rjob *v1alpha1.RemediationJob
	}{
		{
			name: "empty SinkRef",
			rjob: &v1alpha1.RemediationJob{},
		},
		{
			name: "fully populated SinkRef",
			rjob: &v1alpha1.RemediationJob{
				Status: v1alpha1.RemediationJobStatus{
					SinkRef: v1alpha1.SinkRef{
						Type:   "pr",
						URL:    "https://github.com/org/repo/pull/42",
						Number: 42,
						Repo:   "org/repo",
					},
				},
			},
		},
		{
			name: "partial SinkRef (URL only)",
			rjob: &v1alpha1.RemediationJob{
				Status: v1alpha1.RemediationJobStatus{
					SinkRef: v1alpha1.SinkRef{
						URL: "https://github.com/org/repo/pull/99",
					},
				},
			},
		},
		{
			name: "nil rjob-like zero value",
			rjob: &v1alpha1.RemediationJob{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := closer.Close(ctx, tc.rjob, "some reason"); err != nil {
				t.Errorf("NoopSinkCloser.Close() returned non-nil error: %v", err)
			}
		})
	}
}

func TestNoopSinkCloser_NilContextOK(t *testing.T) {
	t.Parallel()
	// NoopSinkCloser must not dereference ctx, so nil context is safe.
	closer := domain.NoopSinkCloser{}
	//nolint:staticcheck // deliberate nil context test
	err := closer.Close(nil, &v1alpha1.RemediationJob{}, "reason") //nolint:staticcheck
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
