# Story 02: Init Script — PAT / Non-GitHub Auth via `GITOPS_GIT_TOKEN`

**Epic:** [epic24-gitops-abstraction](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator using GitLab, Gitea, or a PAT instead of a GitHub App**, I want
to provide a plain git token via `GITOPS_GIT_TOKEN` so that the init container can clone
my GitOps repository without requiring a GitHub App installation.

---

## Background

The `initScript` constant in `internal/jobbuilder/job.go` (lines 47–58) currently
hardcodes the git clone to:

```bash
git clone "https://x-access-token:${TOKEN}@github.com/${GITOPS_REPO}.git" /workspace/repo
```

This embeds three assumptions:
1. `github.com` as the host — must be replaced with `${GITOPS_GIT_HOST}` (from STORY_01)
2. `x-access-token:` as the auth prefix — only valid for GitHub App / GitHub PAT tokens
3. GitHub App token exchange (`get-github-app-token.sh`) as the only auth path

This story replaces the single-path script with a two-branch script:
- **Branch A (PAT/generic token):** `GITOPS_GIT_TOKEN` is set — use it directly, no GitHub App exchange
- **Branch B (GitHub App, default):** `GITOPS_GIT_TOKEN` is absent — run `get-github-app-token.sh` as today

`GITOPS_GIT_TOKEN` flows from a Kubernetes Secret directly into the init container env.
The watcher never reads this value — it never appears in `config.Config`.

---

## Design

### Updated `initScript` constant (`internal/jobbuilder/job.go`)

Replace the current single-branch script with:

```bash
#!/bin/bash
set -euo pipefail

if [ -n "${GITOPS_GIT_TOKEN:-}" ]; then
  # PAT / generic HTTPS token path.
  # Token is provided directly — no GitHub App exchange needed.
  TOKEN="${GITOPS_GIT_TOKEN}"
  printf '%s' "$TOKEN" > /workspace/github-token
else
  # GitHub App path (default).
  # Exchange private key for a short-lived installation token.
  TOKEN=$(get-github-app-token.sh)
  printf '%s' "$TOKEN" > /workspace/github-token
fi

echo "Cloning repository: ${GITOPS_REPO} from ${GITOPS_GIT_HOST}"
if ! git clone "https://x-access-token:${TOKEN}@${GITOPS_GIT_HOST}/${GITOPS_REPO}.git" /workspace/repo; then
  echo "ERROR: Failed to clone ${GITOPS_REPO} from ${GITOPS_GIT_HOST}"
  echo "Check that the token has read access to this repository"
  exit 1
fi
```

Key points:
- `${GITOPS_GIT_HOST}` replaces the hardcoded `github.com` literal (set by STORY_01)
- `x-access-token:` is the auth prefix used by both GitHub App tokens and GitHub PATs;
  for GitLab HTTPS tokens the correct prefix is `oauth2:` or `<username>:`. The prefix
  is kept as `x-access-token:` for backward compatibility with GitHub users. GitLab and
  Gitea users who need a different prefix can set `GITOPS_GIT_TOKEN` to the full
  `username:token` form and rely on the init script formatting it as
  `https://username:token@<host>/...`. This is documented in the Helm chart values.
- `${GITOPS_GIT_TOKEN:-}` uses the `:-` default to avoid `set -u` errors when the var
  is absent. The `[ -n ... ]` test then handles the absent case cleanly.
- The token is still written to `/workspace/github-token` in both paths so that the main
  container's `gh auth login --with-token < /workspace/github-token` works unchanged.

### Init container env in `Build()` (`internal/jobbuilder/job.go`)

The init container already injects the three GitHub App secret vars. Add two more:

```go
{
    Name:  "GITOPS_GIT_HOST",
    Value: gitOpsGitHost(rjob),   // helper defined in STORY_01
},
{
    Name: "GITOPS_GIT_TOKEN",
    ValueFrom: &corev1.EnvVarSource{
        SecretKeyRef: &corev1.SecretKeySelector{
            LocalObjectReference: corev1.LocalObjectReference{Name: "gitops-git-token"},
            Key:                  "token",
            Optional:             ptr(true),   // absent = use GitHub App path
        },
    },
},
```

`Optional: ptr(true)` is critical: when the Secret `gitops-git-token` does not exist
(the common case for GitHub App users), Kubernetes sets the env var to an empty string
rather than failing the pod with `CreateContainerConfigError`.

### Helm chart (`charts/mendabot/values.yaml`)

Add optional PAT secret configuration under `gitops:`:

```yaml
gitops:
  repo: ""
  manifestRoot: ""
  tool: "flux"
  gitHost: "github.com"
  # Optional: name of a pre-existing Secret with key "token" containing a
  # git HTTPS token (PAT, GitLab token, etc.).
  # When set, the GitHub App Secret is not required.
  # When absent (default), the GitHub App Secret is used.
  gitTokenSecretName: ""
```

**`charts/mendabot/templates/deployment-watcher.yaml`** — the init container template
must conditionally inject the `GITOPS_GIT_TOKEN` env var only when
`gitops.gitTokenSecretName` is non-empty. This avoids a `CreateContainerConfigError`
when the secret does not exist:

```yaml
{{- if .Values.gitops.gitTokenSecretName }}
- name: GITOPS_GIT_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ .Values.gitops.gitTokenSecretName | quote }}
      key: token
      optional: true
{{- end }}
```

Alternatively, use `Optional: true` in the job builder as shown above and always inject
the env var reference — the `Optional: true` flag makes Kubernetes tolerate a missing
secret. Both approaches are valid; using `Optional: true` is simpler and avoids
conditional template logic.

### GitHub App env vars: remain unconditional

The three GitHub App env vars (`GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`,
`GITHUB_APP_PRIVATE_KEY`) remain in the init container env unconditionally. The script
only reads them in the GitHub App branch. When `GITOPS_GIT_TOKEN` is set, the script
takes the PAT branch and never calls `get-github-app-token.sh`, so the GitHub App vars
are simply unused. This avoids template conditionals and keeps `Build()` simpler.

If the GitHub App Secret `github-app` does not exist and `GITOPS_GIT_TOKEN` is also not
set, the pod will fail with `CreateContainerConfigError` — which is the correct and
expected failure mode (operator has not configured any auth).

---

## Files to modify

| File | Change |
|------|--------|
| `internal/jobbuilder/job.go` | Update `initScript`; add `GITOPS_GIT_HOST` + optional `GITOPS_GIT_TOKEN` to init container env |
| `internal/jobbuilder/job_test.go` | Add test cases: PAT branch, GitHub App branch, `GITOPS_GIT_HOST` substitution |
| `charts/mendabot/values.yaml` | Add `gitops.gitTokenSecretName` (empty default) |
| `charts/mendabot/templates/deployment-watcher.yaml` | Inject `GITOPS_GIT_HOST`; document PAT secret option |

No Go changes beyond `job.go` — this is a script-text and env-injection change only.

---

## Test cases for `job_test.go`

| Scenario | Setup | Expected |
|----------|-------|----------|
| Default (no PAT) | `GitOpsGitHost` empty, `GITOPS_GIT_TOKEN` not set | `initScript` contains `github.com`; GitHub App env vars present in init container |
| Custom host | `GitOpsGitHost: "gitlab.com"` | `initScript` references `gitlab.com`; clone URL uses `gitlab.com` |
| PAT secret reference | `gitTokenSecretName` set | Init container env contains `GITOPS_GIT_TOKEN` sourced from named Secret |
| Both env vars present | Both `GITOPS_GIT_HOST` and PAT | Both appear in init container env |

The `initScript` text itself is a string constant — test by checking that the rendered
init container `Args[0]` (the bash `-c` argument) contains the expected strings.

---

## Acceptance Criteria

- [ ] `initScript` uses `${GITOPS_GIT_HOST}` in the clone URL (no literal `github.com`)
- [ ] `initScript` branches on `GITOPS_GIT_TOKEN`: if non-empty, uses token directly; if absent/empty, runs `get-github-app-token.sh`
- [ ] In both branches, the token is written to `/workspace/github-token`
- [ ] Init container env always contains `GITOPS_GIT_HOST` (from `gitOpsGitHost()` helper)
- [ ] Init container env always contains `GITOPS_GIT_TOKEN` referenced from Secret `gitops-git-token` with `Optional: true`
- [ ] Existing GitHub App env vars remain unconditionally present in init container env
- [ ] Helm chart `values.yaml` has `gitops.gitTokenSecretName` (empty default)
- [ ] When `gitTokenSecretName` is empty and the Secret `gitops-git-token` is absent, Kubernetes does not fail the pod (`Optional: true`)
- [ ] All existing tests pass with `-race`

---

## Tasks

- [ ] TDD: write failing test cases in `job_test.go` for new init script behaviour
- [ ] Update `initScript` constant in `job.go`
- [ ] Add `GITOPS_GIT_HOST` and optional `GITOPS_GIT_TOKEN` to init container env in `Build()`
- [ ] Update `charts/mendabot/values.yaml`
- [ ] Update `charts/mendabot/templates/deployment-watcher.yaml`
- [ ] Run `go test -timeout 30s -race ./...` — all pass
- [ ] Run `go build ./...` — clean

---

## Dependencies

**Depends on:** STORY_01 (provides `gitOpsGitHost()` helper and `GitOpsGitHost` CRD field)
**Blocks:** Nothing (STORY_03 and STORY_04 are independent)

---

## Definition of Done

- [ ] All acceptance criteria satisfied
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
