// Package v1alpha1 contains API types for the remediation.mechanic.io/v1alpha1 API group.
//
// +groupName=remediation.mechanic.io
// +versionName=v1alpha1
package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

// GroupVersion is the canonical schema.GroupVersion for this package.
// controller-gen reads this variable by convention to determine the API group.
var GroupVersion = schema.GroupVersion{ //nolint:gochecknoglobals
	Group:   "remediation.mechanic.io",
	Version: "v1alpha1",
}
