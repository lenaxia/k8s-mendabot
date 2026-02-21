package v1alpha1_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lenaxia/k8s-mendabot/api/v1alpha1"
)

func TestResult_DeepCopyObject_IndependentCopy(t *testing.T) {
	original := &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-result",
			Namespace: "default",
		},
		Spec: v1alpha1.ResultSpec{
			Kind:         "Pod",
			Name:         "my-pod",
			ParentObject: "my-deployment",
			Error: []v1alpha1.Failure{
				{Text: "back-off restarting failed container"},
			},
		},
	}

	copied := original.DeepCopyObject()
	result, ok := copied.(*v1alpha1.Result)
	if !ok {
		t.Fatalf("DeepCopyObject did not return *Result")
	}

	result.Spec.Kind = "Deployment"
	if original.Spec.Kind != "Pod" {
		t.Errorf("mutating copy affected original: Spec.Kind = %q", original.Spec.Kind)
	}

	result.Spec.Error[0].Text = "different error"
	if original.Spec.Error[0].Text != "back-off restarting failed container" {
		t.Errorf("mutating copy Error slice affected original")
	}
}

func TestResult_DeepCopyInto_SliceIsIndependent(t *testing.T) {
	original := &v1alpha1.Result{
		Spec: v1alpha1.ResultSpec{
			Error: []v1alpha1.Failure{
				{
					Text: "error text",
					Sensitive: []v1alpha1.Sensitive{
						{Unmasked: "secret", Masked: "s*****"},
					},
				},
			},
		},
	}

	var dst v1alpha1.Result
	original.DeepCopyInto(&dst)

	dst.Spec.Error[0].Text = "changed"
	if original.Spec.Error[0].Text != "error text" {
		t.Errorf("mutating dst Error[0].Text affected original")
	}

	dst.Spec.Error[0].Sensitive[0].Unmasked = "leaked"
	if original.Spec.Error[0].Sensitive[0].Unmasked != "secret" {
		t.Errorf("mutating dst Sensitive[0].Unmasked affected original")
	}
}

func TestResult_DeepCopyObject_NilErrors(t *testing.T) {
	original := &v1alpha1.Result{
		Spec: v1alpha1.ResultSpec{
			Error: nil,
		},
	}
	copied := original.DeepCopyObject().(*v1alpha1.Result)
	if copied.Spec.Error != nil {
		t.Errorf("expected nil Error slice in copy, got %v", copied.Spec.Error)
	}
}

func TestResultList_DeepCopyObject_IndependentCopy(t *testing.T) {
	original := &v1alpha1.ResultList{
		Items: []v1alpha1.Result{
			{
				Spec: v1alpha1.ResultSpec{
					Kind: "Pod",
					Error: []v1alpha1.Failure{
						{Text: "some error"},
					},
				},
			},
		},
	}

	copied := original.DeepCopyObject()
	list, ok := copied.(*v1alpha1.ResultList)
	if !ok {
		t.Fatalf("DeepCopyObject did not return *ResultList")
	}

	list.Items[0].Spec.Kind = "Deployment"
	if original.Items[0].Spec.Kind != "Pod" {
		t.Errorf("mutating copy affected original: Items[0].Spec.Kind = %q", original.Items[0].Spec.Kind)
	}

	list.Items[0].Spec.Error[0].Text = "different"
	if original.Items[0].Spec.Error[0].Text != "some error" {
		t.Errorf("mutating copy Error slice affected original")
	}
}

func TestResultList_DeepCopyObject_EmptyItems(t *testing.T) {
	original := &v1alpha1.ResultList{Items: nil}
	copied := original.DeepCopyObject().(*v1alpha1.ResultList)
	if copied.Items != nil {
		t.Errorf("expected nil Items in copy, got %v", copied.Items)
	}
}

func TestAddResultToScheme_RegistersBothTypes(t *testing.T) {
	scheme := v1alpha1.NewResultScheme()

	gvks, _, err := scheme.ObjectKinds(&v1alpha1.Result{})
	if err != nil {
		t.Fatalf("Result not registered in scheme: %v", err)
	}
	found := false
	for _, gvk := range gvks {
		if gvk.Group == "core.k8sgpt.ai" && gvk.Version == "v1alpha1" && gvk.Kind == "Result" {
			found = true
		}
	}
	if !found {
		t.Errorf("Result not registered under core.k8sgpt.ai/v1alpha1, got %v", gvks)
	}

	gvks2, _, err := scheme.ObjectKinds(&v1alpha1.ResultList{})
	if err != nil {
		t.Fatalf("ResultList not registered in scheme: %v", err)
	}
	found2 := false
	for _, gvk := range gvks2 {
		if gvk.Group == "core.k8sgpt.ai" && gvk.Kind == "ResultList" {
			found2 = true
		}
	}
	if !found2 {
		t.Errorf("ResultList not registered under core.k8sgpt.ai/v1alpha1, got %v", gvks2)
	}
}

func TestResult_Namespace_AccessibleViaObjectMeta(t *testing.T) {
	r := &v1alpha1.Result{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
		},
	}
	if r.Namespace != "kube-system" {
		t.Errorf("Namespace: got %q, want %q", r.Namespace, "kube-system")
	}
}
