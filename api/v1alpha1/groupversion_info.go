// Package v1alpha1 contains API types for the remediation.mendabot.io/v1alpha1 API group.
//
// +groupName=remediation.mendabot.io
// +versionName=v1alpha1
package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

// GroupVersion is the canonical schema.GroupVersion for this package.
// controller-gen reads this variable by convention to determine the API group.
var GroupVersion = schema.GroupVersion{ //nolint:gochecknoglobals
	Group:   "remediation.mendabot.io",
	Version: "v1alpha1",
}
