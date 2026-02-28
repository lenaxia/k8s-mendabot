# Worklog: Epic 02 — Job Builder Complete

**Date:** 2026-02-20
**Session:** Implement Build() in internal/jobbuilder, full TDD, 9-gap code review cycle
**Status:** Complete

---

## Objective

Implement `Build(*v1alpha1.RemediationJob) (*batchv1.Job, error)` in `internal/jobbuilder/job.go` — the pure function that constructs a fully-specified `batch/v1 Job` from a `RemediationJob` CRD. All 7 stories in epic02 are covered by this single function.

---

## Work Completed

### 1. Implemented `Build()` in `internal/jobbuilder/job.go`

Pure function, no side effects. Constructs:
- Job name: `mechanic-agent-<first-12-of-fingerprint>`
- Init container `git-token-clone`: same AgentImage, inline bash script from LLD §5, 3 secret env vars from `github-app` Secret, `GITOPS_REPO` literal, mounts `shared-workspace` + `github-app-secret` (read-only)
- Main container `mechanic-agent`: no Command override, 10 literal env vars + 3 secret refs from `llm-credentials`, mounts `shared-workspace` + `prompt-configmap` (read-only) — `github-app-secret` intentionally NOT mounted (security)
- Both containers: independent `SecurityContext` with `AllowPrivilegeEscalation=false`, drop ALL capabilities
- Pod security context: `RunAsNonRoot=true`, `RunAsUser=1000`
- 3 pod volumes: `shared-workspace` (emptyDir), `prompt-configmap` (ConfigMap `opencode-prompt`), `github-app-secret` (Secret `github-app`)
- Job settings: `BackoffLimit=1`, `ActiveDeadlineSeconds=900`, `TTLSecondsAfterFinished=86400`, `RestartPolicy=Never`
- 4 labels + 2 annotations + OwnerReference per spec

### 2. Replaced `job_test.go` with 28 tests

- 2 existing `New()` tests retained
- 21 tests from LLD §7 test table
- 5 new unhappy-path / coverage tests added during review:
  - `TestBuild_NilRJob`
  - `TestBuild_EmptyFingerprint`
  - `TestBuild_ShortFingerprint`
  - `TestBuild_SecurityContexts`
  - `TestBuild_PodSecurityContext`

### 3. Bugs prevented during code review + remediation

| Issue | Fix |
|-------|-----|
| Short fingerprint (< 12 chars) caused runtime panic | Replaced empty-string guard with `len < 12` guard |
| Shared `*SecurityContext` pointer between init + main containers | Inlined independent structs per container |
| initScript missing blank separator lines vs LLD §5 | Added blank lines to match spec verbatim |

---

## Key Decisions

1. **Single `len < 12` guard** replaces two checks (empty-string + short). `len("") == 0 < 12` so empty is covered. Simpler and more correct.

2. **No `ReadOnlyRootFilesystem` on main container** — the agent entrypoint writes to `/tmp/rendered-prompt.txt`. Setting this would break the agent without any benefit.

3. **`github-app-secret` not mounted in main container** — the main container (LLM agent) must not access the long-lived GitHub App private key. It reads only the short-lived installation token from `/workspace/github-token` written by the init container.

4. **All other builder params from `rjob.Spec`** — `AgentImage`, `AgentSA`, `GitOpsRepo`, `GitOpsManifestRoot` are read from the `RemediationJob` spec. `jobbuilder.Config` only holds `AgentNamespace`.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/jobbuilder/...  → PASS (28 tests)
go test -timeout 60s -race ./...                      → PASS (all 9 packages)
go build ./...                                        → clean
go vet ./...                                          → clean
```

---

## Next Steps

epic02 is complete. `jobbuilder.Builder` is already wired into `cmd/watcher/main.go` (line 77).

Next epic: **epic03-agent-image** (Dockerfile.agent + entrypoint scripts) or **epic04-deploy** (Kustomize manifests). Check `docs/BACKLOG/` for priority ordering.

---

## Files Modified

| File | Change |
|------|--------|
| `internal/jobbuilder/job.go` | Implemented `Build()`, `ptr` helper; added `len < 12` fingerprint guard; inlined SecurityContext per container; added blank lines to initScript |
| `internal/jobbuilder/job_test.go` | Replaced panic stub; added 26 new tests (21 from LLD + 5 new coverage tests) |
