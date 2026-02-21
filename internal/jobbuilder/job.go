package jobbuilder

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: Builder satisfies domain.JobBuilder.
var _ domain.JobBuilder = (*Builder)(nil)

// Config holds the configuration for a Builder.
type Config struct {
	AgentNamespace string // namespace where Jobs are created — must equal watcher namespace
}

// Builder constructs batch/v1 Jobs from RemediationJob CRDs.
type Builder struct {
	cfg Config
}

// New creates a new Builder. Returns an error if AgentNamespace is empty.
func New(cfg Config) (*Builder, error) {
	if cfg.AgentNamespace == "" {
		return nil, fmt.Errorf("jobbuilder: AgentNamespace must not be empty")
	}
	return &Builder{cfg: cfg}, nil
}

// Build constructs a batch/v1 Job from a RemediationJob.
func (b *Builder) Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error) {
	panic("not implemented")
}
