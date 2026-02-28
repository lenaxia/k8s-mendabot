package native

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/lenaxia/k8s-mechanic/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := v1alpha1.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	return s
}

func ownerRef(kind, name string, uid types.UID) metav1.OwnerReference {
	return metav1.OwnerReference{
		Kind: kind,
		Name: name,
		UID:  uid,
	}
}

// TestGetParent_NoOwnerRefs: Pod with no ownerReferences → returns "Pod/my-pod".
func TestGetParent_NoOwnerRefs(t *testing.T) {
	s := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	meta := metav1.ObjectMeta{
		Name:            "my-pod",
		Namespace:       "default",
		UID:             "pod-uid-1",
		OwnerReferences: nil,
	}
	got := getParent(context.Background(), c, meta, "Pod")
	want := "Pod/my-pod"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGetParent_PodWithReplicaSetParent: Pod → ReplicaSet → Deployment; expect "Deployment/my-deploy".
func TestGetParent_PodWithReplicaSetParent(t *testing.T) {
	s := newTestScheme()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
			UID:       "deploy-uid-1",
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy-7d9f8b",
			Namespace: "default",
			UID:       "rs-uid-1",
			OwnerReferences: []metav1.OwnerReference{
				ownerRef("Deployment", "my-deploy", "deploy-uid-1"),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(deploy, rs).Build()

	meta := metav1.ObjectMeta{
		Name:      "my-pod",
		Namespace: "default",
		UID:       "pod-uid-1",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("ReplicaSet", "my-deploy-7d9f8b", "rs-uid-1"),
		},
	}

	got := getParent(context.Background(), c, meta, "Pod")
	want := "Deployment/my-deploy"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGetParent_PodWithStatefulSetParent: Pod directly owned by StatefulSet; expect "StatefulSet/my-sts".
func TestGetParent_PodWithStatefulSetParent(t *testing.T) {
	s := newTestScheme()

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-sts",
			Namespace: "default",
			UID:       "sts-uid-1",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(sts).Build()

	meta := metav1.ObjectMeta{
		Name:      "my-pod",
		Namespace: "default",
		UID:       "pod-uid-1",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("StatefulSet", "my-sts", "sts-uid-1"),
		},
	}

	got := getParent(context.Background(), c, meta, "Pod")
	want := "StatefulSet/my-sts"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGetParent_PodWithDaemonSetParent: Pod directly owned by DaemonSet; expect "DaemonSet/my-ds".
func TestGetParent_PodWithDaemonSetParent(t *testing.T) {
	s := newTestScheme()

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ds",
			Namespace: "default",
			UID:       "ds-uid-1",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ds).Build()

	meta := metav1.ObjectMeta{
		Name:      "my-pod",
		Namespace: "default",
		UID:       "pod-uid-1",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("DaemonSet", "my-ds", "ds-uid-1"),
		},
	}

	got := getParent(context.Background(), c, meta, "Pod")
	want := "DaemonSet/my-ds"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGetParent_PodWithJobParent: Pod → Job → CronJob; expect "CronJob/my-cronjob".
func TestGetParent_PodWithJobParent(t *testing.T) {
	s := newTestScheme()

	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cronjob",
			Namespace: "default",
			UID:       "cj-uid-1",
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-job",
			Namespace: "default",
			UID:       "job-uid-1",
			OwnerReferences: []metav1.OwnerReference{
				ownerRef("CronJob", "my-cronjob", "cj-uid-1"),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cj, job).Build()

	meta := metav1.ObjectMeta{
		Name:      "my-pod",
		Namespace: "default",
		UID:       "pod-uid-1",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("Job", "my-job", "job-uid-1"),
		},
	}

	got := getParent(context.Background(), c, meta, "Pod")
	want := "CronJob/my-cronjob"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGetParent_MaxDepthGuard: chain of 15 ConfigMaps each owning the next;
// expect traversal stops at depth 10 and returns something (no infinite loop or panic).
func TestGetParent_MaxDepthGuard(t *testing.T) {
	s := newTestScheme()

	// Build a chain of 15 unstructured ConfigMaps: cm-0 owns cm-1 owns ... cm-14
	// cm-0 is the "input" resource. We expect traversal to stop at depth 10.
	objs := make([]runtime.Object, 0, 15)
	for i := 0; i < 15; i++ {
		cm := &unstructured.Unstructured{}
		cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		cm.SetName(fmt.Sprintf("cm-%d", i))
		cm.SetNamespace("default")
		cm.SetUID(types.UID(fmt.Sprintf("cm-uid-%d", i)))
		if i < 14 {
			cm.SetOwnerReferences([]metav1.OwnerReference{
				ownerRef("ConfigMap", fmt.Sprintf("cm-%d", i+1), types.UID(fmt.Sprintf("cm-uid-%d", i+1))),
			})
		}
		objs = append(objs, cm)
	}

	builder := fake.NewClientBuilder().WithScheme(s)
	for _, obj := range objs {
		builder = builder.WithRuntimeObjects(obj)
	}
	c := builder.Build()

	// Input: cm-0 with owner cm-1
	meta := metav1.ObjectMeta{
		Name:      "cm-0",
		Namespace: "default",
		UID:       "cm-uid-0",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("ConfigMap", "cm-1", "cm-uid-1"),
		},
	}

	// Should not hang; should return something after max depth is hit
	got := getParent(context.Background(), c, meta, "ConfigMap")
	if got == "" {
		t.Error("expected non-empty return from max depth traversal")
	}
	t.Logf("MaxDepthGuard returned: %q", got)
}

