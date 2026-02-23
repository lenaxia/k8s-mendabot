# Security Report: mendabot

<!-- Replace YYYY-MM-DD with the review completion date -->
**Report Date:** YYYY-MM-DD
**Reviewer:**
**Review Type:** Full / Partial (scope: _____________)
**Cluster Available:** yes / no
**CNI (NetworkPolicy Support):** yes / no / N/A
**Previous Report:** [link or "None"]
**Status:** Open / Closed

---

## Executive Summary

<!-- 3-5 sentences. What was reviewed, what was found, what is the overall security posture. -->

**Finding counts:**

| Severity | Open | Remediated | Accepted | Deferred |
|----------|------|-----------|----------|---------|
| CRITICAL | 0 | 0 | 0 | 0 |
| HIGH | 0 | 0 | 0 | 0 |
| MEDIUM | 0 | 0 | 0 | 0 |
| LOW | 0 | 0 | 0 | 0 |
| INFO | 0 | 0 | 0 | 0 |
| **Total** | **0** | **0** | **0** | **0** |

---

## Scope

**What was reviewed:**
- [ ] Phase 1: Static Code Analysis
- [ ] Phase 2: Architecture and Design Review
- [ ] Phase 3: Redaction and Injection Control Depth Testing
- [ ] Phase 4: RBAC Enforcement Testing
- [ ] Phase 5: Network Egress Testing
- [ ] Phase 6: GitHub App Private Key Isolation
- [ ] Phase 7: Audit Log Verification
- [ ] Phase 8: Supply Chain Integrity
- [ ] Phase 9: Operational Security
- [ ] Phase 10: Regression Check

**Phases skipped and reasons:**

| Phase | Reason |
|-------|--------|
| | |

**Git commit reviewed:** `<git rev-parse HEAD>`

**Go module versions:**
```
<!-- paste: go list -m all | head -20 -->
```

---

## Environment

**Test cluster:** (kind / k3s / real cluster / none)
**CNI:** (Cilium / Calico / flannel / none)
**mendabot version:** (git SHA or tag)
**Tools used:**
```
govulncheck: <version>
gosec: <version>
trivy: <version>
staticcheck: <version>
kubectl: <version>
```

---

## Findings

<!-- One section per finding. Copy the template below for each finding. -->
<!-- Finding IDs: YYYY-MM-DD-NNN (e.g., 2026-03-01-001) -->

---

### Finding YYYY-MM-DD-001: [Short title]

**Severity:** CRITICAL / HIGH / MEDIUM / LOW / INFO
**Status:** Open / Remediated / Accepted / Deferred
**Phase:** [Phase number where found]
**Attack Vector:** [AV-XX from THREAT_MODEL.md, or new]

#### Description

<!-- What is the vulnerability? Be specific. -->

#### Evidence

<!-- File path, line number, command output, log lines, etc. -->

```
<!-- paste evidence here -->
```

#### Exploitability

<!-- How would an attacker exploit this? Step by step. What is the precondition? -->

#### Impact

<!-- What happens if exploited? Data loss? Cluster compromise? Credential exposure? -->

#### Recommendation

<!-- Specific, actionable fix. Include code references or commands where applicable. -->

#### Remediation / Acceptance Rationale

<!-- If Remediated: what was done, commit reference. -->
<!-- If Accepted: why the risk is acceptable, what mitigations exist. -->
<!-- If Deferred: tracking reference, target date. -->

---

<!--
### Finding YYYY-MM-DD-002: [Short title]
[copy template above]
-->

---

## Redaction Pattern Gap Analysis

**New patterns identified during review:**

| Input | Passes Through? | Severity | Recommendation |
|-------|----------------|----------|---------------|
| | | | |

**Patterns added this review:** (list any new patterns added to `internal/domain/redact.go`)

---

## Injection Detection Gap Analysis

**New injection strings tested:**

| Input | Detected? | Realistic Threat? | Recommendation |
|-------|-----------|-------------------|---------------|
| | | | |

**Patterns added this review:** (list any new patterns added to `internal/domain/injection.go`)

---

## RBAC Audit Results

**ClusterRole: mendabot-agent — findings:**

| Check | Result | Notes |
|-------|--------|-------|
| No write verbs | Pass / Fail | |
| No pods/exec | Pass / Fail | |
| No nodes/proxy | Pass / Fail | |
| Namespace scope replaces (not supplements) ClusterRole | Pass / Fail / N/A | |

**ClusterRole: mendabot-watcher — findings:**

| Check | Result | Notes |
|-------|--------|-------|
| ConfigMap write namespace-scoped | Pass / Fail | |
| No write outside mendabot ns (except RemediationJobs) | Pass / Fail | |

---

## Container Security Audit Results

**Dockerfile.agent:**

| Check | Result | Notes |
|-------|--------|-------|
| Non-root user | Pass / Fail | |
| All binary checksums | Pass / Fail | Binaries without checksum: |
| Base image pinned to digest | Pass / Fail | |
| No secrets in build args | Pass / Fail | |

