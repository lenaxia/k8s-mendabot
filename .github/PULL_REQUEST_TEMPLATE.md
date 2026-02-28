## What does this PR do?

<!-- Describe the change and why it is needed. Link the relevant issue or
     backlog epic if applicable. -->

## How was it tested?

<!-- Paste the test commands you ran and their outcomes. -->

```
go test -timeout 60s -race ./...
# all N packages PASS
```

<!-- For Helm changes: -->
```
helm lint charts/mechanic/
helm template mechanic charts/mechanic/ --set gitops.repo=test/repo --set gitops.manifestRoot=k8s | head -50
```

<!-- For security-relevant changes (RBAC, Dockerfile, redact, prompt): -->
```
# Which phases of docs/SECURITY/PROCESS.md were re-run?
```

## Checklist

- [ ] Tests pass: `make test`
- [ ] Lint passes: `make lint-full`
- [ ] No secrets detected: `make lint-secrets`
- [ ] Worklog written in `docs/WORKLOGS/` (required for any meaningful change)
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] For security-relevant changes: relevant phases of `docs/SECURITY/PROCESS.md` re-run and findings documented
- [ ] For Helm changes: `helm lint --strict charts/mechanic/` passes
- [ ] For CRD changes: both `testdata/crds/` and `charts/mechanic/crds/` updated

## Worklog reference

<!-- docs/WORKLOGS/NNNN_YYYY-MM-DD_description.md -->
