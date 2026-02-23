package native

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const maxTraversalDepth = 10

// kindToGV maps known Kubernetes Kind names to their GroupVersion for owner reference traversal.
var kindToGV = map[string]schema.GroupVersion{
	"Pod":         {Group: "", Version: "v1"},
	"ReplicaSet":  {Group: "apps", Version: "v1"},
	"Deployment":  {Group: "apps", Version: "v1"},
	"StatefulSet": {Group: "apps", Version: "v1"},
	"DaemonSet":   {Group: "apps", Version: "v1"},
	"Job":         {Group: "batch", Version: "v1"},
	"CronJob":     {Group: "batch", Version: "v1"},
	"ConfigMap":   {Group: "", Version: "v1"},
	"Node":        {Group: "", Version: "v1"},
}

// getParent walks the ownerReferences chain up to the root workload owner.
// Returns "Kind/name" of the root. On any lookup failure, logs at debug level
// and returns the deepest successfully resolved node.
// Guards against infinite loops via max depth (10) and visited UID tracking.
func getParent(ctx context.Context, c client.Client, meta metav1.ObjectMeta, kind string) string {
	logger := zap.NewNop()

	if len(meta.OwnerReferences) == 0 {
		return fmt.Sprintf("%s/%s", kind, meta.Name)
	}

	visited := make(map[types.UID]struct{})
	visited[meta.UID] = struct{}{}

	currentKind := kind
	currentName := meta.Name
	currentNamespace := meta.Namespace
	currentOwnerRefs := meta.OwnerReferences

	for depth := 0; depth < maxTraversalDepth; depth++ {
		if len(currentOwnerRefs) == 0 {
			return fmt.Sprintf("%s/%s", currentKind, currentName)
		}

		owner := currentOwnerRefs[0]

		if _, seen := visited[owner.UID]; seen {
			logger.Debug("circular owner reference detected",
				zap.String("ownerKind", owner.Kind),
				zap.String("ownerName", owner.Name),
				zap.String("ownerUID", string(owner.UID)),
			)
			return fmt.Sprintf("%s/%s", currentKind, currentName)
		}
		visited[owner.UID] = struct{}{}

		gv, ok := kindToGV[owner.Kind]
		if !ok {
			logger.Debug("unknown owner kind, stopping traversal",
				zap.String("ownerKind", owner.Kind),
				zap.String("ownerName", owner.Name),
			)
			return fmt.Sprintf("%s/%s", currentKind, currentName)
		}

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    owner.Kind,
		})

		if err := c.Get(ctx, client.ObjectKey{Namespace: currentNamespace, Name: owner.Name}, obj); err != nil {
			logger.Debug("failed to fetch owner object, falling back to current node",
				zap.String("ownerKind", owner.Kind),
				zap.String("ownerName", owner.Name),
				zap.String("ownerUID", string(owner.UID)),
				zap.Error(err),
			)
			return fmt.Sprintf("%s/%s", currentKind, currentName)
		}

		currentKind = owner.Kind
		currentName = owner.Name
		currentOwnerRefs = obj.GetOwnerReferences()
	}

	return fmt.Sprintf("%s/%s", currentKind, currentName)
}
