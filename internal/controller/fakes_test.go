package controller_test

import (
	"errors"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
	domain "github.com/lenaxia/k8s-mechanic/internal/domain"
)

type fakeJobBuilderCall struct {
	RemediationJob     *v1alpha1.RemediationJob
	CorrelatedFindings []v1alpha1.FindingSpec
}

type fakeJobBuilder struct {
	calls     []fakeJobBuilderCall
	returnJob *batchv1.Job
	returnErr error
}

func (f *fakeJobBuilder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error) {
	f.calls = append(f.calls, fakeJobBuilderCall{rjob, correlatedFindings})
	return f.returnJob, f.returnErr
}

var _ domain.JobBuilder = (*fakeJobBuilder)(nil)

func ptr[T any](v T) *T { return &v }

func defaultFakeJob(rjob *v1alpha1.RemediationJob) *batchv1.Job {
	name := "mechanic-agent-" + rjob.Spec.Fingerprint[:12]
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rjob.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "remediation.mechanic.io/v1alpha1",
					Kind:       "RemediationJob",
					Name:       rjob.Name,
					UID:        rjob.UID,
				},
			},
			Labels: map[string]string{
				"remediation.mechanic.io/remediation-job": rjob.Name,
				"app.kubernetes.io/managed-by":            "mechanic-watcher",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr(int32(1)),
		},
	}
}

func newTestRJob(name, namespace, fingerprint string, uid types.UID) *v1alpha1.RemediationJob {
	return &v1alpha1.RemediationJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
		Spec: v1alpha1.RemediationJobSpec{
			Fingerprint: fingerprint,
		},
	}
}

func TestFakeJobBuilder_RecordsCalls(t *testing.T) {
	rjob1 := newTestRJob("rjob-1", "default", "abcdefghijklmnop", "uid-1")
	rjob2 := newTestRJob("rjob-2", "kube-system", "0123456789abcdef", "uid-2")

	f := &fakeJobBuilder{}
	_, _ = f.Build(rjob1, nil)
	_, _ = f.Build(rjob2, nil)

	if len(f.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(f.calls))
	}
	if f.calls[0].RemediationJob != rjob1 {
		t.Errorf("call[0] got %v, want %v", f.calls[0].RemediationJob, rjob1)
	}
	if f.calls[1].RemediationJob != rjob2 {
		t.Errorf("call[1] got %v, want %v", f.calls[1].RemediationJob, rjob2)
	}
}

func TestFakeJobBuilder_PropagatesError(t *testing.T) {
	want := errors.New("build failed")
	f := &fakeJobBuilder{returnErr: want}
	rjob := newTestRJob("rjob-1", "default", "abcdefghijklmnop", "uid-1")

	job, err := f.Build(rjob, nil)

	if err != want {
		t.Errorf("got err %v, want %v", err, want)
	}
	if job != nil {
		t.Errorf("expected nil job, got %v", job)
	}
}

func TestFakeJobBuilder_ReturnsJob(t *testing.T) {
	rjob := newTestRJob("rjob-1", "default", "abcdefghijklmnop", "uid-1")
	want := defaultFakeJob(rjob)
	f := &fakeJobBuilder{returnJob: want}

	job, err := f.Build(rjob, nil)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if job != want {
		t.Errorf("got job %v, want %v", job, want)
	}
}

func TestDefaultFakeJob_NamePattern(t *testing.T) {
	rjob := newTestRJob("rjob-1", "default", "abcdefghijklmnop", "uid-1")

	job := defaultFakeJob(rjob)

	want := "mechanic-agent-abcdefghijkl"
	if job.Name != want {
		t.Errorf("got name %q, want %q", job.Name, want)
	}
}

func TestDefaultFakeJob_OwnerRef(t *testing.T) {
	rjob := newTestRJob("my-rjob", "my-ns", "abcdefghijklmnop", types.UID("test-uid-42"))

	job := defaultFakeJob(rjob)

	if len(job.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(job.OwnerReferences))
	}
	ref := job.OwnerReferences[0]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"APIVersion", ref.APIVersion, "remediation.mechanic.io/v1alpha1"},
		{"Kind", ref.Kind, "RemediationJob"},
		{"Name", ref.Name, "my-rjob"},
		{"UID", string(ref.UID), "test-uid-42"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestDefaultFakeJob_BackoffLimit(t *testing.T) {
	rjob := newTestRJob("rjob-1", "default", "abcdefghijklmnop", "uid-1")

	job := defaultFakeJob(rjob)

	if job.Spec.BackoffLimit == nil {
		t.Fatal("expected BackoffLimit to be set, got nil")
	}
	if *job.Spec.BackoffLimit != 1 {
		t.Errorf("got BackoffLimit %d, want 1", *job.Spec.BackoffLimit)
	}
}

func TestDefaultFakeJob_Label(t *testing.T) {
	rjob := newTestRJob("my-rjob", "default", "abcdefghijklmnop", "uid-1")

	job := defaultFakeJob(rjob)

	const key = "remediation.mechanic.io/remediation-job"
	got, ok := job.Labels[key]
	if !ok {
		t.Fatalf("expected label %q to be set", key)
	}
	if got != "my-rjob" {
		t.Errorf("got label value %q, want %q", got, "my-rjob")
	}
}
