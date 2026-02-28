# Changelog

All notable changes to k8s-mechanic are documented here.

This project follows [Semantic Versioning](https://semver.org). The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

Pre-1.0: minor-version bumps may include breaking changes while the API is
stabilising. Breaking changes are always called out explicitly.

---

## [Unreleased]

---

## [v0.3.12] — 2026-02-25

### Fixed

- `FINDING_FINGERPRINT` env var truncated to 12 characters to prevent the
  base64 redaction pattern from matching the full 64-character hex fingerprint
  and substituting `[REDACTED-BASE64]` in the agent prompt.

---

## [v0.3.11] — 2026-02-25

### Fixed

- Deep-validation gap fixes for epic25 tool output redaction: wrapper
  test assertions, temp-file cleanup, and edge-case coverage.

---

## [v0.3.10] — 2026-02-25

### Security (epic25 — tool call output redaction)

This release closes pentest finding **P-010 (HIGH)**: raw stdout/stderr from
every LLM-directed tool call was previously passed verbatim to the external
LLM API, allowing a single `kubectl get secret -o yaml` call to send
base64-encoded Secret data to a third-party service.

- Added `cmd/redact` filter binary: reads stdin, applies
  `domain.RedactSecrets`, writes redacted output to stdout. Imports
  `internal/domain` directly — same compiled regex patterns as source-level
  redaction, zero pattern drift.
- Added 12 PATH-shadowing shell wrappers in `docker/scripts/redact-wrappers/`
  that intercept `kubectl`, `helm`, `flux`, `gh`, `sops`, `talosctl`, `yq`,
  `stern`, `kubeconform`, `kustomize`, `age`, and `age-keygen`. Each wrapper
  calls the real binary (renamed to `<tool>.real`), captures combined
  stdout+stderr, and pipes through `redact` before returning output to the
  caller.
- Wrappers hard-fail (exit 1) if `redact` binary is absent at runtime,
  aborting the agent entrypoint visibly rather than passing raw output silently.
- Added `docker/scripts/wrapper-test.sh` CI script that verifies redaction
  function, all wrapper presence and structure, and exit-code passthrough for
  all 12 wrapped tools.
- Documented unwrapped tools and accepted residual risks in
  `docs/SECURITY/THREAT_MODEL.md` AV-02: `curl`, `jq`, `openssl` are not
  wrapped because they are required in the init container GitHub App token
  exchange flow; `git` is not wrapped because wrapping would break diff-based
  PR workflows.
- Updated `docs/SECURITY/THREAT_MODEL.md` to v1.2 with pentest and audit
  outcomes for all attack vectors.

### Security (pentest findings P-003 through P-009)

Remediated from the 2026-02-24 pentest report:

- **P-004 (HIGH)** — Agent `ClusterRole` wildcard `resources: ["*"]` implicitly
  granted `nodes/proxy` access (kubelet metrics, log directory listing).
  Replaced with four explicit rules scoped to `core`, `apps`, `batch`, and
  `remediation.mechanic.io` API groups. `nodes/proxy`, `pods/exec`, and all
  other dangerous subresources are excluded by omission.
- **P-008 (MEDIUM)** — `domain.DetectInjection` was not called in the
  `RemediationJobController` dispatch path, allowing a directly-created
  `RemediationJob` to bypass injection detection. Added injection detection
  against both `Finding.Errors` and `Finding.Details` in `dispatch()` before
  `JobBuilder.Build()` is called.
- **P-003 (MEDIUM)** — `golang.org/x/net v0.30.0` had 4 CVEs in the module
  dependency tree (GO-2026-4441, GO-2026-4440, GO-2025-3595, GO-2025-3503).
  Upgraded to v0.45.0.
- **P-006 (LOW)** — PEM private key headers (`-----BEGIN RSA PRIVATE KEY-----`)
  were not covered by redaction patterns. Added regex covering RSA, EC, DSA,
  and OPENSSH private key blocks in `internal/domain/redact.go`.
- **P-007 (LOW)** — `X-API-Key` HTTP header colon-space format was not matched
  for short values. Added `(?i)(x-api-key\s*[=:]\s*)\S+` pattern.
- **2026-02-24-001 (MEDIUM)** — Prompt injection envelope
  (`=== BEGIN/END FINDING ERRORS ===`) and HARD RULE 8 were missing from
  `charts/mechanic/files/prompts/core.txt` (regression of epic12 STORY_05).
  Restored both the envelope delimiters and the HARD RULE.
- **2026-02-24-002 (MEDIUM)** — Watcher `ClusterRole` granted unnecessary
  cluster-wide `secrets` read. Removed `"secrets"` from `ClusterRole`;
  namespace-scoped `Role` already covers the legitimate use case.

### Fixed

- Added `.trivyignore` for upstream stdlib CVEs not yet fixed in pre-built
  binaries; entries carry mandatory expiry dates.

---

## [v0.3.9] — 2026-02-24

### Fixed

- Agent tool versions bumped across the board for CVE remediation.
- Go toolchain upgraded to 1.25.7; `golang.org/x/oauth2` upgraded to v0.35.
- `age` now compiled from source (Go build) rather than downloaded as a
  pre-built binary, eliminating an absent upstream CHECKSUMS file dependency.

---

## [v0.3.8] — 2026-02-24

### Documentation

- README onboarding audit: Secrets setup, annotation reference, Helm values
  table, and RemediationJob lifecycle state diagram.

---

## [v0.3.7] — 2026-02-24

### Added — Severity tiers (epic24)

Every finding is now classified as `critical`, `high`, `medium`, or `low`
before dispatch. The classification is propagated through the full pipeline.

- **Domain:** `Severity` named type and constants; `SeverityLevel()`,
  `MeetsSeverityThreshold()`, `ParseSeverity()` in `internal/domain`.
- **Providers:** All 6 native providers assign severity:
  - Pod: `critical` (CrashLoopBackOff >5 restarts), `high` (CrashLoopBackOff
    ≤5, OOMKilled, ImagePullBackOff, Unschedulable), `medium` (default).
  - Deployment/StatefulSet: `critical` (0 ready), `high` (<50% ready),
    `medium` (degraded-but-available).
  - Node: `critical` (NotReady), `high` (pressure conditions).
  - PVC: `high`. Job: `medium`.
- **Filter:** `MIN_SEVERITY` env var (`critical`/`high`/`medium`/`low`);
  configurable via `watcher.minSeverity` in `values.yaml` (default: `low`).
  Suppressed findings emit `finding.suppressed.min_severity` audit log.
- **Agent prompt:** `FINDING_SEVERITY` injected as env var; prompt includes a
  `=== SEVERITY CALIBRATION ===` block calibrating investigation depth by tier
  (maximum thoroughness for critical, conservative minimal-change for low).

### Added — Namespace-level annotation gate (epic16 STORY_04)

Annotating a `Namespace` object with `mechanic.io/enabled=false` or
`mechanic.io/skip-until=YYYY-MM-DD` now suppresses all findings from every
resource in that namespace, regardless of the resource's own annotations.

### Added — Namespace filtering (epic15)

Two new env vars (Helm values) allow operators to scope what mechanic watches:

- `WATCH_NAMESPACES` / `watcher.watchNamespaces`: comma-separated allowlist.
  Empty = watch all namespaces (default).
- `EXCLUDE_NAMESPACES` / `watcher.excludeNamespaces`: comma-separated denylist.
  Empty = no exclusions (default).
- Node findings always bypass the namespace filter (nodes are cluster-scoped).

### Added — Kubernetes Events on RemediationJob (epic21)

`kubectl describe rjob <name>` now shows a live lifecycle timeline in the
`Events` section. Events emitted:

| Event | Reason | Type |
|-------|--------|------|
| Finding detected, RemediationJob created | `FindingDetected` | Normal |
| Duplicate fingerprint skipped | `DuplicateFingerprint` | Normal |
| Source object deleted, job cancelled | `SourceDeleted` | Normal |
| Finding cleared on source | `FindingCleared` | Normal |
| Agent Job dispatched | `JobDispatched` | Normal |
| Agent Job succeeded | `JobSucceeded` | Normal |
| Agent Job failed (retryable) | `JobFailed` | Warning |
| Agent Job permanently failed | `JobPermanentlyFailed` | Warning |

### Added — Per-resource annotation control (epic16)

Three annotations gate mechanic's behaviour on any watched resource or
Namespace:

| Annotation | Value | Effect |
|---|---|---|
| `mechanic.io/enabled` | `"false"` | Suppress all findings from this resource |
| `mechanic.io/skip-until` | `"YYYY-MM-DD"` | Suppress until end-of-day UTC on this date |
| `mechanic.io/priority` | `"critical"` | Bypass stabilisation window; dispatch immediately |

Malformed `skip-until` dates are silently ignored (no suppression) to prevent
typos from permanently disabling investigations.

---

## [v0.3.6] — 2026-02-23

### Fixed

- Secret cache scoped to agent namespace; `get` on `secrets` added to
  namespace-scoped `Role` for readiness checker.

---

## [v0.3.5] — 2026-02-23

### Added — Structured audit log (epic23)

All suppression and dispatch decisions now emit structured log lines with
`"audit": true` and a stable `"event"` string field, queryable from Loki,
Elasticsearch, Datadog, or any log aggregation system. Events include:
`finding.suppressed.min_severity`, `finding.suppressed.stabilisation_window`,
`finding.stabilisation_window_bypassed`, `finding.injection_detected`,
`remediationjob.created`, `remediationjob.cancelled`, `job.dispatched`,
`job.succeeded`, `job.failed`, `job.permanently_failed`.

### Added — LLM readiness gate

Watcher startup validates LLM provider credentials before registering
controllers, emitting a clear error if the secret is absent or malformed.

### Added — Test infrastructure (epic14)

- Shared envtest suite with deterministic object-name pre-test cleanup.
- CRD testdata drift detection rules documented in `README-LLM.md`.

---

## [v0.3.4] — 2026-02-23

### Added — Dead-letter queue / retry cap (epic17)

Prevents a broken finding from burning unlimited LLM quota across infinite
retries.

- `PermanentlyFailed` phase: when `RetryCount >= MaxRetries`, the
  `RemediationJob` is tombstoned with `PermanentlyFailed` phase. No further
  dispatch occurs. Visible via `kubectl describe rjob <name>`.
- `MaxRetries` field on `RemediationJobSpec` (default: 3, minimum: 1).
  Configurable via `watcher.maxInvestigationRetries` in `values.yaml`.
- `RetryCount` field on `RemediationJobStatus` tracks accumulated attempts.
- `PermanentlyFailed` tombstones are never deleted by the source provider
  reconciler — they survive as evidence of the failure history.
- `job.permanently_failed` audit log event emitted with `retryCount` and
  `effectiveMaxRetries` fields.

---

## [v0.3.3] — 2026-02-23

### Added — Security hardening (epic12)

Structured security review with evidence-based findings and remediations.
All 11 open findings remediated:

- **Secret redaction patterns expanded:** Bearer JWT tokens
  (`Authorization: Bearer ...`), JSON-encoded password fields
  (`"password":"..."` ), Redis empty-username URLs (`redis://:pass@host`).
- **Injection detection extended:** `FINDING_DETAILS` field now screened by
  `domain.DetectInjection` with its own envelope in the agent prompt.
  New injection pattern: `stop following/obeying the rules`.
- **Go module CVEs:** `go 1.23.0` → `go 1.23.12`; address three stdlib CVEs
  in net/url and crypto/tls.
- **GitHub Actions SHA pinning:** All third-party actions pinned to commit
  SHAs in all three workflow files.
- **Trivy severity threshold:** Raised from `CRITICAL` to `CRITICAL,HIGH`.
- **Base image digest pinning:** `Dockerfile.agent` and `Dockerfile.watcher`
  both pin `debian:bookworm-slim` and `golang:1.23-bookworm` to digest.
- **Binary checksum coverage:** SHA256 ARG variables added for `yq`, `age`,
  and `opencode`; `sha256sum --check` added to each install block.
- **RBAC least-privilege:** ConfigMap write access removed from watcher
  `ClusterRole`; scoped to namespace-level `Role` only.

### Added — Prometheus metrics

Optional metrics `Service` and Prometheus Operator `ServiceMonitor` for
watcher health observability. Enable with `metrics.enabled: true` and
`metrics.serviceMonitor.enabled: true` in `values.yaml`.

---

## [v0.3.2] — 2026-02-23

### Fixed

- `FINDING_DETAILS` is now optional; native providers that do not set it no
  longer cause the agent entrypoint to fail with an unbound variable error.

---

## [v0.3.1] — 2026-02-23

### Changed

- Branch table housekeeping; epic09 merged and feature branch retired.

---

## [v0.3.0] — 2026-02-23

### Added — Helm chart (epic10)

Full Helm chart at `charts/mechanic/` replacing the Kustomize-only deployment.

- `helm install mechanic charts/mechanic/ --set gitops.repo=org/repo --set gitops.manifestRoot=kubernetes`
- 13 templates: namespace, service accounts, RBAC (ClusterRole, ClusterRoleBinding,
  Role, RoleBinding for both watcher and agent), watcher Deployment, prompt
  ConfigMap (core + agent), CRD install/upgrade hook.
- Optional: metrics Service and Prometheus Operator `ServiceMonitor`.
- Optional: opt-in `NetworkPolicy` restricting agent Job egress.
- `values.yaml` schema with full inline documentation.
- `NOTES.txt` with post-install setup instructions.

### Added — Cascade prevention (epic11)

Prevents mechanic from triggering infinite self-remediation loops.

- **Circuit breaker:** ConfigMap-backed persistent state; cooldown period via
  `SELF_REMEDIATION_COOLDOWN_SECONDS` (default: 5 minutes). Zero cooldown
  disables the circuit breaker entirely.
- **Chain depth tracking:** `ChainDepth` field on `Finding`; maximum depth via
  `SELF_REMEDIATION_MAX_DEPTH` (default: 2). Findings that exceed the depth
  cap are suppressed with a `finding.suppressed.max_depth` audit event.
- **Cascade checker:** Infrastructure cascade detection — suppress pod findings
  on NotReady nodes; suppress OOMKilled pods on nodes with MemoryPressure;
  suppress namespace-wide failures above a configurable threshold.
- **8 Prometheus metrics** for cascade monitoring: circuit breaker activations,
  chain depth histogram, max-depth-exceeded counter, cascade suppressions by
  reason.

### Added — Pluggable agent runner (epic08)

A single `AGENT_TYPE` env var (`opencode` or `claude`) controls which AI agent
binary the watcher injects into agent Jobs. Zero-maintenance for new providers.

- Per-agent Secrets: `llm-credentials-opencode`, `llm-credentials-claude`.
  Single key `provider-config` (opaque JSON blob) — mechanic never interprets
  LLM provider details.
- Split prompt ConfigMaps: `agent-prompt-core` (shared investigation
  instructions) + `agent-prompt-<agentType>` (agent-specific preamble).
- Overridable via `prompt.coreOverride` / `prompt.agentOverride` in
  `values.yaml`.
- Claude entrypoint is a validated stub (exits with a clear error) — not yet
  functional.

> **Breaking changes** from v0.2.x:
> - Secret renamed: `llm-credentials` → `llm-credentials-opencode`
> - Secret key schema changed: `api-key`/`base-url`/`model` → `provider-config`
> - Helm values `prompt.name`/`prompt.override` → `prompt.coreOverride`/`prompt.agentOverride`
> - ConfigMap `opencode-prompt` → `agent-prompt-core` + `agent-prompt-opencode`

### Added — Agent network policy (epic12 STORY_02)

Opt-in `NetworkPolicy` restricts agent Job egress to the cluster API server,
GitHub (`443/tcp`), and the configured LLM endpoint. Enable via
`networkPolicy.enabled: true`. Requires a CNI that enforces `NetworkPolicy`.

### Added — Namespace-scoped agent RBAC (epic12 STORY_04)

`AGENT_RBAC_SCOPE=namespace` (Helm: `watcher.agentRBACScope`) switches the
agent from a cluster-wide `ClusterRole` to a namespace-scoped `Role`, limiting
cluster reads to the namespaces specified in `watcher.agentWatchNamespaces`.

---

## [v0.2.x] — 2026-02-22

### Added — Native Kubernetes provider (epic09)

Replaced the k8sgpt operator dependency with direct Kubernetes API watchers.
mechanic no longer requires k8sgpt-operator to be installed.

**Six native providers:** `PodProvider`, `DeploymentProvider`,
`StatefulSetProvider`, `PVCProvider`, `NodeProvider`, `JobProvider`.

Detected conditions:

| Provider | Conditions detected |
|---|---|
| Pod | `CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `OOMKilled`, non-zero exit code, Unschedulable/Pending |
| Deployment | Replica count mismatch, `Available=False` |
| StatefulSet | Replica count mismatch, `Available=False` |
| PVC | `Phase=Pending` + `ProvisioningFailed` event |
| Node | `NodeReady=False/Unknown`, pressure conditions (`MemoryPressure`, `DiskPressure`, `PIDPressure`) |
| Job | `Failed > 0` + `Active == 0` + no completion time (CronJobs excluded) |

- **Deduplication by parent resource:** pod restarts from the same Deployment
  produce one `RemediationJob`, not one per pod restart. Fingerprint:
  `sha256(namespace + kind + parentObject + sorted(errors))`.
- **Stabilisation window:** configurable hold period (default: 120s) filters
  transient blips before dispatch (`STABILISATION_WINDOW_SECONDS`).
- **Owner-reference traversal:** `getParent` walks up to 10 levels of owner
  references with circular-reference guard.
- k8sgpt dependency removed from Go module, Dockerfile, and all RBAC manifests.

### Added — Secret redaction (epic12 STORY_01)

`domain.RedactSecrets` applied at all six native provider ingestion points
before text is stored in `RemediationJob` or injected into the agent. Patterns
cover URL credentials, base64-encoded values ≥40 characters, and common secret
key prefixes.

### Added — Prompt injection detection (epic12 STORY_05)

`domain.DetectInjection` screens `Finding.Errors` before dispatch.
`INJECTION_DETECTION_ACTION=suppress` drops matching findings entirely;
default `log` emits a structured audit event and continues.

---

## [v0.1.x] — 2026-02-19 to 2026-02-22

### Added — Initial implementation

Foundation release. All core epics (00 through 06) implemented via strict TDD.

- **Controller core (epic01):** `RemediationJobReconciler` (controller-runtime)
  with deduplication by parent-resource fingerprint, `MAX_CONCURRENT_JOBS`
  throttle, phase lifecycle (`Pending → Dispatched → Running → Succeeded /
  Failed / Cancelled`), envtest integration tests.
- **JobBuilder (epic02):** Pure `Build()` function; init container exchanges
  GitHub App private key for short-lived installation token; main container
  runs the agent with `FINDING_*` env vars, projected prompt ConfigMap volume,
  and read-only RBAC.
- **Agent image (epic03):** `debian:bookworm-slim` base; opencode, kubectl,
  helm, flux, kustomize, gh, kubeconform, yq, jq, stern, sops, age, talosctl
  installed with SHA256 verification. Runs as non-root (`uid=1000`).
- **Kustomize manifests (epic04):** Namespace, CRD, ClusterRole/ClusterRoleBinding
  and Role/RoleBinding for watcher and agent, prompt ConfigMap, Secret
  placeholders, watcher Deployment.
- **Agent prompt (epic05):** Structured investigation protocol with 8 HARD
  RULES enforced on every invocation.
- **CI/CD (epic06):** GitHub Actions workflows for watcher and agent image
  builds to `ghcr.io`; test suite on every push; Trivy CVE scanning on release
  tags.
- **k8sgpt provider (epic00-epic01):** Initial source provider backed by
  k8sgpt-operator `Result` CRDs. Replaced by native providers in v0.2.x.

---

[Unreleased]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.12...HEAD
[v0.3.12]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.11...v0.3.12
[v0.3.11]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.10...v0.3.11
[v0.3.10]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.9...v0.3.10
[v0.3.9]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.8...v0.3.9
[v0.3.8]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.7...v0.3.8
[v0.3.7]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.6...v0.3.7
[v0.3.6]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.5...v0.3.6
[v0.3.5]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.4...v0.3.5
[v0.3.4]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.3...v0.3.4
[v0.3.3]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/lenaxia/k8s-mechanic/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/lenaxia/k8s-mechanic/compare/v0.2.15...v0.3.0
[v0.2.x]: https://github.com/lenaxia/k8s-mechanic/compare/v0.1.9...v0.2.15
[v0.1.x]: https://github.com/lenaxia/k8s-mechanic/releases/tag/v0.1.2
