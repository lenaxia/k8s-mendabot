# Phase 6: GitHub App Private Key Isolation

**Date run:**
**Reviewer:**

---

## 6.1 Code Verification

**Init container volume mounts:**
```bash
grep -n 'github-app' internal/jobbuilder/job.go
```
```
<!-- paste output -->
```

**Init container env vars:**
```bash
grep -n 'GITHUB_APP' internal/jobbuilder/job.go
```
```
<!-- paste output — all GITHUB_APP_* must appear only in initContainer Env block -->
```

**Main container volume mounts (manual review):**
```
<!-- paste the VolumeMounts block for the main container from job.go -->
```

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `github-app-secret` volume in init container VolumeMounts | pass / fail | | |
| `GITHUB_APP_PRIVATE_KEY` in init container Env only | pass / fail | | |
| `GITHUB_APP_ID` in init container Env only | pass / fail | | |
| `GITHUB_APP_INSTALLATION_ID` in init container Env only | pass / fail | | |
| Main container VolumeMounts has no `github-app-secret` reference | pass / fail | | |
| Shared emptyDir carries only the short-lived token | pass / fail | | |

**Findings from code review:** (none / list → add to findings.md)

---

## 6.2 Live Verification

**Status:** Executed / SKIPPED — reason: ______

**Agent pod used:**
```
<!-- pod name -->
```

**Main container env check:**
```bash
kubectl exec -n mechanic "$AGENT_POD" -c mechanic-agent -- env | grep -i github
```
```
<!-- paste output — must NOT contain GITHUB_APP_PRIVATE_KEY -->
```

**Main container mount check:**
```bash
kubectl exec -n mechanic "$AGENT_POD" -c mechanic-agent -- ls /secrets/ 2>&1
```
```
<!-- paste output — expected: "No such file or directory" or empty -->
```

**Workspace contents:**
```bash
kubectl exec -n mechanic "$AGENT_POD" -c mechanic-agent -- ls /workspace/
```
```
<!-- paste output — should show github-token and repo/, NOT the private key -->
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| `GITHUB_APP_PRIVATE_KEY` absent from main container env | absent | | |
| `/secrets/github-app` not mounted in main container | absent | | |
| `/workspace/github-token` present | present | | |
| Private key file not in `/workspace/` | absent | | |

---

## Phase 6 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
