# Story: JobBuilder — inject DRY_RUN env var and annotation

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 0.75 hours

---

## User Story

As a **cluster operator** using dry-run mode, I want the agent Job to receive a `DRY_RUN=true`
environment variable and a Job-level annotation so that the agent script knows not to create
any PRs, and so the reconciler can identify dry-run Jobs without re-reading the
`RemediationJob` spec.

---

## Background

`internal/jobbuilder/job.go` exposes a single method:

```go
func (b *Builder) Build(rjob *v1alpha1.RemediationJob, correlatedFindings []v1alpha1.FindingSpec) (*batchv1.Job, error)
```

The `Builder` struct holds a local `Config` (of type `jobbuilder.Config`, **not**
`config.Config`):

```go
type Config struct {
    AgentNamespace string
    AgentType      config.AgentType
    TTLSeconds     int32
}
```

The `jobbuilder.Config` is constructed in `main.go` from the `config.Config`. To carry the
dry-run flag through, `jobbuilder.Config` must gain a `DryRun bool` field as the fourth
field, which is populated from `config.Config.DryRun` at construction time.

The main container is named `"mendabot-agent"`. Its `Env` slice is built inline and
currently ends with `AGENT_TYPE`. The env var `DRY_RUN=true` must be appended to the main
container's `Env` slice — and only when `b.cfg.DryRun == true`. The init container
(`git-token-clone`) must **not** receive `DRY_RUN`.

The annotation key is `mendabot.io/dry-run` with value `"true"`. It is added to the Job's
`ObjectMeta.Annotations` map. The existing annotations (lines 244–247 of `job.go`) are:

```go
Annotations: map[string]string{
    "remediation.mendabot.io/fingerprint-full": rjob.Spec.Fingerprint,
    "remediation.mendabot.io/finding-parent":   rjob.Spec.Finding.ParentObject,
},
```

When `b.cfg.DryRun == true`, a third entry is added:

```go
"mendabot.io/dry-run": "true",
```

---

## Exact Code Locations

| File | Current state | Change |
|------|--------------|--------|
| `internal/jobbuilder/job.go` | `type Config struct { AgentNamespace string; AgentType config.AgentType; TTLSeconds int32 }` | add `DryRun bool` as fourth field |
| `internal/jobbuilder/job.go` | `mainContainer.Env` slice ends at `{Name: "AGENT_TYPE", Value: ...}` | append `DRY_RUN=true` conditionally after slice definition |
| `internal/jobbuilder/job.go` | `Annotations` map literal (lines 244–247) | add `"mendabot.io/dry-run": "true"` entry conditionally |
| `internal/jobbuilder/job_test.go` | — | add five new test functions |
| `cmd/watcher/main.go` | construction of `jobbuilder.Config{}` | add `DryRun: cfg.DryRun` |

---

## Acceptance Criteria

- [ ] `jobbuilder.Config` gains `DryRun bool`
- [ ] When `b.cfg.DryRun == true`:
  - `job.Annotations["mendabot.io/dry-run"] == "true"`
  - The main container (`"mendabot-agent"`) has an env var `DRY_RUN` with value `"true"`
- [ ] When `b.cfg.DryRun == false` (the default):
  - `job.Annotations` does **not** contain key `"mendabot.io/dry-run"`
  - The main container does **not** have a `DRY_RUN` env var
- [ ] The init container (`"git-token-clone"`) never has a `DRY_RUN` env var,
  regardless of the flag
- [ ] `cmd/watcher/main.go` populates `jobbuilder.Config.DryRun` from `cfg.DryRun`
- [ ] `go test -race ./internal/jobbuilder/...` passes

---

## Implementation

### 1. Extend `jobbuilder.Config`

```go
type Config struct {
    AgentNamespace string
    AgentType      config.AgentType
    TTLSeconds     int32
    DryRun         bool
}
```

### 2. Append env var to main container (after the slice literal, before `volumes`)

```go
if b.cfg.DryRun {
    mainContainer.Env = append(mainContainer.Env, corev1.EnvVar{
        Name:  "DRY_RUN",
        Value: "true",
    })
}
```

### 3. Add annotation conditionally

The simplest approach is to build the annotation map with the two always-present keys, then
conditionally add the third:

```go
annotations := map[string]string{
    "remediation.mendabot.io/fingerprint-full": rjob.Spec.Fingerprint,
    "remediation.mendabot.io/finding-parent":   rjob.Spec.Finding.ParentObject,
}
if b.cfg.DryRun {
    annotations["mendabot.io/dry-run"] = "true"
}
```

Then reference `annotations` in the `ObjectMeta` literal instead of the inline map.

### 4. Update `cmd/watcher/main.go`