// TestGetParent_CircularOwnerRefs: A owns B owns A (circular); expect no infinite loop.
func TestGetParent_CircularOwnerRefs(t *testing.T) {
	s := newTestScheme()

	// Two ConfigMaps: A owns B, B owns A
	cmA := &unstructured.Unstructured{}
	cmA.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cmA.SetName("cm-a")
	cmA.SetNamespace("default")
	cmA.SetUID("cm-uid-a")
	cmA.SetOwnerReferences([]metav1.OwnerReference{
		ownerRef("ConfigMap", "cm-b", "cm-uid-b"),
	})

	cmB := &unstructured.Unstructured{}
	cmB.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	cmB.SetName("cm-b")
	cmB.SetNamespace("default")
	cmB.SetUID("cm-uid-b")
	cmB.SetOwnerReferences([]metav1.OwnerReference{
		ownerRef("ConfigMap", "cm-a", "cm-uid-a"),
	})

	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(cmA, cmB).Build()

	meta := metav1.ObjectMeta{
		Name:      "cm-a",
		Namespace: "default",
		UID:       "cm-uid-a",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("ConfigMap", "cm-b", "cm-uid-b"),
		},
	}

	// Should terminate and return something sane — not loop forever
	got := getParent(context.Background(), c, meta, "ConfigMap")
	if got == "" {
		t.Error("expected non-empty return from circular owner refs")
	}
	t.Logf("CircularOwnerRefs returned: %q", got)
}

// TestGetParent_OwnerNotFound: ownerReference points to a resource that doesn't exist;
// expect fallback to "Kind/name" of input (no error, no panic).
func TestGetParent_OwnerNotFound(t *testing.T) {
	s := newTestScheme()
	// No objects in the fake client — owner won't be found
	c := fake.NewClientBuilder().WithScheme(s).Build()

	meta := metav1.ObjectMeta{
		Name:      "my-pod",
		Namespace: "default",
		UID:       "pod-uid-1",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("ReplicaSet", "missing-rs", "missing-uid"),
		},
	}

	got := getParent(context.Background(), c, meta, "Pod")
	want := "Pod/my-pod"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGetParent_DirectDeploymentOwner: Pod directly owned by Deployment (no ReplicaSet);
// expect "Deployment/my-deploy".
func TestGetParent_DirectDeploymentOwner(t *testing.T) {
	s := newTestScheme()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
			UID:       "deploy-uid-1",
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(deploy).Build()

	meta := metav1.ObjectMeta{
		Name:      "my-pod",
		Namespace: "default",
		UID:       "pod-uid-1",
		OwnerReferences: []metav1.OwnerReference{
			ownerRef("Deployment", "my-deploy", "deploy-uid-1"),
		},
	}

	got := getParent(context.Background(), c, meta, "Pod")
	want := "Deployment/my-deploy"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
