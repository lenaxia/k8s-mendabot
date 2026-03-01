# Worklog: epic29 STORY_06 — Kyverno ClusterPolicy + jobbuilder security hardening

**Date:** 2026-02-28
**Session:** Implement STORY_06: Kyverno optional ClusterPolicy and jobbuilder ReadOnlyRootFilesystem + /tmp emptyDir
**Status:** Complete

---

## Objective

Implement STORY_06 of epic29-agent-hardening:

1. Go (TDD): Add `ReadOnlyRootFilesystem: true`, `RunAsNonRoot: true` to the main agent container `SecurityContext` in `internal/jobbuilder/job.go`, plus a dedicated `/tmp` emptyDir volume and VolumeMount.
2. Helm: Add `agent.kyvernoPolicy.enabled` and `agent.kyvernoPolicy.allowedImagePrefix` to `charts/mechanic/values.yaml`. Create `charts/mechanic/templates/kyverno-policy-agent.yaml` with 7 rules covering access control (A), write denial (B), pod security (C), and curl audit (D).

---

## Work Completed

### 1. Go — jobbuilder TDD

**Tests added to `internal/jobbuilder/job_test.go`:**
- Updated `TestBuild_SecurityContexts` (line 605–607): changed assertion from "ReadOnlyRootFilesystem must not be set" to "must be set to true". Added RunAsNonRoot assertion.
- `TestBuild_TmpVolume_Present`: verifies `tmp` emptyDir volume exists in pod spec.
- `TestBuild_TmpVolumeMount_MainContainer`: verifies `tmp` VolumeMount at `/tmp` on main container.
- `TestBuild_WorkspaceVolume_StillPresent`: regression — shared-workspace emptyDir and /workspace mount still present.
- `TestBuild_TmpVolume_PresentWhenDryRun`: verifies `/tmp` volume+mount present when DryRun=true.
- `TestBuild_TmpVolume_PresentWhenHardenKubectl`: verifies `/tmp` volume+mount present when HardenAgentKubectl=true.

All 6 new/updated tests failed before implementation (TDD red phase confirmed).

**Changes to `internal/jobbuilder/job.go`:**
- Main container `SecurityContext`: added `ReadOnlyRootFilesystem: ptr(true)` and `RunAsNonRoot: ptr(true)`.
- Main container `VolumeMounts`: appended `{Name: "tmp", MountPath: "/tmp"}`.
- `volumes` slice: appended `{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}` (always-on, unconditional).

### 2. Helm chart — values

Added to `charts/mechanic/values.yaml` under `agent:`:
- `kyvernoPolicy.enabled: false`
- `kyvernoPolicy.allowedImagePrefix: "ghcr.io/lenaxia/mechanic-agent"`
- Full inline comments explaining both fields, Kyverno v1.9+ requirement, and the CRD-not-found failure mode.

Added matching entries to `charts/mechanic/values.schema.json` under the `agent` object:
- `hardenKubectl`: boolean (was missing from schema — caused schema validation failures)
- `extraRedactPatterns`: array of string (was missing from schema)
- `kyvernoPolicy`: object with `enabled` (boolean) and `allowedImagePrefix` (string)

### 3. Helm chart — ClusterPolicy template

Created `charts/mechanic/templates/kyverno-policy-agent.yaml` with:
- Top-level `{{- if .Values.agent.kyvernoPolicy.enabled }}` guard (zero resources when disabled).
- `spec.background: false` (admission-only enforcement).
- 7 rules using per-rule `validationFailureAction`:
  - `deny-agent-secret-read` (Enforce): denies GET/LIST/WATCH on Secrets by agent SA.
  - `deny-agent-pod-exec` (Enforce): denies Pod/exec by agent SA.
  - `deny-agent-pod-portforward` (Enforce): denies Pod/portforward by agent SA.
  - `deny-agent-writes` (Enforce): denies mutating verbs on all resources except RemediationJob/RemediationJob/status.
  - `restrict-agent-image` (Enforce): conditional on `allowedImagePrefix` non-empty; enforces image prefix on mechanic-watcher-managed Jobs.
  - `enforce-agent-pod-security` (Enforce): validates readOnlyRootFilesystem, runAsNonRoot, allowPrivilegeEscalation=false, capabilities.drop=ALL on mechanic-watcher-managed Pods.
  - `audit-agent-direct-api-calls` (Audit): fires in PolicyReport when agent SA accesses resources outside ClusterRole allowlist.
- Used `mechanic.agentSAName` helper (produces `<fullname>-agent`) — the delegation prompt referenced a non-existent `mechanic.agentName` helper.
- JMESPath escape pattern `{{ "{{" }} request.operation {{ "}}" }}` produces literal `{{ request.operation }}` in rendered output.

---

## Key Decisions

1. **`mechanic.agentSAName` vs. `mechanic.agentName`**: The delegation prompt referenced `mechanic.agentName` which does not exist in `_helpers.tpl`. The correct helper is `mechanic.agentSAName` which produces `<fullname>-agent`. Used the correct helper.

2. **Schema file update required**: `values.schema.json` had `additionalProperties: false` on the `agent` object but was missing `hardenKubectl`, `extraRedactPatterns` (added in prior stories) and the new `kyvernoPolicy`. Without the schema update, `helm template` returned a validation error. Updated all three.

3. **`/tmp` volume always-on**: The `/tmp` emptyDir is added unconditionally (not behind a flag). This is correct — `ReadOnlyRootFilesystem: true` requires `/tmp` to be writable at all times, not just in dry-run or hardened mode.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/jobbuilder/...   # PASS (5 new tests + 1 updated)
go test -timeout 30s -race ./...                       # 18 packages: all PASS

helm template test-release charts/mechanic/ \
  --set agent.kyvernoPolicy.enabled=false \
  --set gitops.repo=org/repo \
  --set gitops.manifestRoot=k8s/ \
  | grep -c "ClusterPolicy"
# output: 0

helm template test-release charts/mechanic/ \
  --set agent.kyvernoPolicy.enabled=true \
  --set gitops.repo=org/repo \
  --set gitops.manifestRoot=k8s/ \
  | grep "kind:"
# includes: kind: ClusterPolicy

helm template test-release charts/mechanic/ \
  --set agent.kyvernoPolicy.enabled=true \
  --set 'agent.kyvernoPolicy.allowedImagePrefix=' \
  --set gitops.repo=org/repo \
  --set gitops.manifestRoot=k8s/ \
  | grep "restrict-agent-image"
# output: (empty — rule correctly skipped)

helm lint charts/mechanic/ --set gitops.repo=org/repo --set gitops.manifestRoot=k8s/
# 1 chart(s) linted, 0 chart(s) failed
```

---

## Next Steps

- STORY_06 complete. Epic 29 stories 01–06 are now implemented.
- Manual verification items from the DoD (runtime check that opencode does not write to root filesystem at startup, policy admission tests with `curl`) remain as operational checks.
- Update epic29 README status if needed.

---

## Files Modified

- `internal/jobbuilder/job.go` — added ReadOnlyRootFilesystem, RunAsNonRoot, /tmp VolumeMount, tmp emptyDir volume
- `internal/jobbuilder/job_test.go` — updated TestBuild_SecurityContexts; added 5 new tests
- `charts/mechanic/values.yaml` — added agent.kyvernoPolicy.enabled and agent.kyvernoPolicy.allowedImagePrefix
- `charts/mechanic/values.schema.json` — added hardenKubectl, extraRedactPatterns, kyvernoPolicy to agent schema
- `charts/mechanic/templates/kyverno-policy-agent.yaml` — new file, 7-rule ClusterPolicy
