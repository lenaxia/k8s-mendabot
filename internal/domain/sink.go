package domain

import (
	"context"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

// SinkCloser closes an open sink (PR or issue) when the underlying finding resolves.
// Implementations must be idempotent: closing an already-closed sink returns nil.
// Close returns nil immediately if rjob.Status.SinkRef.URL is empty.
type SinkCloser interface {
	Close(ctx context.Context, rjob *v1alpha1.RemediationJob, reason string) error
}

// NoopSinkCloser is a SinkCloser that does nothing.
// Used when PR_AUTO_CLOSE=false or in tests that do not need real closure.
type NoopSinkCloser struct{}

func (NoopSinkCloser) Close(_ context.Context, _ *v1alpha1.RemediationJob, _ string) error {
	return nil
}
