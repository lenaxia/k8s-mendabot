package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	resultGroupVersion = schema.GroupVersion{
		Group:   "core.k8sgpt.ai",
		Version: "v1alpha1",
	}
	resultSchemeBuilder = runtime.NewSchemeBuilder(addResultTypes)
	// AddResultToScheme registers Result and ResultList under core.k8sgpt.ai/v1alpha1.
	AddResultToScheme = resultSchemeBuilder.AddToScheme
)

func addResultTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(resultGroupVersion,
		&Result{},
		&ResultList{},
	)
	metav1.AddToGroupVersion(s, resultGroupVersion)
	return nil
}

// NewResultScheme creates a fresh scheme with only the Result / ResultList types
// registered. Used in tests where a minimal scheme is needed.
func NewResultScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = AddResultToScheme(s)
	return s
}

// ResultSpec describes the body of a k8sgpt analysis Result.
type ResultSpec struct {
	Backend      string    `json:"backend"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	Error        []Failure `json:"error"`
	Details      string    `json:"details"`
	ParentObject string    `json:"parentObject"`
}

// Failure is a single error identified within a Result.
type Failure struct {
	Text      string      `json:"text,omitempty"`
	Sensitive []Sensitive `json:"sensitive,omitempty"`
}

// Sensitive holds masked/unmasked pairs for redactable values.
type Sensitive struct {
	Unmasked string `json:"unmasked,omitempty"`
	Masked   string `json:"masked,omitempty"`
}

// ResultStatus is intentionally minimal — the watcher reads Results but never
// writes their status. Only the fields needed for scheme completeness are defined.
type ResultStatus struct{}

// Result represents a single k8sgpt analysis result.
// +kubebuilder:object:root=true
type Result struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResultSpec   `json:"spec,omitempty"`
	Status ResultStatus `json:"status,omitempty"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *Result) DeepCopyInto(out *Result) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.Error != nil {
		errors := make([]Failure, len(in.Spec.Error))
		for i, f := range in.Spec.Error {
			errors[i].Text = f.Text
			if f.Sensitive != nil {
				sensitive := make([]Sensitive, len(f.Sensitive))
				copy(sensitive, f.Sensitive)
				errors[i].Sensitive = sensitive
			}
		}
		out.Spec.Error = errors
	}
	out.Spec.Backend = in.Spec.Backend
	out.Spec.Kind = in.Spec.Kind
	out.Spec.Name = in.Spec.Name
	out.Spec.Details = in.Spec.Details
	out.Spec.ParentObject = in.Spec.ParentObject
}

// DeepCopyObject implements runtime.Object.
func (in *Result) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Result)
	in.DeepCopyInto(out)
	return out
}

// ResultList contains a list of Result.
// +kubebuilder:object:root=true
type ResultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Result `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *ResultList) DeepCopyInto(out *ResultList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		items := make([]Result, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&items[i])
		}
		out.Items = items
	}
}

// DeepCopyObject implements runtime.Object.
func (in *ResultList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ResultList)
	in.DeepCopyInto(out)
	return out
}
