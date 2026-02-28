# Worklog: Epic 11 Story 01 — Schema Foundations (ChainDepth)

**Date:** 2026-02-25
**Session:** Add ChainDepth field to domain.Finding, FindingSpec, CRD testdata, and wire provider mapping
**Status:** Complete

---

## Objective

Implement STORY_01 of epic11-self-remediation-cascade: add the `ChainDepth` field as a
typed integer that flows from `domain.Finding` → `api/v1alpha1.FindingSpec` → CRD schema
→ `internal/provider/provider.go` mapping. This field is the prerequisite for STORY_02
(detection logic) and all downstream stories in the epic.

---

## Work Completed

### 1. TDD — unit test written first (`internal/domain/provider_test.go`)

Added `t.Run("ChainDepthDoesNotAffectFingerprint", ...)` inside `TestFindingFingerprint`.
The test creates two Findings identical except for `ChainDepth` (0 vs 5) and asserts both
produce the same fingerprint. Confirmed the test failed to compile before the field was
added.

### 2. TDD — integration test written first (`internal/controller/integration_test.go`)

Added `TestRemediationJobChainDepthRoundTrip` at the end of the file. It creates a
`RemediationJob` with `Spec.Finding.ChainDepth = 2` via `k8sClient.Create`, reads it
back via `k8sClient.Get`, and asserts the value is preserved. Confirmed the test failed
(returned 0 instead of 2) before the CRD testdata was updated.

### 3. `internal/domain/provider.go` — `ChainDepth int` added to `Finding`

Added field after `Severity` with a comment explaining it is not part of the
fingerprint. No other changes to this file.

### 4. `api/v1alpha1/remediationjob_types.go` — `ChainDepth int32` added to `FindingSpec`

Added `ChainDepth int32 \`json:"chainDepth,omitempty"\`` after `Details`. Marked
`+optional`. No other changes.

### 5. `testdata/crds/remediationjob_crd.yaml` — `chainDepth: {type: integer}` added

Inserted after `details: {type: string}` inside the `finding.properties` block
(line 78). This is required for envtest to preserve the field — the API server silently
strips unknown fields without it.

### 6. `internal/provider/provider.go` — `ChainDepth` mapped in `FindingSpec` literal

Added `ChainDepth: int32(finding.ChainDepth),` after `Details: finding.Details,` at
line 409. The explicit `int32()` cast handles the domain→CRD type narrowing (both
are non-negative in practice; chain depth will never exceed int32 range).

---

## Key Decisions

- **ChainDepth not in fingerprint**: Confirmed correct. Two findings from different
  cascade depths that affect the same parent resource are the same problem. Including
  chain depth in the fingerprint would create duplicate RemediationJobs for the same
  root cause at different cascade levels — exactly the dedup bug STORY_02 must prevent.

- **`int` in domain, `int32` in CRD type**: Domain types use platform-native `int`;
  CRD/API types use explicitly-sized `int32` (Kubernetes convention, JSON serialisation
  safety). The cast `int32(finding.ChainDepth)` is explicit and correct.

- **DeepCopyInto copies ChainDepth correctly**: `FindingSpec` is a plain value struct —
  all fields are value types (string, int32). The line `out.Spec = in.Spec` in
  `DeepCopyInto` performs a struct assignment, which copies all value fields including
  `ChainDepth` by value. No pointer was introduced, so no additional copy code is needed.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race -count=1 ./...
```

All 13 packages pass. Full output:

```
ok  github.com/lenaxia/k8s-mechanic/api/v1alpha1           1.099s
ok  github.com/lenaxia/k8s-mechanic/cmd/redact             2.034s
ok  github.com/lenaxia/k8s-mechanic/cmd/watcher            1.295s
ok  github.com/lenaxia/k8s-mechanic/internal/config        1.254s
ok  github.com/lenaxia/k8s-mechanic/internal/controller    11.427s
ok  github.com/lenaxia/k8s-mechanic/internal/domain        1.322s
ok  github.com/lenaxia/k8s-mechanic/internal/jobbuilder    1.271s
ok  github.com/lenaxia/k8s-mechanic/internal/logging       1.057s
ok  github.com/lenaxia/k8s-mechanic/internal/provider      9.974s
ok  github.com/lenaxia/k8s-mechanic/internal/provider/native 1.558s
ok  github.com/lenaxia/k8s-mechanic/internal/readiness     1.073s
ok  github.com/lenaxia/k8s-mechanic/internal/readiness/llm 1.485s
ok  github.com/lenaxia/k8s-mechanic/internal/readiness/sink 1.361s
```

`go build ./...` — clean.
`go vet ./...` — clean.

---

## Next Steps

STORY_01 is complete. STORY_02 (detection logic: populate ChainDepth in provider(s)
by inspecting the source object for signs it was triggered by a prior remediation)
can now be started. Read STORY_02 from `docs/BACKLOG/epic11-self-remediation-cascade/`
before beginning.

---

## Files Modified

- `internal/domain/provider.go` — `ChainDepth int` field added to `Finding` struct
- `api/v1alpha1/remediationjob_types.go` — `ChainDepth int32` field added to `FindingSpec`
- `testdata/crds/remediationjob_crd.yaml` — `chainDepth: {type: integer}` added to `finding.properties`
- `internal/provider/provider.go` — `ChainDepth: int32(finding.ChainDepth)` mapping added
- `internal/domain/provider_test.go` — `ChainDepthDoesNotAffectFingerprint` sub-test added
- `internal/controller/integration_test.go` — `TestRemediationJobChainDepthRoundTrip` added
- `docs/WORKLOGS/0084_2026-02-25_epic11-story01-schema-foundations.md` — this file
