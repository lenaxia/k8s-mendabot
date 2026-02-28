# Story 03: Prompt — `GITOPS_TOOL`-Conditional Investigation Step

**Epic:** [epic24-gitops-abstraction](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator running ArgoCD or plain Helm**, I want the investigation agent to
run diagnostics appropriate to my GitOps tool rather than running Flux-specific commands
that fail or produce misleading output on my cluster.

---

## Background

`charts/mechanic/files/prompts/core.txt` (lines 80–93) contains a Flux-specific
investigation step that the agent runs on every investigation:

```
STEP 5 — Understand the Flux/Helm state

  flux get all -n ${FINDING_NAMESPACE}
  kubectl get helmreleases -n ${FINDING_NAMESPACE}
  kubectl get kustomizations -n ${FINDING_NAMESPACE}
  flux logs --follow=false -n ${FINDING_NAMESPACE} --kind=HelmRelease --name=<release-name>
```

On a non-Flux cluster:
- `flux get all` exits non-zero with a connection-refused or "not found" error
- `kubectl get helmreleases` errors because the CRD does not exist
- `kubectl get kustomizations` errors because the CRD does not exist
- The agent wastes investigation time on command failures and may mis-diagnose the issue

This story replaces the hard-coded step with a `${GITOPS_TOOL}`-conditional block. The
agent reads the tool name from the env var and executes the appropriate diagnostic
commands.

This is a **shell script + prompt text change only** — zero Go code changes (STORY_01
handles the Go side).

---

## Design

### Updated Step 5 in `charts/mechanic/files/prompts/core.txt`

Replace lines 80–93 with:

```
STEP 5 — Understand the GitOps tool state

  The GitOps tool configured for this cluster is: ${GITOPS_TOOL}

  Run the diagnostics for your tool. Skip commands that are not applicable.

  If GITOPS_TOOL is "flux":

    flux get all -n ${FINDING_NAMESPACE}
    helm list -n ${FINDING_NAMESPACE}

    Search for HelmReleases and Kustomizations in the namespace. The HelmRelease
    name may differ from ${FINDING_PARENT} — look it up from the manifest files
    found in Step 4 or by listing:
    kubectl get helmreleases -n ${FINDING_NAMESPACE} 2>/dev/null || true
    kubectl get kustomizations -n ${FINDING_NAMESPACE} 2>/dev/null || true

    Once the HelmRelease name is known:
    flux logs --follow=false -n ${FINDING_NAMESPACE} --kind=HelmRelease --name=<release-name>
    helm get values <release-name> -n ${FINDING_NAMESPACE}

  If GITOPS_TOOL is "argocd":

    kubectl get applications -A 2>/dev/null || true
    kubectl get applications -n argocd \
      --field-selector metadata.name=<app-name> 2>/dev/null || true

    Find the ArgoCD Application managing ${FINDING_NAMESPACE}:
    kubectl get applications -A -o json \
      | jq '.items[] | select(.spec.destination.namespace == "${FINDING_NAMESPACE}") | .metadata.name'

    Inspect the application:
    kubectl describe application <app-name> -n argocd
    helm list -n ${FINDING_NAMESPACE}

  If GITOPS_TOOL is "helm-only" or unset:

    helm list -n ${FINDING_NAMESPACE}
    helm history <release-name> -n ${FINDING_NAMESPACE}
    helm get values <release-name> -n ${FINDING_NAMESPACE}
    helm get manifest <release-name> -n ${FINDING_NAMESPACE} | kubectl diff -f - 2>/dev/null || true
```

### Step title update (line 77 in `core.txt`)

The `=== ENVIRONMENT ===` section lists available tools. Update:

```
- All tools available: kubectl, helm, flux, argocd, talosctl, kustomize, gh, git,
  jq, yq, kubeconform, stern, sops, age.
- The active GitOps tool is: ${GITOPS_TOOL}
- Use the GitOps tool commands in Step 5 that match ${GITOPS_TOOL}.
```

### `entrypoint-common.sh` — add `GITOPS_TOOL` to envsubst

`docker/scripts/entrypoint-common.sh` line 105:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
```

Add `${GITOPS_TOOL}` to the list:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}${GITOPS_TOOL}'
```

Without this, `${GITOPS_TOOL}` is left as a literal string in the rendered prompt and
the agent sees the unexpanded variable, not the value.

### `charts/mechanic/files/prompts/opencode.txt` — tool listing

Update the tool availability line to list both Flux and ArgoCD, and note that the active
tool is set via `${GITOPS_TOOL}`:

```
- Use the shell tool to run kubectl, helm, flux, argocd, gh, git, and other CLI tools.
  The active GitOps deployment tool is ${GITOPS_TOOL} — use that tool's commands in Step 5.
```

---

## Files to modify

| File | Change |
|------|--------|
| `charts/mechanic/files/prompts/core.txt` | Replace Step 5 with `GITOPS_TOOL`-conditional block; update tool listing in `=== ENVIRONMENT ===` |
| `charts/mechanic/files/prompts/opencode.txt` | Update tool listing to include `argocd`; note active tool |
| `docker/scripts/entrypoint-common.sh` | Add `${GITOPS_TOOL}` to the `VARS` envsubst list |

No Go code changes. No Dockerfile changes.

---

## Exact diff: `core.txt` Step 5 section

**Before (lines 77–93):**
```
- All tools available: kubectl, helm, flux, talosctl, kustomize, gh, git,
  jq, yq, kubeconform, stern, sops, age.

...

STEP 5 — Understand the Flux/Helm state

  flux get all -n ${FINDING_NAMESPACE}
  helm list -n ${FINDING_NAMESPACE}

  Search for HelmReleases and Kustomizations in the namespace. The HelmRelease name
  may differ from ${FINDING_PARENT} — look it up from the manifest files found in
  Step 4 or by listing:
  kubectl get helmreleases -n ${FINDING_NAMESPACE}
  kubectl get kustomizations -n ${FINDING_NAMESPACE}

  Once the HelmRelease name is known:
  flux logs --follow=false -n ${FINDING_NAMESPACE} --kind=HelmRelease --name=<release-name>
  helm get values <release-name> -n ${FINDING_NAMESPACE}
```

**After:**
```
- All tools available: kubectl, helm, flux, argocd, talosctl, kustomize, gh, git,
  jq, yq, kubeconform, stern, sops, age.
- The active GitOps tool is: ${GITOPS_TOOL}
- Use the GitOps tool commands in Step 5 that match ${GITOPS_TOOL}.

...

STEP 5 — Understand the GitOps tool state

  The GitOps tool configured for this cluster is: ${GITOPS_TOOL}

  Run the diagnostics for your tool. Skip commands that are not applicable.

  If GITOPS_TOOL is "flux":

    flux get all -n ${FINDING_NAMESPACE}
    helm list -n ${FINDING_NAMESPACE}

    Search for HelmReleases and Kustomizations in the namespace. The HelmRelease
    name may differ from ${FINDING_PARENT} — look it up from the manifest files
    found in Step 4 or by listing:
    kubectl get helmreleases -n ${FINDING_NAMESPACE} 2>/dev/null || true
    kubectl get kustomizations -n ${FINDING_NAMESPACE} 2>/dev/null || true

    Once the HelmRelease name is known:
    flux logs --follow=false -n ${FINDING_NAMESPACE} --kind=HelmRelease --name=<release-name>
    helm get values <release-name> -n ${FINDING_NAMESPACE}

  If GITOPS_TOOL is "argocd":

    kubectl get applications -A 2>/dev/null || true
    kubectl get applications -n argocd \
      --field-selector metadata.name=<app-name> 2>/dev/null || true

    Find the ArgoCD Application managing ${FINDING_NAMESPACE}:
    kubectl get applications -A -o json \
      | jq '.items[] | select(.spec.destination.namespace == "${FINDING_NAMESPACE}") | .metadata.name'

    Inspect the application:
    kubectl describe application <app-name> -n argocd
    helm list -n ${FINDING_NAMESPACE}

  If GITOPS_TOOL is "helm-only" or unset:

    helm list -n ${FINDING_NAMESPACE}
    helm history <release-name> -n ${FINDING_NAMESPACE}
    helm get values <release-name> -n ${FINDING_NAMESPACE}
    helm get manifest <release-name> -n ${FINDING_NAMESPACE} | kubectl diff -f - 2>/dev/null || true
```

---

## Acceptance Criteria

- [ ] `core.txt` Step 5 title is "Understand the GitOps tool state" (not "Flux/Helm state")
- [ ] `core.txt` Step 5 contains conditional diagnostic blocks for `flux`, `argocd`, and `helm-only`
- [ ] Flux commands (`flux get all`, `kubectl get helmreleases`, `kubectl get kustomizations`,
      `flux logs`) appear only within the `flux` conditional block
- [ ] ArgoCD commands (`kubectl get applications`, `kubectl describe application`) appear
      in the `argocd` block
- [ ] Plain Helm commands appear in the `helm-only` block
- [ ] `core.txt` `=== ENVIRONMENT ===` section lists `argocd` alongside `flux` in the tools list
- [ ] `core.txt` `=== ENVIRONMENT ===` section references `${GITOPS_TOOL}` and instructs the
      agent to use the matching block in Step 5
- [ ] `opencode.txt` tool line lists both `flux` and `argocd`
- [ ] `entrypoint-common.sh` `VARS` list includes `${GITOPS_TOOL}`
- [ ] When `GITOPS_TOOL=flux`, `${GITOPS_TOOL}` in the rendered prompt resolves to `flux`
- [ ] When `GITOPS_TOOL=argocd`, `${GITOPS_TOOL}` in the rendered prompt resolves to `argocd`

---

## Tasks

- [ ] Edit `charts/mechanic/files/prompts/core.txt`: update `=== ENVIRONMENT ===` tool line; replace Step 5
- [ ] Edit `charts/mechanic/files/prompts/opencode.txt`: update tool listing line
- [ ] Edit `docker/scripts/entrypoint-common.sh`: add `${GITOPS_TOOL}` to `VARS`
- [ ] Manual review: render the prompt with `GITOPS_TOOL=flux` and verify it matches the old Step 5 semantically
- [ ] Manual review: render the prompt with `GITOPS_TOOL=argocd` and verify ArgoCD block is visible

---

## Dependencies

**Depends on:** STORY_01 (defines `GITOPS_TOOL` and injects it into the main container env)
**Blocks:** Nothing

---

## Definition of Done

- [ ] All acceptance criteria satisfied
- [ ] Prompt files are syntactically valid (no unterminated `${...}` tokens)
- [ ] `entrypoint-common.sh` change does not break any other variable substitutions
- [ ] `go test -timeout 30s -race ./...` still passes (no Go changes in this story)
