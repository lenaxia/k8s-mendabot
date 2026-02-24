# Phase 6: GitHub App Private Key Isolation

**Date run:** 2026-02-23
**Reviewer:** OpenCode (automated review)

---

## 6.1 Code Verification

**Init container volume mounts:**
```bash
grep -n 'github-app' internal/jobbuilder/job.go
```
```
67:  {Name: "GITHUB_APP_ID", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: ...Name:"github-app"}}
73:  {Name: "GITHUB_APP_INSTALLATION_ID", ValueFrom: ...Name:"github-app"}
82:  {Name: "GITHUB_APP_PRIVATE_KEY", ValueFrom: ...Name:"github-app"}
100: {Name: "github-app-secret", MountPath: "/secrets/github-app"}
```

**Init container env vars:**
```bash
grep -n 'GITHUB_APP' internal/jobbuilder/job.go
```
```
67:  GITHUB_APP_ID          — SecretKeyRef, "github-app" — initContainer Env only
73:  GITHUB_APP_INSTALLATION_ID — SecretKeyRef, "github-app" — initContainer Env only
82:  GITHUB_APP_PRIVATE_KEY — SecretKeyRef, "github-app" — initContainer Env only
```

**Main container volume mounts:**
```
{Name: "shared-workspace", MountPath: "/workspace"}
{Name: "prompt-configmap",  MountPath: "/prompt"}
{Name: "agent-token",       MountPath: "/var/run/secrets/mendabot/serviceaccount"}
```

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `github-app-secret` volume in init container VolumeMounts | **pass** | 100–104 | Present only in initContainer VolumeMounts |
| `GITHUB_APP_PRIVATE_KEY` in init container Env only | **pass** | 82 | Only in initContainer.Env block |
| `GITHUB_APP_ID` in init container Env only | **pass** | 67 | Only in initContainer.Env block |
| `GITHUB_APP_INSTALLATION_ID` in init container Env only | **pass** | 73 | Only in initContainer.Env block |
| Main container VolumeMounts has no `github-app-secret` reference | **pass** | 167–182 | Not present |
| Shared emptyDir carries only the short-lived token | **pass** | initScript:38 | `printf '%s' "$TOKEN" > /workspace/github-token` — private key never written to shared volume |

**Findings from code review:** none

---

## 6.2 Live Verification

**Status:** SKIPPED — reason: no running cluster available

Deferred to next cluster-available review.

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| `GITHUB_APP_PRIVATE_KEY` absent from main container env | absent | SKIPPED | |
| `/secrets/github-app` not mounted in main container | absent | SKIPPED | |
| `/workspace/github-token` present | present | SKIPPED | |
| Private key file not in `/workspace/` | absent | SKIPPED | |

---

## Phase 6 Summary

**Total findings:** 0
**Findings added to findings.md:** none
**Code review result:** All six isolation checks PASS. Live verification deferred.
