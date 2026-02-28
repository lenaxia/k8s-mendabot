# Story 01: Config and CRD — `GITOPS_TOOL` and `GITOPS_GIT_HOST` fields

**Epic:** [epic24-gitops-abstraction](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 2 hours

---

## User Story

As a **cluster operator running ArgoCD or plain Helm** (not Flux), I want to configure
`GITOPS_TOOL` so that the investigation agent runs diagnostics appropriate to my GitOps
tool, and `GITOPS_GIT_HOST` so that the agent clones from my git host (GitLab, Gitea,
etc.) rather than assuming `github.com`.

---

## Background

Before STORY_02 and STORY_03 can inject the new env vars into agent Jobs, the vars must
exist in three places:

1. `internal/config/config.go` — so the watcher reads them from the environment
2. `api/v1alpha1/remediationjob_types.go` — so they are stored on the CRD and passed
   through to the job builder
3. The CRD YAML schemas — so the Kubernetes API server accepts the new fields
4. The Helm chart — so operators can set them via `values.yaml`

This story adds all four. STORY_02 and STORY_03 then consume the fields set here.

---

## Design

### 1. `internal/config/config.go`

Add two optional fields to `Config`:

```go
// GitOpsTool identifies the GitOps deployment tool in use.
// Controls which diagnostic commands the agent runs in Step 5 of the prompt.
// Accepted values: "flux", "argocd", "helm-only".
// Default: "flux" (backward-compatible).
GitOpsTool string // GITOPS_TOOL — default "flux"

// GitOpsGitHost is the git host domain used to build the clone URL.
// Default: "github.com" (backward-compatible).
GitOpsGitHost string // GITOPS_GIT_HOST — default "github.com"
```

Parsing rules in `FromEnv()`:

```go
cfg.GitOpsTool = os.Getenv("GITOPS_TOOL")
if cfg.GitOpsTool == "" {
    cfg.GitOpsTool = "flux"
}
switch cfg.GitOpsTool {
case "flux", "argocd", "helm-only":
    // valid
default:
    return Config{}, fmt.Errorf(
        "GITOPS_TOOL %q is not supported; accepted values: flux, argocd, helm-only",
        cfg.GitOpsTool,
    )
}

cfg.GitOpsGitHost = os.Getenv("GITOPS_GIT_HOST")
if cfg.GitOpsGitHost == "" {
    cfg.GitOpsGitHost = "github.com"
}
```

`GITOPS_GIT_HOST` accepts any non-empty string — no allowlist needed. Operators can set
it to `gitlab.com`, `gitea.example.com`, etc.

### 2. `api/v1alpha1/remediationjob_types.go`

Add two optional fields to `RemediationJobSpec`:

```go
// GitOpsTool identifies the GitOps deployment tool in use.
// Controls which diagnostic commands the agent runs.
// Accepted values: flux, argocd, helm-only.
// Defaults to "flux" when empty.
GitOpsTool string `json:"gitOpsTool,omitempty"`

// GitOpsGitHost is the git host domain used to build the clone URL.
// Defaults to "github.com" when empty.
GitOpsGitHost string `json:"gitOpsGitHost,omitempty"`
```

Both are `omitempty` — existing `RemediationJob` objects already in etcd will have empty
strings for these fields, which the job builder handles by using the defaults.

### 3. CRD YAML schemas

Two files require identical additions under `spec.versions[0].schema.openAPIV3Schema.properties.spec.properties`:

**`charts/mechanic/crds/remediationjob.yaml`**
**`internal/controller/testdata/crds/remediationjob_crd.yaml`**

Add:
```yaml
gitOpsTool:
  type: string
gitOpsGitHost:
  type: string
```

Neither field is added to the `required` list — both are optional.

### 4. Helm chart

**`charts/mechanic/values.yaml`** — add under the `gitops:` key:

```yaml
gitops:
  repo: ""
  manifestRoot: ""
  # GitOps deployment tool. Controls which diagnostic commands the agent runs.
  # Accepted values: flux, argocd, helm-only
  # Default: flux
  tool: "flux"
  # Git host domain for cloning the GitOps repository.
  # Default: github.com
  # Set to gitlab.com, gitea.example.com, etc. for non-GitHub hosts.
  gitHost: "github.com"
```

**`charts/mechanic/templates/deployment-watcher.yaml`** — inject the new env vars into
the watcher Deployment:

```yaml
- name: GITOPS_TOOL
  value: {{ .Values.gitops.tool | default "flux" | quote }}
- name: GITOPS_GIT_HOST
  value: {{ .Values.gitops.gitHost | default "github.com" | quote }}
```

### 5. Job builder (`internal/jobbuilder/job.go`)

`Build()` must propagate the new fields from `rjob.Spec` to the main container's env:

```go
{Name: "GITOPS_TOOL",     Value: gitOpsTool(rjob)},
{Name: "GITOPS_GIT_HOST", Value: gitOpsGitHost(rjob)},
```

Helper functions (unexported):

```go
func gitOpsTool(rjob *v1alpha1.RemediationJob) string {
    if rjob.Spec.GitOpsTool == "" {
        return "flux"
    }
    return rjob.Spec.GitOpsTool
}

func gitOpsGitHost(rjob *v1alpha1.RemediationJob) string {
    if rjob.Spec.GitOpsGitHost == "" {
        return "github.com"
    }
    return rjob.Spec.GitOpsGitHost
}
```

`GITOPS_GIT_HOST` is also needed by the init container (STORY_02 will use it to build the
clone URL). Add it to the init container env here so STORY_02 can use it without
modifying `Build()` again:

```go
{Name: "GITOPS_GIT_HOST", Value: gitOpsGitHost(rjob)},
```

### 6. `SourceProviderReconciler` — propagate fields to CRD

`internal/provider/provider.go` creates `RemediationJob` objects. It must set the two
new spec fields from `config.Config`:

```go
GitOpsTool:   r.Cfg.GitOpsTool,
GitOpsGitHost: r.Cfg.GitOpsGitHost,
```

---

## Files to modify

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `GitOpsTool`, `GitOpsGitHost` fields + parsing |
| `internal/config/config_test.go` | Add test cases for new fields |
| `api/v1alpha1/remediationjob_types.go` | Add `GitOpsTool`, `GitOpsGitHost` to `RemediationJobSpec` |
| `charts/mechanic/crds/remediationjob.yaml` | Add schema entries for new fields |
| `internal/controller/testdata/crds/remediationjob_crd.yaml` | Add schema entries for new fields |
| `charts/mechanic/values.yaml` | Add `gitops.tool`, `gitops.gitHost` |
| `charts/mechanic/templates/deployment-watcher.yaml` | Inject `GITOPS_TOOL`, `GITOPS_GIT_HOST` |
| `internal/jobbuilder/job.go` | Propagate fields to init + main container env |
| `internal/jobbuilder/job_test.go` | Add test cases for new env vars |
| `internal/provider/provider.go` | Set `GitOpsTool`, `GitOpsGitHost` when creating `RemediationJob` |

---

## Acceptance Criteria

- [ ] `GITOPS_TOOL` env var parsed by `FromEnv()`; defaults to `"flux"`; rejected if not one of `flux`, `argocd`, `helm-only`
- [ ] `GITOPS_GIT_HOST` env var parsed by `FromEnv()`; defaults to `"github.com"`; any non-empty string accepted
- [ ] `RemediationJobSpec.GitOpsTool` and `RemediationJobSpec.GitOpsGitHost` defined as optional fields
- [ ] Both CRD schemas updated with the new optional string fields (not required)
- [ ] Helm chart `values.yaml` has `gitops.tool` (default `"flux"`) and `gitops.gitHost` (default `"github.com"`)
- [ ] Watcher Deployment template injects `GITOPS_TOOL` and `GITOPS_GIT_HOST`
- [ ] `Build()` in `job.go` injects `GITOPS_TOOL` and `GITOPS_GIT_HOST` into both init and main container env
- [ ] Empty `GitOpsTool` in spec is treated as `"flux"` (default) in the job builder
- [ ] Empty `GitOpsGitHost` in spec is treated as `"github.com"` (default) in the job builder
- [ ] `SourceProviderReconciler` copies `GitOpsTool` and `GitOpsGitHost` from config to new `RemediationJob` spec
- [ ] All existing tests pass with `-race`
- [ ] New config test cases cover: missing env vars (defaults applied), `GITOPS_TOOL=argocd`, `GITOPS_TOOL=invalid` (error), `GITOPS_GIT_HOST=gitlab.com`

---

## Tasks

- [ ] TDD: write failing tests in `config_test.go` and `job_test.go`
- [ ] Add `GitOpsTool`, `GitOpsGitHost` to `Config` struct and `FromEnv()`
- [ ] Add `GitOpsTool`, `GitOpsGitHost` to `RemediationJobSpec`
- [ ] Update both CRD YAML schemas
- [ ] Update Helm chart `values.yaml` and `deployment-watcher.yaml`
- [ ] Update `Build()` in `job.go` (init + main container env, default helpers)
- [ ] Update `SourceProviderReconciler` to propagate fields
- [ ] Run `go test -timeout 30s -race ./...` — all pass
- [ ] Run `go build ./...` — clean

---

## Dependencies

**Depends on:** Nothing (foundational story for this epic)
**Blocks:** STORY_02 (uses `GITOPS_GIT_HOST` in init script), STORY_03 (uses `GITOPS_TOOL` in prompt envsubst)

---

## Definition of Done

- [ ] All acceptance criteria above satisfied
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