Wherever `jobbuilder.Config{AgentNamespace: cfg.AgentNamespace, AgentType: cfg.AgentType, TTLSeconds: int32(cfg.RemediationJobTTLSeconds)}` is constructed, change to:

```go
jobbuilder.Config{
    AgentNamespace: cfg.AgentNamespace,
    AgentType:      cfg.AgentType,
    TTLSeconds:     int32(cfg.RemediationJobTTLSeconds),
    DryRun:         cfg.DryRun,
}
```

---

## Test Cases

All new tests live in `internal/jobbuilder/job_test.go`. Use the existing `buildJob` helper
for the non-dry-run baseline; create a separate `buildDryRunJob` helper for dry-run tests:

```go
func buildDryRunJob(t *testing.T) *batchv1.Job {
    t.Helper()
    b, err := New(Config{AgentNamespace: "mendabot", DryRun: true})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    job, err := b.Build(testRJob, nil)
    if err != nil {
        t.Fatalf("Build: %v", err)
    }
    return job
}
```

| Test Name | Builder config | Assertion |
|-----------|---------------|-----------|
| `TestBuild_DryRun_AnnotationPresent` | `DryRun: true` | `job.Annotations["mendabot.io/dry-run"] == "true"` |
| `TestBuild_DryRun_EnvVarPresent` | `DryRun: true` | main container has `DRY_RUN=true` |
| `TestBuild_NoDryRun_AnnotationAbsent` | `DryRun: false` (default) | `job.Annotations` does not contain `"mendabot.io/dry-run"` |
| `TestBuild_NoDryRun_EnvVarAbsent` | `DryRun: false` (default) | main container has no `DRY_RUN` env var |
| `TestBuild_DryRun_InitContainerNoEnvVar` | `DryRun: true` | init container `"git-token-clone"` has no `DRY_RUN` env var |

Example test implementations:

```go
func TestBuild_DryRun_AnnotationPresent(t *testing.T) {
    job := buildDryRunJob(t)
    if got, ok := job.Annotations["mendabot.io/dry-run"]; !ok {
        t.Error("annotation mendabot.io/dry-run missing")
    } else if got != "true" {
        t.Errorf("annotation mendabot.io/dry-run = %q, want %q", got, "true")
    }
}

func TestBuild_DryRun_EnvVarPresent(t *testing.T) {
    job := buildDryRunJob(t)
    main := job.Spec.Template.Spec.Containers[0]
    val, ok := getEnv(main, "DRY_RUN")
    if !ok {
        t.Fatal("DRY_RUN env var missing from main container")
    }
    if val != "true" {
        t.Errorf("DRY_RUN = %q, want %q", val, "true")
    }
}

func TestBuild_NoDryRun_AnnotationAbsent(t *testing.T) {
    job := buildJob(t) // DryRun: false (default)
    if _, ok := job.Annotations["mendabot.io/dry-run"]; ok {
        t.Error("annotation mendabot.io/dry-run must not be present when DryRun=false")
    }
}

func TestBuild_NoDryRun_EnvVarAbsent(t *testing.T) {
    job := buildJob(t) // DryRun: false (default)
    main := job.Spec.Template.Spec.Containers[0]
    if _, ok := getEnv(main, "DRY_RUN"); ok {
        t.Error("DRY_RUN env var must not be present when DryRun=false")
    }
}

func TestBuild_DryRun_InitContainerNoEnvVar(t *testing.T) {
    job := buildDryRunJob(t)
    var init corev1.Container
    for _, c := range job.Spec.Template.Spec.InitContainers {
        if c.Name == "git-token-clone" {
            init = c
            break
        }
    }
    if _, ok := getEnv(init, "DRY_RUN"); ok {
        t.Error("DRY_RUN must not be injected into the git-token-clone init container")
    }
}
```

---

## Tasks

- [ ] Write all five test functions in `internal/jobbuilder/job_test.go` (TDD — verify they
  fail before making changes)
- [ ] Add `DryRun bool` to `jobbuilder.Config`
- [ ] Add conditional env var append in `Build` (main container only)
- [ ] Refactor annotation map to variable and add conditional third entry
- [ ] Update `cmd/watcher/main.go` to pass `DryRun: cfg.DryRun`
- [ ] Run `go test -race ./internal/jobbuilder/...` — all pass
- [ ] Run `go build ./...` — clean

---

## Dependencies

**Depends on:** STORY_01 (config — `cfg.DryRun` field must exist before `main.go` can pass it)
**Blocks:** STORY_04 (reconciler reads the `mendabot.io/dry-run` annotation to detect dry-run Jobs)

---

## Definition of Done

- [ ] All five new jobbuilder tests pass with `-race`
- [ ] Existing jobbuilder tests unchanged and still pass
- [ ] Full test suite passes: `go test -timeout 120s -race ./...`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
