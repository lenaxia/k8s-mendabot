package domain

import (
	batchv1 "k8s.io/api/batch/v1"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

// JobBuilder constructs a batch/v1 Job from a RemediationJob.
// The concrete implementation lives in internal/jobbuilder.
type JobBuilder interface {
	Build(rjob *v1alpha1.RemediationJob) (*batchv1.Job, error)
}