**Trivy scan results:**

```
<!-- paste trivy output here, or "Skipped — images not available" -->
```

---

## CI/CD Pipeline Audit Results

| Check | Workflow | Result | Notes |
|-------|---------|--------|-------|
| Least-privilege permissions | build-watcher.yaml | Pass / Fail | |
| Least-privilege permissions | build-agent.yaml | Pass / Fail | |
| Actions pinned to SHA | all | Pass / Fail | Not pinned: |
| No fork PR secret exposure | all | Pass / Fail | |
| Vulnerability scan in CI | all | Pass / Fail | |

---

## Supply Chain Audit Results

**Binary checksum coverage:**

| Binary | Checksum Verified? | Method |
|--------|-------------------|--------|
| kubectl | yes / no | sha256sum from dl.k8s.io |
| helm | yes / no | sha256sum from get.helm.sh |
| flux | yes / no | |
| opencode | yes / no | |
| talosctl | yes / no | |
| kustomize | yes / no | |
| yq | yes / no | |
| stern | yes / no | |
| age | yes / no | |
| sops | yes / no | |
| kubeconform | yes / no | |
| gh | yes / no | apt signed repo |

---

## Live Test Results

### Phase 3: Injection End-to-End

| Test | Result | Notes |
|------|--------|-------|
| TC-A: Direct RemediationJob injection | PASS / FAIL / SKIPPED | |
| TC-B: Provider-level injection | PASS / FAIL / SKIPPED | |

### Phase 4: RBAC Enforcement

| Test | Expected | Actual | Pass? |
|------|----------|--------|-------|
| Agent reads Secret (cluster scope) | Allowed | | |
| Agent reads Secret (namespace scope, in-scope ns) | Allowed | | |
| Agent reads Secret (namespace scope, out-of-scope ns) | Forbidden | | |
| Agent creates pod | Forbidden | | |
| Agent creates deployment | Forbidden | | |
| Agent accesses pods/exec | Forbidden | | |
| Agent accesses nodes/proxy | Forbidden | | |
| Watcher reads Secrets | Forbidden | | |

### Phase 5: Network Egress

| Test | Expected | Actual | Pass? |
|------|----------|--------|-------|
| DNS resolution | Allowed | | |
| GitHub API (443) | Allowed | | |
| Arbitrary external HTTPS | Blocked | | |
| Kubernetes API (6443) | Allowed | | |
| Non-API cluster service | Blocked | | |

### Phase 6: Private Key Isolation

| Test | Expected | Actual | Pass? |
|------|----------|--------|-------|
| GITHUB_APP_PRIVATE_KEY in main container env | Absent | | |
| /secrets/github-app mounted in main container | Absent | | |
| /workspace/github-token present in main container | Present | | |

### Phase 7: Audit Log Completeness

| Event | Fired? | Includes audit=true? | Includes event field? |
|-------|--------|---------------------|----------------------|
| remediationjob.cancelled | yes / no | yes / no | yes / no |
| finding.injection_detected | yes / no | yes / no | yes / no |
| finding.suppressed.cascade | yes / no | yes / no | yes / no |
| finding.suppressed.circuit_breaker | yes / no | yes / no | yes / no |
| finding.suppressed.max_depth | yes / no | yes / no | yes / no |
| finding.suppressed.stabilisation_window | yes / no | yes / no | yes / no |
| remediationjob.created | yes / no | yes / no | yes / no |
| remediationjob.deleted_ttl | yes / no | yes / no | yes / no |
| job.succeeded / job.failed | yes / no | yes / no | yes / no |
| job.dispatched | yes / no | yes / no | yes / no |

---

## Regression Check

| Previous Finding | Previous Report | Still Remediated? | Notes |
|-----------------|----------------|-------------------|-------|
| | | | |

**Accepted residual risks re-confirmed:**

| Risk ID | Risk | Acceptance Still Valid? | Notes |
|---------|------|------------------------|-------|
| AR-01 | Agent can read all Secrets (cluster scope) | yes / no | |
| AR-02 | Redaction false negatives | yes / no | |
| AR-03 | NetworkPolicy requires CNI | yes / no | |
| AR-04 | Prompt injection not fully preventable | yes / no | |
| AR-05 | GitHub token in shared emptyDir | yes / no | |
| AR-06 | HARD RULEs are prompt-only controls | yes / no | |

---

## New Accepted Residual Risks

<!-- Document any new risks that are being accepted with this review. -->

| ID | Risk | Severity | Rationale | Sign-off |
|----|------|----------|-----------|---------|
| | | | | |

---

## Recommendations for Next Review

<!-- Things to check more carefully next time. New attack surfaces to be aware of. -->

---

## Checklist Sign-off

The accompanying checklist (`docs/SECURITY/CHECKLIST.md`, copied for this review)
has been completed. All items are checked or marked SKIPPED with a reason.

**Reviewer signature / initials:**
**Date:**

---

*This report was produced following the process defined in `docs/SECURITY/PROCESS.md` v1.0.*
