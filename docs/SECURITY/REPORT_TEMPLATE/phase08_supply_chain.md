# Phase 8: Supply Chain Integrity

**Date run:**
**Reviewer:**

---

## 8.1 Docker Image Binary Checksum Coverage

For each binary in `Dockerfile.agent`, verify a SHA256 checksum step is present.

**Command:**
```bash
grep -E '(curl|sha256sum|sha256)' docker/Dockerfile.agent
```
```
<!-- paste output -->
```

| Binary | Download URL Pattern | Checksum Verified? | Method | Notes |
|--------|---------------------|-------------------|--------|-------|
| kubectl | dl.k8s.io | yes / no | sha256sum from dl.k8s.io | |
| helm | get.helm.sh | yes / no | sha256sum from get.helm.sh | |
| flux | github releases | yes / no | | |
| opencode | github releases | yes / no | | |
| talosctl | github releases | yes / no | | |
| kustomize | github releases | yes / no | | |
| yq | github releases | yes / no | | |
| stern | github releases | yes / no | | |
| age | github releases | yes / no | | |
| sops | github releases | yes / no | | |
| kubeconform | github releases | yes / no | | |
| gh | apt (GPG-signed repo) | yes (apt) / no | GPG key + signed apt repo | |

**Binaries without checksum verification:**
```
<!-- list any, or "none" -->
```

**Findings:** (none / list → add each to findings.md)

---

## 8.2 GitHub Actions Pin Audit

**Command:**
```bash
grep -r 'uses:' .github/workflows/
```
```
<!-- paste output -->
```

| Action | Current Ref | Pinned to SHA? | Trusted Org? | Notes |
|--------|------------|----------------|--------------|-------|
| | | | | |

**Actions not pinned to SHA:**
```
<!-- list any, or "none" -->
```

**Findings:** (none / list → add each to findings.md)

---

## 8.3 Base Image Currency

**Base images in use:**
```bash
grep 'FROM' docker/Dockerfile.agent docker/Dockerfile.watcher
```
```
<!-- paste output -->
```

**Trivy scan — agent image:**

**Status:** Executed / SKIPPED — reason: ______

```bash
docker build -f docker/Dockerfile.agent -t mendabot-agent:review-scan . 2>&1 | tail -5
trivy image --severity HIGH,CRITICAL mendabot-agent:review-scan
```
```
<!-- paste output, or reference raw/trivy-agent.txt -->
```

**Trivy scan — watcher image:**

**Status:** Executed / SKIPPED — reason: ______

```bash
docker build -f docker/Dockerfile.watcher -t mendabot-watcher:review-scan . 2>&1 | tail -5
trivy image --severity HIGH,CRITICAL mendabot-watcher:review-scan
```
```
<!-- paste output, or reference raw/trivy-watcher.txt -->
```

**HIGH/CRITICAL CVEs found:**

| CVE | Package | Severity | Fixed In | Notes |
|-----|---------|----------|---------|-------|
| | | | | |

**Findings:** (none / list → add each to findings.md)

---

## 8.4 Go Module Integrity

```bash
go mod verify
```
```
<!-- paste output — should be "all modules verified" -->
```

**Result:** PASS / FAIL

**Findings:** (none / list → add each to findings.md)

---

## Phase 8 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
