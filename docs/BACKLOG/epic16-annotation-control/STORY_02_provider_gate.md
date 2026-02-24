# Story: Providers — ExtractFinding Annotation Gate

**Epic:** [epic16-annotation-control](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1 hour

---

## User Story

As a **cluster operator**, I want to annotate a Pod, Deployment, StatefulSet, Job, Node,
or PVC with `mendabot.io/enabled: "false"` or `mendabot.io/skip-until: "YYYY-MM-DD"` so
that mendabot's `ExtractFinding` returns `(nil, nil)` for that resource and no
`RemediationJob` is ever created for it during the suppression period.

---

## Background

`client.Object` (the interface passed as `obj` to `ExtractFinding`) embeds
`metav1.Object`, which includes `GetAnnotations() map[string]string`. This means
`obj.GetAnnotations()` can be called directly on the interface value — **before any
concrete type assertion is performed**. Checking annotations first is therefore the
cheapest possible guard: it avoids all provider-specific reflection and status inspection
whenever the resource is suppressed.

All six native providers follow the same pattern:

```go
func (p *xyzProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
    concrete, ok := obj.(*XyzType)      // type assertion — line N
    if !ok {
        return nil, fmt.Errorf(...)
    }
    // ... provider-specific logic ...
}
```

The annotation check is inserted **before** the type assertion at line N.

---

## Design

### Insertion point — all six providers

Insert the following two lines **immediately before** the type assertion in each
provider's `ExtractFinding`:

```go
if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
    return nil, nil
}
```

`obj.GetAnnotations()` is called on the `client.Object` interface — no type assertion
is required and the call is always safe regardless of the concrete type.

`time.Now()` is passed directly; tests that need a fixed clock must exercise
`domain.ShouldSkip` directly (unit-tested in STORY_01) or use a fake object whose
annotations are set to a static past/future date.

### Changes by file

#### `internal/provider/native/pod.go`

Current `ExtractFinding` header (line 49–53):
```go
func (p *podProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("podProvider: expected *corev1.Pod, got %T", obj)
	}
```

After change:
```go
func (p *podProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("podProvider: expected *corev1.Pod, got %T", obj)
	}
```

Add `"time"` to the import block.

#### `internal/provider/native/deployment.go`

Current `ExtractFinding` header (line 39–43):
```go
func (p *deploymentProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	deploy, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil, fmt.Errorf("deploymentProvider: expected *appsv1.Deployment, got %T", obj)
	}
```

After change:
```go
func (p *deploymentProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	deploy, ok := obj.(*appsv1.Deployment)
	if !ok {
		return nil, fmt.Errorf("deploymentProvider: expected *appsv1.Deployment, got %T", obj)
	}
```

Add `"time"` to the import block.

#### `internal/provider/native/statefulset.go`

Current `ExtractFinding` header (line 39–43):
```go
func (p *statefulSetProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil, fmt.Errorf("statefulSetProvider: expected *appsv1.StatefulSet, got %T", obj)
	}
```

After change:
```go
func (p *statefulSetProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil, fmt.Errorf("statefulSetProvider: expected *appsv1.StatefulSet, got %T", obj)
	}
```

Add `"time"` to the import block.

#### `internal/provider/native/job.go`

Current `ExtractFinding` header (line 42–46):
```go
func (p *jobProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("jobProvider: expected *batchv1.Job, got %T", obj)
	}
```

After change:
```go
func (p *jobProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("jobProvider: expected *batchv1.Job, got %T", obj)
	}
```

Add `"time"` to the import block.

#### `internal/provider/native/node.go`

`nodeProvider` is cluster-scoped — Nodes have no namespace, but they do have
`ObjectMeta` and therefore annotations. The insertion point is the same.

Current `ExtractFinding` header (line 53–57):
```go
func (n *nodeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil, fmt.Errorf("nodeProvider: expected *corev1.Node, got %T", obj)
	}
```

After change:
```go
func (n *nodeProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil, fmt.Errorf("nodeProvider: expected *corev1.Node, got %T", obj)
	}
```

Add `"time"` to the import block.

#### `internal/provider/native/pvc.go`

Current `ExtractFinding` header (line 39–43):
```go
func (p *pvcProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil, fmt.Errorf("pvcProvider: expected *corev1.PersistentVolumeClaim, got %T", obj)
	}
```

After change:
```go
func (p *pvcProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil, fmt.Errorf("pvcProvider: expected *corev1.PersistentVolumeClaim, got %T", obj)
	}
```

Add `"time"` to the import block.

---

## Acceptance Criteria

- [ ] All six `ExtractFinding` functions check `domain.ShouldSkip` as the **first** statement
- [ ] The check is placed before the concrete type assertion in all six providers
- [ ] `obj.GetAnnotations()` is called on the `client.Object` interface value — no type
  assertion is done before this call
- [ ] `time` is added to the import block in every modified file
- [ ] Existing provider tests continue to pass unmodified (they do not set the suppression
  annotations, so no behaviour changes)
- [ ] New tests are added to each provider's `_test.go` file covering the two suppression
  cases (see Test Cases below)

---

## Test Cases

Add to each provider's existing `_test.go` file. The test structure mirrors the existing
table-driven tests in those files.

For every provider (pod, deployment, statefulset, job, node, pvc):

| Test Name | Object setup | Expected |
|---|---|---|
| `AnnotationEnabled_False` | Healthy-looking object with annotation `mendabot.io/enabled: "false"` | `(nil, nil)` |
| `AnnotationSkipUntilFuture` | Failing object with annotation `mendabot.io/skip-until: "2099-12-31"` | `(nil, nil)` |

**Node-specific note:** Nodes are cluster-scoped (no namespace). The test object is a
`*corev1.Node` with `ObjectMeta.Annotations` set directly — no namespace required.

**Rationale for not testing `skip-until` past or malformed in provider tests:** those
boundary cases are fully covered by the unit tests for `domain.ShouldSkip` in STORY_01.
Provider tests only need to confirm the gate is wired in and fires at all.

---

## Tasks

- [ ] Ensure STORY_01 is complete (i.e. `domain.ShouldSkip` exists and tests pass)
- [ ] Write failing tests in each provider's `_test.go` for `AnnotationEnabled_False` and
  `AnnotationSkipUntilFuture` (TDD — verify failure before modifying the providers)
- [ ] Add the annotation guard to `pod.go`, `deployment.go`, `statefulset.go`, `job.go`,
  `node.go`, and `pvc.go` as described above
- [ ] Add `"time"` import to each modified file
- [ ] Run `go test -race ./internal/provider/native/...` — all tests must pass
- [ ] Run `go vet ./internal/provider/native/...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (`domain.ShouldSkip` and annotation constants)
**Blocks:** Nothing

---

## Definition of Done

- [ ] All six providers have the annotation guard as the first line of `ExtractFinding`
- [ ] All new and existing provider tests pass with `-race`
- [ ] Full test suite `go test -race ./...` passes
- [ ] `go vet ./...` clean
