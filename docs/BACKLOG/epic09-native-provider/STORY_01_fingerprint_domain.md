# Story: Promote Fingerprint to Domain Function

**Epic:** [epic09-native-provider](README.md)
**Priority:** Critical
**Status:** Not Started
**Estimated Effort:** 1 hour

---

## User Story

As a **developer**, I want fingerprint computation to be a standalone pure function in
`internal/domain` so that every `SourceProvider` implementation gets correct, identical
deduplication behaviour without duplicating the algorithm or requiring an equivalence test.

---

## Background

`Fingerprint(f *Finding) (string, error)` is currently the fourth method on the
`SourceProvider` interface. It is implemented identically in every provider:
SHA256 over `namespace + kind + parentObject + sorted(errorTexts)`. This is a textbook
violation of DRY caused by putting domain logic on an interface that does not need it.

**Note on current codebase state:** `fingerprintFor()` and `reconciler.go` / `reconciler_test.go`
in `internal/provider/k8sgpt/` do **not** exist — they were removed in an earlier epic. The only
copy of the algorithm that remains is `K8sGPTProvider.Fingerprint` in
`internal/provider/k8sgpt/provider.go`. There is no `TestFingerprintEquivalence` test to delete.

**Scope of this story:** Add `domain.FindingFingerprint` and update the reconciler to call it.
The `Fingerprint` method is removed from the interface and the `K8sGPTProvider` struct in STORY_02,
which depends on this story. Splitting the work this way keeps each story self-contained and
buildable: after STORY_01 the interface still has `Fingerprint` (so existing code compiles),
and STORY_02 removes it atomically alongside updating the reconciler's call site.

---

## Exact Function Signature

```go
// FindingFingerprint computes the deduplication key for a Finding.
// It is a pure function — the same input always produces the same output.
//
// Algorithm:
//  1. Parse f.Errors (pre-serialised JSON) into []struct{ Text string }.
//     An empty string or "[]" are treated identically (zero texts).
//  2. Extract the Text field from each element and sort the resulting slice.
//  3. Build a payload struct containing Namespace, Kind, ParentObject, and
//     the sorted texts.
//  4. JSON-encode the payload with SetEscapeHTML(false) to avoid mangling
//     "<", ">", and "&" characters inside error texts.
//  5. Return the lowercase hex SHA256 of the encoded bytes (always 64 chars).
//
// Returns an error only if f.Errors is non-empty and not valid JSON, or if
// json.Encode fails (extremely unlikely in practice).
func FindingFingerprint(f *Finding) (string, error)
```

**Fields that enter the hash** (matching `K8sGPTProvider.Fingerprint` exactly):

| Payload field | Source in `*Finding` |
|---|---|
| `namespace` | `f.Namespace` |
| `kind` | `f.Kind` |
| `parentObject` | `f.ParentObject` |
| `errorTexts` | sorted `[]string` extracted from `f.Errors` JSON |

**Fields NOT in the hash** (intentionally excluded):

| Field | Reason |
|---|---|
| `f.Name` | The pod name changes on restart; the parent anchor is the stable identifier |
| `f.Details` | LLM-generated explanation; changes between runs |
| `f.SourceRef` | Provider-implementation detail; not part of the logical failure identity |

**Byte-for-byte compatibility requirement:** `domain.FindingFingerprint` must produce
identical output to `K8sGPTProvider.Fingerprint` for any given `*Finding`. This is
essential for deduplication continuity: any `RemediationJob` created by the old provider
before this migration will have a fingerprint that matches the new function — no orphaned
jobs, no duplicate re-dispatches during the upgrade window.

---

## Acceptance Criteria

- [ ] `domain.FindingFingerprint(f *Finding) (string, error)` defined in
  `internal/domain/provider.go`
- [ ] Algorithm: parse `f.Errors` JSON into `[]struct{ Text string }`, sort texts, hash
  `sha256(json(namespace, kind, parentObject, sortedTexts))` with `SetEscapeHTML(false)`;
  identical to the current `K8sGPTProvider.Fingerprint` implementation
- [ ] New unit tests for `domain.FindingFingerprint` written in `internal/domain/provider_test.go`
  covering the full test table below (TDD — tests must fail before implementation)
- [ ] All existing tests pass without modification after this story (the interface still has
  `Fingerprint` at this point — removal is STORY_02)

---

## Test Cases (all must be written before implementation)

The payload fields that determine the fingerprint are `Namespace`, `Kind`, `ParentObject`,
and `sorted(errorTexts)` — a change to any of these must change the fingerprint. `Name`,
`Details`, and `SourceRef` do NOT affect the fingerprint.

| Test Name | Input | Expected |
|-----------|-------|----------|
| `Deterministic` | Same `*Finding` called twice | Identical output both times |
| `ErrorOrderIndependent` | `Errors: [{"text":"b"},{"text":"a"}]` vs `[{"text":"a"},{"text":"b"}]`; same `Namespace`, `Kind`, `ParentObject` | Same fingerprint |
| `SameParentDifferentNames` | `Name: "pod-abc-1"` vs `Name: "pod-abc-2"`; same `Namespace`, `Kind`, `ParentObject`, `Errors` | Same fingerprint (Name not hashed) |
| `DifferentErrors` | Same `Namespace`, `Kind`, `ParentObject`; different error texts | Different fingerprint |
| `DifferentParents` | Same errors; `ParentObject: "deploy-a"` vs `ParentObject: "deploy-b"` | Different fingerprint |
| `DifferentNamespaces` | Same `Kind`, `ParentObject`, errors; `Namespace: "ns-a"` vs `Namespace: "ns-b"` | Different fingerprint |
| `DifferentKinds` | Same `Namespace`, `ParentObject`, errors; `Kind: "Pod"` vs `Kind: "Deployment"` | Different fingerprint |
| `EmptyErrors` | `Errors: ""` vs `Errors: "[]"` | Same fingerprint (both yield zero texts) |
| `HTMLCharacters` | Error text containing `<`, `>`, `&` | Does not panic; produces stable output; output does not contain `\u003c` or similar HTML-escaped sequences (SetEscapeHTML is false) |
| `Returns64HexChars` | Any valid input | Output is exactly 64 lowercase hex characters |

---

## Tasks

- [ ] Write all 10 tests in `internal/domain/provider_test.go` (TDD — must fail before
  implementation)
- [ ] Implement `domain.FindingFingerprint` in `internal/domain/provider.go`
- [ ] Run tests — all must pass
- [ ] Run full test suite: `go test -timeout 120s -race ./...`

---

## Dependencies

**Depends on:** epic01-controller complete
**Blocks:** STORY_02

---

## Definition of Done

- [ ] `domain.FindingFingerprint` tests pass with `-race`
- [ ] Full test suite passes with `-race`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
