# Phase 6: GitHub App Private Key Isolation

**Date run:** 2026-02-24
**Cluster:** yes (v0.3.9, default namespace)

---

## 6.1 Code Verification

**grep on job.go:**
```
50:TOKEN=$(get-github-app-token.sh)
51:printf '%s' "$TOKEN" > /workspace/github-token
54: git clone "https://x-access-token:${TOKEN}@github.com/${GITOPS_REPO}.git"
80: Name: "GITHUB_APP_ID"      → secretKeyRef github-app
89: Name: "GITHUB_APP_INSTALLATION_ID"  → secretKeyRef github-app
98: Name: "GITHUB_APP_PRIVATE_KEY"      → secretKeyRef github-app
113/114: VolumeMounts for init container → shared-workspace
151-154: VolumeMounts for main container → shared-workspace only
189: Volume shared-workspace (emptyDir)
```

| Check | Result | Line | Notes |
|-------|--------|------|-------|
| `GITHUB_APP_PRIVATE_KEY` in init container Env only | PASS | 98–104 | Not present in main container Env block |
| `GITHUB_APP_ID` in init container Env only | PASS | 80–87 | Not present in main container Env block |
| `GITHUB_APP_INSTALLATION_ID` in init container Env only | PASS | 89–96 | Not present in main container Env block |
| Init container VolumeMounts: shared-workspace only | PASS | 113–114 | Plus github-app-secret volume |
| Main container VolumeMounts: shared-workspace only | PASS | 151–154 | No github-app-secret mount |
| Token written to shared emptyDir, not private key | PASS | 51 | `printf '%s' "$TOKEN" > /workspace/github-token` |
| `entrypoint-common.sh` reads from file, not env | PASS | line 64 | `gh auth login --with-token < /workspace/github-token` |
| Entrypoint does not log/echo token | PASS | reviewed | No `echo $TOKEN` or similar |

---

## 6.2 Live Verification

**Status:** Executed (via job spec inspection — pod was Completed)

Live job `mendabot-agent-0cd2345e0966` inspected:

**Init container env vars:**
```
GITHUB_APP_ID
GITHUB_APP_INSTALLATION_ID
GITHUB_APP_PRIVATE_KEY
GITOPS_REPO
```

**Main container env vars:**
```
FINDING_KIND
FINDING_NAME
FINDING_NAMESPACE
FINDING_PARENT
FINDING_ERRORS
FINDING_DETAILS
FINDING_FINGERPRINT
FINDING_SEVERITY
GITOPS_REPO
GITOPS_MANIFEST_ROOT
SINK_TYPE
AGENT_PROVIDER_CONFIG
AGENT_TYPE
```

| Check | Expected | Actual | Pass? |
|-------|----------|--------|-------|
| `GITHUB_APP_PRIVATE_KEY` absent from main container env | absent | absent | PASS |
| `GITHUB_APP_ID` absent from main container env | absent | absent | PASS |
| `GITHUB_APP_INSTALLATION_ID` absent from main container env | absent | absent | PASS |
| No `github-app-secret` volume in main container | absent | absent | PASS |

**Note:** Live exec into the completed pod was not possible (phase=Succeeded). Job spec inspection via `kubectl get job -o jsonpath` is authoritative for env var presence — it shows the exact template used to create the pods.

---

## Phase 6 Summary

All GitHub App private key isolation checks pass. The key is correctly limited to the init container. The main container has access only to the short-lived installation token via the shared emptyDir volume.

**Total findings:** 0
