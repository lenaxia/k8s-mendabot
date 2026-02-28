# Story 04: Deploy Manifest — Secret Mount + PR_AUTO_CLOSE

## Status: Complete

## Goal

Mount the GitHub App credentials Secret into the watcher Deployment (Helm chart and
any Kustomize overlays), and add the `PR_AUTO_CLOSE` and GitHub App env vars so the
watcher can exchange App credentials for an installation token at runtime.

## Background

The agent Job already receives the GitHub App credentials via a Secret (`secret-github-app`)
through env vars (`GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PRIVATE_KEY`).
The watcher Deployment currently does NOT mount this Secret — it has no need to call
GitHub today. After epic26, the watcher needs these credentials to auto-close PRs.

The Helm chart (`charts/mechanic/templates/deployment-watcher.yaml`) is the canonical
deployment path. Kustomize overlays in `deploy/overlays/` are secondary.

## Acceptance Criteria

- [x] `charts/mechanic/templates/deployment-watcher.yaml` adds:
      - `PR_AUTO_CLOSE` env var from `values.yaml`
      - `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PRIVATE_KEY` env
        vars sourced from `secretRef: { name: mechanic-github-app }` (same Secret the
        agent uses)
- [x] `charts/mechanic/values.yaml` adds `watcher.prAutoClose: true`
- [x] Secret mount is **required** (no `optional: true`): watcher pod will not start if
      the Secret is absent — this is a hard misconfiguration, not a graceful degradation
- [x] `main.go` wiring does NOT include a graceful fallback for missing credentials:
      if the Secret is mounted (pod started), the env vars are always present; if
      credential parsing fails, log at Error and exit non-zero (fail fast)
- [x] `go build ./...` succeeds
- [x] Helm template renders without errors: `helm template . | kubectl apply --dry-run=client -f -`

## Implementation Notes

### charts/mechanic/values.yaml

Add under `watcher:`:

```yaml
  # -- Automatically close open GitHub PRs/issues when the underlying finding resolves.
  # Set to false to disable auto-close and leave PRs open for manual review.
  prAutoClose: true
```

### charts/mechanic/templates/deployment-watcher.yaml

After the existing `DISABLE_CASCADE_CHECK` env var block, add:

```yaml
        - name: PR_AUTO_CLOSE
          value: {{ .Values.watcher.prAutoClose | quote }}
        envFrom:
        - secretRef:
            name: mechanic-github-app
```

Wait — `envFrom` must be a sibling of `env` on the container spec, not nested inside
`env`. The correct placement is at the container level:

```yaml
      containers:
      - name: watcher
        image: ...
        env:
          ... (existing env entries) ...
          - name: PR_AUTO_CLOSE
            value: {{ .Values.watcher.prAutoClose | quote }}
        envFrom:
        - secretRef:
            name: mechanic-github-app
```

The Secret `mechanic-github-app` must contain these keys (same as used by the agent):
- `GITHUB_APP_ID`
- `GITHUB_APP_INSTALLATION_ID`
- `GITHUB_APP_PRIVATE_KEY`

This is the same Secret already documented in the deploy README. No new Secret is
created — the watcher reuses the one the agent already uses.

**Required (not optional):** Do not add `optional: true`. If the Secret is missing,
Kubernetes will keep the Pod in `Pending` with an event clearly stating the reason.
This is the desired behaviour — a missing Secret is a misconfiguration, not a graceful
degradation scenario.

### cmd/watcher/main.go

Wire up `GitHubAppTokenProvider` and `GitHubSinkCloser` after reading `cfg`.

The Secret is a **required mount** — if the pod started, the env vars are always
present. Do not implement a graceful fallback for missing credentials. If credential
parsing fails (e.g. malformed PEM), log at Error level and exit non-zero immediately.
A misconfigured token provider that silently falls back to a no-op would mean
`PR_AUTO_CLOSE=true` has no visible effect, which is harder to diagnose than a
crash-loop.

```go
// Wire GitHub App token provider and sink closer.
// envFrom on the Deployment guarantees these vars are present when the pod starts.
var sinkCloser domain.SinkCloser
if cfg.PRAutoClose {
    appID, err := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
    if err != nil || appID <= 0 {
        setupLog.Error(err, "GITHUB_APP_ID is missing or invalid; cannot start with PR_AUTO_CLOSE=true")
        os.Exit(1)
    }
    installID, err := strconv.ParseInt(os.Getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
    if err != nil || installID <= 0 {
        setupLog.Error(err, "GITHUB_APP_INSTALLATION_ID is missing or invalid; cannot start with PR_AUTO_CLOSE=true")
        os.Exit(1)
    }
    privKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")))
    if err != nil {
        setupLog.Error(err, "GITHUB_APP_PRIVATE_KEY is missing or invalid; cannot start with PR_AUTO_CLOSE=true")
        os.Exit(1)
    }
    sinkCloser = &sinkhub.GitHubSinkCloser{
        TokenProvider: &igithub.GitHubAppTokenProvider{
            AppID:          appID,
            InstallationID: installID,
            PrivateKey:     privKey,
        },
    }
} else {
    sinkCloser = domain.NoopSinkCloser{}
}
```

Then pass `sinkCloser` when constructing each `SourceProviderReconciler`:

```go
SinkCloser: sinkCloser,
```

**Why no graceful fallback:** The previous draft included an `else` branch that fell
back to `NoopSinkCloser{}` when credentials were absent even with `PR_AUTO_CLOSE=true`.
That code was dead — if `envFrom.secretRef` has no `optional: true`, the pod never
starts without the Secret, so the env vars are always present when the code runs. The
fallback created a false impression of graceful degradation and masked misconfiguration.
Fail fast; let the operator fix the Secret.

## Files Touched

| File | Change |
|------|--------|
| `charts/mechanic/templates/deployment-watcher.yaml` | Add `PR_AUTO_CLOSE` env var; add `envFrom` for GitHub App Secret |
| `charts/mechanic/values.yaml` | Add `watcher.prAutoClose: true` |
| `cmd/watcher/main.go` | Wire `GitHubAppTokenProvider` + `GitHubSinkCloser`; pass `SinkCloser` to reconciler |

## TDD Sequence

This story is primarily manifest and wiring work, not business logic. There are no
unit tests for the YAML itself. Validate with:

```bash
helm template charts/mechanic | kubectl apply --dry-run=client -f -
go build ./...
```

The integration is verified end-to-end in the cluster after STORY_05 lands.
