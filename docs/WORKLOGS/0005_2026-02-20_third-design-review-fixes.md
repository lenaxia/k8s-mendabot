# Worklog: Third Design Review — Gap Analysis and Fixes

**Date:** 2026-02-20
**Session:** Exhaustive gap analysis across all design documents and backlog stories; fix all confirmed real issues
**Status:** Complete

---

## Objective

Perform a fresh systematic deep dive across all LLDs, HLD, backlog stories, and source files
to find remaining gaps, contradictions, and missing definitions. Assess each finding as
blocking or minor. Fix all confirmed real issues.

---

## Work Completed

### 1. Assessed 56 reported issues

Reviewed every issue against the actual document content. Categorised each as:
- **Real and fixed** — genuine inconsistency corrected
- **Overblown / not an issue** — technically fine on re-examination

### 2. CONTROLLER_LLD.md fixes

- **Issue 2:** Fixed syntax error `}for` → `}\nfor` in §7 main.go snippet
- **Issue 3/Issue 45:** Clarified §5.1 — `ResultReconciler` is the unexported concrete type
  created by `SourceProviderReconciler.SetupWithManager`. Added prose explanation; removed the
  misleading duplicate struct definition
- **Issue 4:** Fixed `syncPhaseFromJob` signature — removed unused `rjob` param; qualified
  return type as `v1alpha1.RemediationJobPhase`
- **Issue 42/56:** Rewrote §6.2 reconcile loop: step 2 now handles the Succeeded TTL deletion
  logic (check `CompletedAt + RemediationJobTTL` and delete if past deadline). Active-job
  count now uses `job.Status.Active > 0 OR (job.Status.Succeeded == 0 AND CompletionTime == nil)`
  instead of the incorrect `CompletionTime == nil` alone (which counted Failed jobs)
- **Issue 45:** Updated §11 integration test names to `TestSourceProviderReconciler_*`
  consistently with REMEDIATIONJOB_LLD

### 3. HLD.md fixes

- **Issue 6/8:** Data flow step 2 and dedup section now say "SourceProviderReconciler" not
  "ResultReconciler"
- **Issue 44:** §10 step 1 updated from `--search` to the correct `--json/--jq` exact
  `headRefName` filter, matching PROMPT_LLD §2

### 4. REMEDIATIONJOB_LLD.md fixes

- **Issue 7:** §3.1 step 3 label value corrected to `fp[:12]` (was using full fingerprint)
- **Issue 42:** §9 TTL section updated to reference CONTROLLER_LLD §6.2 as the authoritative
  location for the TTL logic

### 5. SINK_PROVIDER_LLD.md fix

- **Issue 9:** §4 completely rewritten — removed the claim that `sinkType` is a post-v1
  addition. It is already in v1. Section now documents the current state accurately.

### 6. PROVIDER_LLD.md fixes

- **Issue 10:** §5 code snippet now has proper error check on `SetupWithManager`
- **Issue 12:** HLD reference corrected from §4 to §5

### 7. DEPLOY_LLD.md fixes

- **Issue 31:** CRD YAML indentation fixed — `schema:` key moved to correct indentation level
  (was erroneously nested inside the last `additionalPrinterColumns` entry)
- **Issue 32:** Added minimum Kubernetes version note (1.25+ for CEL `x-kubernetes-validations`;
  1.29+ recommended for GA support)

### 8. AGENT_IMAGE_LLD.md fix

- **Issue 11:** HLD reference corrected from §9 to §10

### 9. WATCHER_IMAGE_LLD.md fix

- **Issue 43:** §9 security context now documents the Pod-level `seccompProfile: RuntimeDefault`
  that was present in DEPLOY_LLD but missing here

### 10. JOBBUILDER_LLD.md fixes (Issue 40 — security)

- **Removed `github-app-secret` volume mount from main container spec** — this is a security
  violation. The LLM agent must not have filesystem access to the GitHub App private key.
  Only the init container mounts the secret; the main container reads only the short-lived
  token from `/workspace/github-token`. Added explicit security rationale.
- Updated test table entry for `TestBuild_Volumes_AllPresent` to document the distinction
  between pod-level volumes (3) and main container mounts (2)

### 11. config.go + config_test.go fixes (Issue 15/16)

- Added `RemediationJobTTLSeconds int` field to `Config` struct
- Added `REMEDIATION_JOB_TTL_SECONDS` parsing in `FromEnv()` with default 604800
- Added 5 new tests: default value, explicit value, invalid string, zero value, negative
- All tests pass: `go test -timeout 30s -race ./internal/config/...` → ok

### 12. README-LLM.md fixes (Issues 33/34/35/36/37/38)

- Removed duplicate `api/v1alpha1/` block
- Added `crd-remediationjob.yaml`, `role-agent.yaml`, `rolebinding-agent.yaml` to deploy/kustomize listing
- Added `Dockerfile.watcher` and `agent-entrypoint.sh` to docker/ listing
- Changed "(7 modules)" to "(9 LLDs)"
- Changed opencode install method from "Official install script" to "Pinned GitHub release binary (not install script)"

### 13. Backlog fixes

