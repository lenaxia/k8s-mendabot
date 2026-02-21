# Story: fingerprintFor Implementation and Tests

**Epic:** [Controller](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1.5 hours

---

## User Story

As a **developer**, I want a `fingerprintFor()` function that produces a stable,
parent-resource-aware SHA256 hash from a `ResultSpec` so that multiple pods from the same
Deployment produce one fingerprint, not many.

---

## Acceptance Criteria

- [ ] Same `kind` + `parentObject` + same error texts → same fingerprint, regardless of
  resource name or error order
- [ ] Different `parentObject` → different fingerprint (even with identical errors)
- [ ] Different `kind` → different fingerprint
- [ ] Different error texts → different fingerprint
- [ ] nil error slice and empty error slice produce the same fingerprint
- [ ] Function is deterministic — same input always produces same output
- [ ] Returns a 64-character lowercase hex string

---

## Test Cases (all must be written before implementation)

| Test Name | Input | Expected |
|-----------|-------|----------|
| `SameParentDifferentPods` | Same kind/parent/errors, different Name | Same fingerprint |
| `DifferentErrors` | Same kind/parent, different error text | Different fingerprint |
| `ErrorOrderIndependent` | Same errors in different order | Same fingerprint |
| `DifferentParents` | Same errors, different parentObject | Different fingerprint |
| `EmptyErrors` | nil vs `[]Failure{}` | Same fingerprint |
| `DifferentKinds` | Same parent/errors, different kind | Different fingerprint |
| `Deterministic` | Same spec called twice | Same output both times |
| `FingerprintEquivalence` | A `*v1alpha1.Result` fed to both `fingerprintFor()` and `K8sGPTProvider.Fingerprint(finding)` (where `finding` comes from `provider.ExtractFinding(result)`) | Both return identical string. **Must include a sub-case where error text contains `<`, `>`, and `&` to guard against json.Marshal HTML-escaping divergence between the two code paths.** |

---

## Notes on Fingerprint Equivalence

`fingerprintFor` (in `reconciler.go`) and `K8sGPTProvider.Fingerprint` (in `provider.go`)
implement the same algorithm at different abstraction levels:

- `fingerprintFor` operates on `v1alpha1.ResultSpec.Error []Failure` — it reads `f.Text`
  directly from the slice.
- `K8sGPTProvider.Fingerprint` operates on `*domain.Finding.Errors` — it re-parses the
  pre-serialised JSON string produced by `ExtractFinding`.

The risk: Go's `json.Marshal` HTML-escapes `<`, `>`, and `&` by default. If an error text
contains these characters, `ExtractFinding` stores the escaped form in `Finding.Errors`.
`Fingerprint()` then hashes the escaped text. `fingerprintFor()` hashes the raw text.
The hashes diverge → deduplication silently broken.

**Required fix in implementation:** Both functions must hash identical bytes. The simplest
approach is to ensure `fingerprintFor` and `Fingerprint` both operate on the same bytes.
The recommended implementation uses `json.NewEncoder` with `SetEscapeHTML(false)` in
`fingerprintFor`, and the same encoder in `ExtractFinding`'s serialisation step. This
guarantees the round-trip is lossless. Verify with `TestFingerprintEquivalence`.

---

## Tasks

- [ ] Write all 7 tests in `internal/provider/k8sgpt/reconciler_test.go` (TDD — tests first, must fail)
- [ ] Implement `fingerprintFor()` in `internal/provider/k8sgpt/reconciler.go`
- [ ] Run tests — all must pass

---

## Dependencies

**Depends on:** STORY_01 (scheme)
**Blocks:** STORY_03 (dedup map), STORY_04 (reconcile loop)

---

## Definition of Done

- [ ] All 7 tests pass with `-race`
- [ ] `go vet` clean
