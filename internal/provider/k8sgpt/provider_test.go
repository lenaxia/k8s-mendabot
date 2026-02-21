package k8sgpt_test

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/lenaxia/k8s-mendabot/api/v1alpha1"
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"github.com/lenaxia/k8s-mendabot/internal/provider/k8sgpt"
)

var _ domain.SourceProvider = (*k8sgpt.K8sGPTProvider)(nil)

func TestK8sGPTProvider_ProviderName(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	got := p.ProviderName()
	if got != v1alpha1.SourceTypeK8sGPT {
		t.Errorf("ProviderName() = %q, want %q", got, v1alpha1.SourceTypeK8sGPT)
	}
}

func TestK8sGPTProvider_ObjectType(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	obj := p.ObjectType()
	if obj == nil {
		t.Fatal("ObjectType() returned nil")
	}
	if _, ok := obj.(*v1alpha1.Result); !ok {
		t.Errorf("ObjectType() returned %T, want *v1alpha1.Result", obj)
	}
}

func TestK8sGPTProvider_ExtractFinding_NoErrors(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	result := &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec:       v1alpha1.ResultSpec{Kind: "Pod", Name: "pod-abc", Error: nil},
	}
	finding, err := p.ExtractFinding(result)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for empty errors, got %+v", finding)
	}
}

func TestK8sGPTProvider_ExtractFinding_EmptySlice(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	result := &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec:       v1alpha1.ResultSpec{Kind: "Pod", Error: []v1alpha1.Failure{}},
	}
	finding, err := p.ExtractFinding(result)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if finding != nil {
		t.Errorf("expected nil finding for empty errors, got %+v", finding)
	}
}

func TestK8sGPTProvider_ExtractFinding_WithErrors(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	result := &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{Name: "result-xyz", Namespace: "production"},
		Spec: v1alpha1.ResultSpec{
			Kind:         "Pod",
			Name:         "pod-xyz",
			ParentObject: "my-deployment",
			Details:      "The pod is crash looping",
			Error: []v1alpha1.Failure{
				{Text: "CrashLoopBackOff"},
				{Text: "back-off restarting failed container"},
			},
		},
	}

	finding, err := p.ExtractFinding(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding")
	}

	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Kind", finding.Kind, "Pod"},
		{"Name", finding.Name, "pod-xyz"},
		{"Namespace", finding.Namespace, "production"},
		{"ParentObject", finding.ParentObject, "my-deployment"},
		{"Details", finding.Details, "The pod is crash looping"},
		{"SourceRef.Kind", finding.SourceRef.Kind, "Result"},
		{"SourceRef.Name", finding.SourceRef.Name, "result-xyz"},
		{"SourceRef.Namespace", finding.SourceRef.Namespace, "production"},
		{"SourceRef.APIVersion", finding.SourceRef.APIVersion, "core.k8sgpt.ai/v1alpha1"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}

	if finding.Errors == "" {
		t.Error("expected non-empty Errors JSON")
	}
	if !strings.Contains(finding.Errors, "CrashLoopBackOff") {
		t.Errorf("Errors JSON missing expected text; got %q", finding.Errors)
	}
}

func TestK8sGPTProvider_ExtractFinding_SensitiveRedacted(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	result := &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec: v1alpha1.ResultSpec{
			Kind: "Pod",
			Error: []v1alpha1.Failure{
				{
					Text: "connection refused",
					Sensitive: []v1alpha1.Sensitive{
						{Unmasked: "my-secret-token", Masked: "***"},
					},
				},
			},
		},
	}

	finding, err := p.ExtractFinding(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding == nil {
		t.Fatal("expected non-nil finding")
	}

	if strings.Contains(finding.Errors, "my-secret-token") {
		t.Error("sensitive unmasked value must not appear in Finding.Errors")
	}
	if strings.Contains(finding.Errors, "sensitive") {
		t.Error("sensitive field key must not appear in Finding.Errors")
	}
	if !strings.Contains(finding.Errors, "connection refused") {
		t.Errorf("error text should be preserved; got %q", finding.Errors)
	}
}

func TestK8sGPTProvider_ExtractFinding_WrongType(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	wrongObj := &v1alpha1.RemediationJob{}

	finding, err := p.ExtractFinding(wrongObj)
	if err == nil {
		t.Error("expected error for wrong object type, got nil")
	}
	if finding != nil {
		t.Errorf("expected nil finding on error, got %+v", finding)
	}
}

func TestK8sGPTProvider_Fingerprint_Deterministic(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	finding := &domain.Finding{
		Kind:         "Pod",
		Namespace:    "default",
		ParentObject: "my-deploy",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
	}
	fp1, err := p.Fingerprint(finding)
	if err != nil {
		t.Fatalf("Fingerprint returned unexpected error: %v", err)
	}
	fp2, err := p.Fingerprint(finding)
	if err != nil {
		t.Fatalf("Fingerprint returned unexpected error: %v", err)
	}
	if fp1 != fp2 {
		t.Errorf("Fingerprint must be deterministic; got %q and %q", fp1, fp2)
	}
	if len(fp1) != 64 {
		t.Errorf("expected 64-char hex fingerprint, got %d chars: %q", len(fp1), fp1)
	}
}

func TestK8sGPTProvider_Fingerprint_OrderIndependent(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	f1 := &domain.Finding{
		Kind: "Pod", Namespace: "default", ParentObject: "dp",
		Errors: `[{"text":"alpha"},{"text":"beta"}]`,
	}
	f2 := &domain.Finding{
		Kind: "Pod", Namespace: "default", ParentObject: "dp",
		Errors: `[{"text":"beta"},{"text":"alpha"}]`,
	}
	fp1, err := p.Fingerprint(f1)
	if err != nil {
		t.Fatalf("Fingerprint f1 error: %v", err)
	}
	fp2, err := p.Fingerprint(f2)
	if err != nil {
		t.Fatalf("Fingerprint f2 error: %v", err)
	}
	if fp1 != fp2 {
		t.Error("Fingerprint must be order-independent")
	}
}

// TestK8sGPTProvider_Fingerprint_MalformedErrors verifies that malformed Errors JSON
// returns an error rather than silently producing a collision-prone fingerprint.
func TestK8sGPTProvider_Fingerprint_MalformedErrors(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	f := &domain.Finding{
		Namespace:    "default",
		Kind:         "Pod",
		ParentObject: "my-deployment",
		Errors:       "this is not json {{{",
	}
	_, err := p.Fingerprint(f)
	if err == nil {
		t.Fatal("expected error for malformed Errors JSON, got nil")
	}
}

// TestK8sGPTProvider_Fingerprint_EmptyErrors verifies that an empty Errors string
// (e.g. a finding with no error details) produces a valid fingerprint, not an error.
func TestK8sGPTProvider_Fingerprint_EmptyErrors(t *testing.T) {
	p := &k8sgpt.K8sGPTProvider{}
	f := &domain.Finding{
		Namespace:    "default",
		Kind:         "Pod",
		ParentObject: "my-deployment",
		Errors:       "",
	}
	fp, err := p.Fingerprint(f)
	if err != nil {
		t.Fatalf("unexpected error for empty Errors: %v", err)
	}
	if len(fp) != 64 {
		t.Errorf("expected 64-char hex fingerprint, got %d chars", len(fp))
	}
}
