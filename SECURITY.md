# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

To report a vulnerability, open a
[GitHub Security Advisory](https://github.com/lenaxia/k8s-mechanic/security/advisories/new)
on this repository. This keeps the report confidential until a fix is ready.

If you are unable to use GitHub Security Advisories, contact the maintainer
directly via the contact details on their
[GitHub profile](https://github.com/lenaxia).

Please include:

- A clear description of the vulnerability
- Steps to reproduce or a proof-of-concept (as much as you can provide)
- The component affected (`mechanic-watcher`, `mechanic-agent`, Helm chart,
  RBAC manifests, etc.)
- Your assessment of impact and severity
- Any suggested remediation, if you have one

You will receive an acknowledgement within **5 business days**. A substantive
response with triage outcome will follow within **10 business days**. If the
issue is confirmed, we will coordinate a fix and disclosure timeline with you.

---

## Supported Versions

| Version | Supported |
|---|---|
| Latest release (`main`) | Yes |
| Previous release | Critical and HIGH fixes only |
| Older releases | No |

---

## Security Architecture

k8s-mechanic is designed to run inside a Kubernetes cluster with a minimal,
auditable attack surface. The key security properties are:

### Agent isolation

- The agent Job runs as a non-root user (`uid=1000`).
- The agent holds **read-only** RBAC (`get/list/watch`) — it cannot create,
  modify, or delete any Kubernetes resource.
- All cluster changes go through Git pull requests; no direct writes to the
  cluster are possible.
- The GitHub App private key is isolated to the init container. The main agent
  container never receives it.
- Short-lived GitHub App installation tokens (1-hour TTL) are used for all Git
  operations.

### Secret redaction

Error text extracted from cluster state (pod status messages, node conditions,
event messages) is passed through `domain.RedactSecrets` before being stored in
a `RemediationJob` or injected into the agent prompt. Redaction covers:

- URL-embedded credentials (`scheme://user:pass@host`)
- Base64-encoded values ≥ 40 characters
- PEM private key blocks
- Named patterns: `password=`, `token=`, `secret=`, `api-key=`, `x-api-key:`,
  GitHub tokens (`ghp_`, `ghs_`, `gho_`, `glpat-`), AWS secret keys,
  Bearer JWT tokens, and others

Tool call output (stdout/stderr of every tool the agent executes) is similarly
piped through the same redaction binary (`/usr/local/bin/redact`) before being
returned to the LLM context. See
[`docs/SECURITY/THREAT_MODEL.md`](docs/SECURITY/THREAT_MODEL.md) AV-02 for the
full wrapper inventory and documented exceptions.

### Prompt injection defence

- All untrusted text sourced from cluster state is bounded to 500 characters
  per field before storage.
- Error text is wrapped in an explicit `=== BEGIN/END FINDING ERRORS ===`
  delimiter in the agent prompt, instructing the LLM to treat it as data only.
- `domain.DetectInjection` screens `Finding.Errors` and `Finding.Details` at
  both the provider ingestion point and the controller dispatch point. When
  `INJECTION_DETECTION_ACTION=suppress` is set, matching findings are silently
  dropped and permanently failed rather than dispatched.

### Network restriction

An opt-in `NetworkPolicy` restricts agent Job egress to the cluster API server,
GitHub (`443/tcp`), and the configured LLM endpoint. Enable it with
`networkPolicy.enabled: true` in `values.yaml`. Requires a CNI that enforces
`NetworkPolicy` (Cilium, Calico, etc.).

### Supply chain

- All binary downloads in `Dockerfile.agent` are verified with SHA256 checksums
  from official release pages.
- The `gh` CLI is installed from GitHub's GPG-signed apt repository.
- Base images are pinned to digests.
- GitHub Actions workflows are pinned to commit SHAs.
- Every release tag is scanned with [Trivy](https://trivy.dev) for
  `CRITICAL` and `HIGH` CVEs (ignore-unfixed). The build fails on any fixable
  finding. Unfixed CVEs from upstream pre-built binaries are tracked with
  mandatory expiry dates in [`.trivyignore`](.trivyignore).
- `govulncheck` runs in CI against Go module dependencies.

---

## Threat Model

The full threat model is maintained at
[`docs/SECURITY/THREAT_MODEL.md`](docs/SECURITY/THREAT_MODEL.md). It covers:

| Attack Vector | Risk | Status |
|---|---|---|
| AV-01: Prompt injection via cluster state | CRITICAL | Mitigated (layered controls) |
| AV-02: Credential exposure via error text and tool call output | HIGH | Mitigated (redaction at source + tool wrappers) |
| AV-03: Cluster Secret exfiltration by agent | HIGH | Mitigated (explicit RBAC; NetworkPolicy opt-in) |
| AV-04: GitHub App key compromise | CRITICAL | Mitigated (init container isolation; short-lived tokens) |
| AV-05: Malicious GitOps PR | HIGH | Mitigated (branch protection + human review required) |
| AV-06: Supply chain attack on Docker image | HIGH | Mitigated (checksum verification; Trivy CI scan) |
| AV-07: Vulnerable Go dependencies | MEDIUM | Mitigated (govulncheck in CI; `go.sum` pinning) |
| AV-08: RBAC over-permission on watcher | MEDIUM | Mitigated (cluster-wide Secrets removed from ClusterRole) |
| AV-09: RemediationJob spec injection | MEDIUM | Mitigated (injection detection in controller dispatch path) |
| AV-10: GitHub token in shared volume | MEDIUM | Accepted residual risk (unavoidable init-container pattern; 1-hour TTL) |
| AV-11: Agent image not pinned to digest | LOW | Operator responsibility |
| AV-12: Log injection / structured log pollution | LOW | Mitigated (zap JSON encoder escapes newlines and quotes) |

Accepted residual risks, pentest outcomes, and the full evidence base for each
mitigation are documented in the threat model.

---

## Security Review Process

mechanic undergoes a structured, evidence-based security review after any
major change and at minimum quarterly. The review process is documented in
[`docs/SECURITY/PROCESS.md`](docs/SECURITY/PROCESS.md) and covers 10 phases:

1. Static code analysis (`govulncheck`, `gosec`, `staticcheck`)
2. Architecture and design review (data flow traces, RBAC audit, container audit)
3. Redaction and injection control depth testing
4. RBAC enforcement testing (live cluster)
5. Network egress testing
6. GitHub App private key isolation testing
7. Audit log verification
8. Supply chain integrity
9. Operational security review
10. Regression check against known findings

Completed reports are committed to
[`docs/SECURITY/`](docs/SECURITY/) and are immutable once merged.

**Closure criteria:** No CRITICAL or HIGH findings may remain Open. Any
CRITICAL/HIGH finding accepted with residual risk requires explicit written
sign-off in the report.

---

## Known Residual Risks

The following risks are accepted with documented rationale. See
[`docs/SECURITY/THREAT_MODEL.md`](docs/SECURITY/THREAT_MODEL.md) §8 for the
full acceptance record.

| ID | Risk | Severity |
|----|------|----------|
| AR-01 | Agent can read all Secrets cluster-wide (default scope). Namespace scope opt-in available via `AGENT_RBAC_SCOPE=namespace`. | HIGH |
| AR-02 | Regex-based redaction has false negatives. Best-effort; not a substitute for proper secret management. | MEDIUM |
| AR-03 | NetworkPolicy requires a CNI that enforces it. Not automatically applied. | MEDIUM |
| AR-04 | Prompt injection cannot be fully prevented. Layered mitigations reduce but do not eliminate risk. | MEDIUM |
| AR-05 | GitHub App token shared via emptyDir volume between init and main containers. 1-hour TTL limits exposure. | MEDIUM |
| AR-06 | HARD RULEs in the agent prompt are LLM instructions, not technical controls. GitHub branch protection is the external enforcer. | MEDIUM |

---

## Disclosure Policy

- We follow [coordinated disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure).
- We will acknowledge reports within 5 business days.
- We aim to release a fix within 30 days of confirmation for HIGH and CRITICAL
  findings, and within 90 days for MEDIUM and below.
- We will credit reporters in the release notes unless they request otherwise.
- We will not take legal action against researchers acting in good faith.
