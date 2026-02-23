package main

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgoscheme.AddToScheme: %v", err)
	}
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatalf("batchv1.AddToScheme: %v", err)
	}
	if err := v1alpha1.AddRemediationToScheme(s); err != nil {
		t.Fatalf("AddRemediationToScheme: %v", err)
	}
	return s
}

func TestScheme_ContainsRemediationJobKinds(t *testing.T) {
	s := buildScheme(t)
	gvk := schema.GroupVersionKind{
		Group:   "remediation.mendabot.io",
		Version: "v1alpha1",
		Kind:    "RemediationJob",
	}
	if !s.Recognizes(gvk) {
		t.Errorf("scheme does not recognise %v", gvk)
	}
	gvkList := schema.GroupVersionKind{
		Group:   "remediation.mendabot.io",
		Version: "v1alpha1",
		Kind:    "RemediationJobList",
	}
	if !s.Recognizes(gvkList) {
		t.Errorf("scheme does not recognise %v", gvkList)
	}
}

func TestScheme_ContainsBatchV1Job(t *testing.T) {
	s := buildScheme(t)
	gvk := schema.GroupVersionKind{
		Group:   "batch",
		Version: "v1",
		Kind:    "Job",
	}
	if !s.Recognizes(gvk) {
		t.Errorf("scheme does not recognise %v", gvk)
	}
}