- **epic00-foundation README:** Status updated to "In Progress"; STORY_01 marked Complete in table
- **epic00.1-interfaces README:** Removed `processedEntry` and `JobBuilderConfig` from Blocks and
  Success Criteria; replaced with current types (`SourceProvider`, `Finding`, `SourceRef`, `JobBuilder`)
- **epic00.1 STORY_01:** AddToScheme criterion now correctly specifies two separate functions
  for two separate API groups (`core.k8sgpt.ai` and `remediation.mechanic.io`)
- **epic00-foundation STORY_01:** Go version criterion relaxed from "1.24 or later" to "1.23 or later"
  (matches go.mod, Dockerfile, README-LLM.md technology stack)
- **epic02 STORY_01:** Removed the incorrect `domain.JobBuilderConfig` references; Config struct
  is `jobbuilder.Config` in the `internal/jobbuilder` package, not in `internal/domain`
- **epic01 STORY_06:** Provider loop corrected from `[]provider.SourceProvider{k8sgpt.NewProvider(...)}` to
  `[]domain.SourceProvider{&k8sgpt.K8sGPTProvider{}}`
- **epic01 STORY_05:** Completely rewritten — renamed from "Error-Filter Predicate" to
  "No-Error Filtering in ExtractFinding". Manager-level predicate replaced with
  `ExtractFinding()` nil-return pattern per CONTROLLER_LLD §5.3 and PROVIDER_LLD §8
- **epic01 STORY_03:** Stale annotation `opencode.io/result-name` replaced with
  `rjob.Spec.SourceResultRef.Name` lookup. Test names updated to `TestSourceProviderReconciler_*`.
  Test description "predicate filtered" → "ExtractFinding returns nil, nil"
- **epic02 STORY_04 + STORY_05:** `b.cfg.AgentImage` → `rjob.Spec.AgentImage`

---

## Key Decisions

1. **Active-job counting:** Use `job.Status.Active > 0 OR (job.Status.Succeeded == 0 AND CompletionTime == nil)` to count jobs consuming the concurrent slot. The original `CompletionTime == nil` was wrong because Failed jobs also have nil `CompletionTime`.

2. **`syncPhaseFromJob` signature:** The function does not need `rjob` as a parameter — it maps Job status to a phase value only. Callers apply the result. This is cleaner.

3. **github-app-secret not mounted in main container:** This is the correct security boundary. The agent is an LLM running potentially attacker-influenced code. Giving it access to the long-lived GitHub App private key is unacceptable. The short-lived token in `/workspace/github-token` has a 1-hour window; the private key has indefinite validity.

4. **Go version stays at 1.23:** go.mod, Dockerfile.watcher, and README-LLM.md all say 1.23. STORY_01's "1.24 or later" acceptance criterion was wrong. Changing the criterion rather than the go.mod.

5. **STORY_05 redesign:** The predicate approach was completely wrong per the LLDs. The story now correctly describes the `ExtractFinding` nil-return approach that was always the design intent.

---

## Blockers

None.

---

## Tests Run

```
go test -timeout 30s -race ./internal/config/...
```
Result: PASS (all tests including 5 new RemediationJobTTLSeconds tests)

---

## Next Steps

Design documents are now consistent. Implementation can begin with epic00-foundation:

1. **epic00-foundation/STORY_02** — Typed configuration: `config.go` and `config_test.go` are already done (completed as part of fixes). Mark STORY_02 Complete.
2. **epic00-foundation/STORY_03** — Structured logging: create `internal/logging/logging.go` with zap initialisation.
3. **epic00-foundation/STORY_04** — Vendored CRD types: create `api/v1alpha1/result_types.go`.
4. Then proceed to **epic00.1-interfaces** in story order.

---

## Files Modified

- `docs/DESIGN/lld/CONTROLLER_LLD.md`
- `docs/DESIGN/lld/REMEDIATIONJOB_LLD.md`
- `docs/DESIGN/lld/SINK_PROVIDER_LLD.md`
- `docs/DESIGN/lld/PROVIDER_LLD.md`
- `docs/DESIGN/lld/DEPLOY_LLD.md`
- `docs/DESIGN/lld/AGENT_IMAGE_LLD.md`
- `docs/DESIGN/lld/WATCHER_IMAGE_LLD.md`
- `docs/DESIGN/lld/JOBBUILDER_LLD.md`
- `docs/DESIGN/HLD.md`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `README-LLM.md`
- `docs/BACKLOG/epic00-foundation/README.md`
- `docs/BACKLOG/epic00-foundation/STORY_01_module_setup.md`
- `docs/BACKLOG/epic00.1-interfaces/README.md`
- `docs/BACKLOG/epic00.1-interfaces/STORY_01_domain_types.md`
- `docs/BACKLOG/epic01-controller/STORY_03_dedup_map.md`
- `docs/BACKLOG/epic01-controller/STORY_05_predicate.md`
- `docs/BACKLOG/epic01-controller/STORY_06_manager.md`
- `docs/BACKLOG/epic02-jobbuilder/STORY_01_builder_struct.md`
- `docs/BACKLOG/epic02-jobbuilder/STORY_04_init_container.md`
- `docs/BACKLOG/epic02-jobbuilder/STORY_05_main_container.md`
