# High-Level Design — Multi-Source Signal Architecture

**Version:** 2.0
**Date:** 2026-02-25
**Status:** Proposed
**Supersedes:** `docs/DESIGN/HLD.md` (v1.4)

---

## Document Control

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 2.0 | 2026-02-25 | Multi-source signal layer: AlertSource CRD, resource-level fingerprinting, cross-source priority deduplication, AlertSourceReconciler, enriched agent context | LLM / Human |
| 2.1 | 2026-02-25 | Design review fixes: descope PagerDuty/OpsGenie, simplify pending-alert flow, fix multi-replica/leader-election, add CRD generation prerequisite, define SourceResultRef sentinel, add DeepCopyInto/config/correlation notes | LLM / Human |

---

## Table of Contents

1. [Motivation and Context](#1-motivation-and-context)
2. [Design Principles](#2-design-principles)
3. [Architecture Overview](#3-architecture-overview)
4. [Signal Source Categories](#4-signal-source-categories)
5. [AlertSource CRD](#5-alertsource-crd)
6. [Fingerprint Redesign](#6-fingerprint-redesign)
7. [Cross-Source Deduplication and Priority](#7-cross-source-deduplication-and-priority)
8. [AlertSourceReconciler](#8-alertsourcereconciler)
9. [Webhook Receiver](#9-webhook-receiver)
10. [Native Provider Configuration](#10-native-provider-configuration)
11. [Pending Alert Annotation Pattern](#11-pending-alert-annotation-pattern)
12. [Agent Context Enrichment](#12-agent-context-enrichment)
13. [Priority Tuning — Future Direction](#13-priority-tuning--future-direction)
14. [RemediationJob CRD Changes](#14-remediationjob-crd-changes)
15. [RBAC Changes](#15-rbac-changes)
16. [Data Flow](#16-data-flow)
17. [Failure Modes](#17-failure-modes)
18. [Configuration Reference](#18-configuration-reference)
19. [What Does Not Change](#19-what-does-not-change)
20. [Scope and Success Criteria](#20-scope-and-success-criteria)

---

## 1. Motivation and Context

The v1 architecture watches native Kubernetes objects (Pods, Deployments, Nodes, etc.) and
detects unhealthy states by inspecting current object status. This covers a foundational set
of failure modes but has a structural ceiling:

- **Transient-but-significant events** (OOMKill that resolves before the stabilisation window
  expires, single crash-and-recover) are swallowed silently.
- **Signals not represented as K8s object state** — Prometheus alerting rules, scrape target
  health, PDB violations, custom metric thresholds — are completely invisible to the watcher.
- **Signals from external platforms** (PagerDuty, OpsGenie, Datadog) are not reachable via
  K8s informers.

The v1 design is right for zero-config baseline coverage. But any team serious enough to run
autonomous remediation is already running a monitoring stack (Prometheus, Datadog, etc.) that
produces richer, more intentional signal. The v2 architecture makes that signal the primary
input, while retaining native providers as an opinionated fallback.

---

## 2. Design Principles

**1. Monitoring stacks already solved alert deduplication and filtering.**
Prometheus Alertmanager's `for:` duration, inhibition rules, and grouping are the right
place to decide "is this worth acting on?" Mechanic should not re-implement this. External
alert sources should bypass the stabilisation window — the monitoring stack already waited.

**2. One RemediationJob per unhealthy resource at a time.**
The unit of remediation is the resource (Deployment, Node, Pod, etc.), not the specific
error text observed at a point in time. Multiple sources detecting the same broken deployment
should produce one investigation, not several.

**3. Higher-quality signal wins; lower-quality signal falls back.**
Native K8s providers offer zero-config coverage. External alert sources offer curated,
intentional signal. When both detect the same resource, the higher-priority source determines
the context the agent receives. Native providers do not fire if a higher-priority source
already owns the resource.

**4. Priority is mutable and should be designed for future feedback.**
Initial priorities are human-configured. The architecture stores priorities in a way that
a future feedback controller could update them based on remediation outcomes (PR merged
vs. closed as noise). No feedback mechanism is implemented in v2 — the design must not
prevent it.

**5. Operator pattern for source configuration.**
External alert sources are Kubernetes resources (`AlertSource` CRD). Native provider
configuration remains in Helm values — it does not need runtime updates and has opinionated
safe defaults.

**6. Same binary, same core pipeline.**
The webhook receiver, polling loop, and existing informer-based providers all feed the
same downstream pipeline: fingerprint → priority resolution → dedup → readiness gate →
create `RemediationJob`. No new microservices.

---

## 3. Architecture Overview

```
SIGNAL SOURCES
─────────────────────────────────────────────────────────────────
 Native (informers)           External (push or pull)
 ┌──────────────────┐         ┌───────────────────────────────┐
 │ Pod provider     │         │ AlertSource CR: alertmanager  │
 │ Deployment       │         │   webhook  POST /webhook/v1/  │
 │ StatefulSet      │         │   poll     GET /api/v2/alerts │
 │ Node             │         ├───────────────────────────────┤
 │ PVC              │         │ (future: pagerduty, opsgenie) │
 │ Job              │         │   out of scope for v2         │
 └──────────┬───────┘         └──────────────┬────────────────┘
            │ Finding                         │ Finding
            │ (SourceType=                    │ (SourceType="alertmanager",
            │  "native",                      │  Priority=per-CR,
            │  Priority=10)                   │  SkipStabilisation=true)
            └──────────────────┬──────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │   Resource Fingerprint   │
                  │   SHA256(ns+kind+parent) │
                  └────────────┬─────────────┘
                               │
                  ┌────────────▼─────────────────────────────┐
                  │   Priority Resolution + Dedup             │
                  │                                          │
                  │   Active RJ exists?                      │
                  │   ├─ No → create RJ                      │
                  │   ├─ Yes, same or higher priority →      │
                  │   │   suppress incoming finding          │
                  │   └─ Yes, lower priority →               │
                  │       annotate RJ with pending finding;  │
                  │       process on RJ completion           │
                  └────────────┬─────────────────────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │   Readiness Gate         │
                  │   (sink + LLM health)    │
                  └────────────┬─────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │   RemediationJob CRD     │
                  └────────────┬─────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │   RemediationJob         │
                  │   Reconciler             │
                  │   (unchanged)            │
                  └────────────┬─────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │   Agent Job              │
                  │   (enriched env vars)    │
                  └──────────────────────────┘
```

---

## 4. Signal Source Categories

### 4.1 Native Sources (baseline, zero-config)

Watch native Kubernetes objects via controller-runtime informers. No external dependencies.
Intended to give coverage to teams that have not yet configured a monitoring stack, or for
failure modes that monitoring rules have not yet been written for.

| Provider | Watches | Enabled by default |
|---|---|---|
| Pod | `corev1.Pod` | Yes |
| Deployment | `appsv1.Deployment` | Yes |
| StatefulSet | `appsv1.StatefulSet` | Yes |
| Node | `corev1.Node` | Yes |
| PVC | `corev1.PersistentVolumeClaim` | Yes |
| Job | `batchv1.Job` | Yes |

Native sources are configured exclusively via Helm values. No `AlertSource` CRD is created
for them. They share a single configurable priority (default: `10`).

### 4.2 External Alert Sources (primary signal path)

Receive signals from monitoring platforms via push (webhook) or pull (polling). Each
external source is represented by an `AlertSource` CRD applied to the cluster.

Built-in implementations in v2:

| Type | Mode | Notes |
|---|---|---|
| `alertmanager` | webhook, poll, or both | Standard Alertmanager v2 webhook payload |

PagerDuty and OpsGenie adapters are explicitly **out of scope for v2**. Reasons:
- PagerDuty resource identity comes from `incident.custom_fields` and `incident.title` pattern
  matching, which are site-specific configurations with no standard schema. A generic adapter
  would be unusable without validation against real customer payloads.
- OpsGenie's `k8s:<key>=<value>` tag convention is informal and undocumented — its adoption
  varies widely between teams.

These will be delivered as follow-up adapters once the adapter interface is proven stable in
production with Alertmanager. The `AlertSourceAdapter` interface is designed to support them
without CRD changes — adding a new adapter requires only (1) implementing the interface and
(2) one registry entry in `main.go`.

---

## 5. AlertSource CRD

External alert sources are configured via the `AlertSource` CRD
(`alertsources.mechanic.io/v1alpha1`). Each CR represents one configured integration.

```yaml
apiVersion: mechanic.io/v1alpha1
kind: AlertSource
metadata:
  name: alertmanager
  namespace: mechanic       # must be in the mechanic namespace
spec:
  type: alertmanager        # adapter type; determines payload parsing
  priority: 90              # higher wins in cross-source dedup; mutable
  stabilisationWindow: 0s   # 0 = skip; push sources have already waited via `for:`

  webhook:
    enabled: true
    path: /webhook/v1/alertmanager    # registered on the HTTP server
    hmacSecretRef:                    # optional; if set, validates X-Alertmanager-Key
      name: alertmanager-webhook-secret
      key: secret

  poll:
    enabled: false
    url: http://alertmanager.monitoring.svc:9093
    interval: 60s
    authSecretRef:                    # optional bearer token
      name: alertmanager-auth
      key: token

  # Maps alert label names to K8s resource identity fields.
  # Defaults are shown; override only when label names differ.
  labelMapping:
    namespace: namespace
    deployment: deployment
    pod: pod
    node: kubernetes_node
    statefulset: statefulset
    service: service
    pvc: persistentvolumeclaim
    alertname: alertname              # always present; used as fallback kind

status:
  conditions:
    - type: Ready
      status: "True"
  lastPollTime: "2026-02-25T10:00:00Z"
  alertsReceived: 142
  alertsDispatched: 8
```

The `AlertSourceReconciler` watches `AlertSource` CRDs and:
- Registers/deregisters webhook paths dynamically as CRs are created or deleted
- Starts/stops polling goroutines when `poll.enabled` changes
- Updates `status` with health and throughput metrics

The `AlertSourceReconciler` also watches `RemediationJob` objects. When an RJ it created
reaches a terminal state with a `mechanic.io/pending-alert` annotation present, it handles
the pending finding directly — no channel sharing with `RemediationJobReconciler` is required.
See §11 for the pending-alert annotation pattern and §8 for the reconciler internals.

---

## 6. Fingerprint Redesign

### 6.1 Change from v1

v1 fingerprint: `SHA256(namespace + kind + parentObject + sorted(errorTexts))`

v2 fingerprint: `SHA256(namespace + kind + parentObject)`

Error texts are **stored**, not **hashed**. The fingerprint identifies *what resource is
broken*, not *how it is currently broken*. This makes cross-source deduplication natural:
the same broken Deployment detected by the native provider and by an Alertmanager alert
produces the same fingerprint regardless of how each source describes the error.

### 6.2 Implications

**One active RemediationJob per resource at a time.** If a Deployment is being investigated,
a new finding for that Deployment from any source is handled via the priority resolution
path (§7), not by creating a second RJ.

**Error texts are still stored and surfaced to the agent.** `RemediationJob.Spec.Finding.Errors`
continues to hold the raw error JSON. A new annotation `mechanic.io/error-summary` is added
to the RJ for quick human inspection. The agent receives error texts via `FINDING_ERRORS`.

**A new RJ is created when:** the existing RJ is in a terminal state (Succeeded or Failed)
and the resource is still or again unhealthy. The new RJ receives the current error context
at the time it is created.

**Dynamic error states no longer cause spurious new RJs.** Previously, a Deployment with
`desired=3 ready=1` would produce a different fingerprint from `desired=3 ready=2` as pods
came up, potentially creating two concurrent investigations. With resource-level fingerprinting
this cannot happen.

See `FINGERPRINT_LLD.md` for the full algorithm, test cases, and migration notes.

---

## 7. Cross-Source Deduplication and Priority

### 7.1 Priority Values

Priority is a non-negative integer stored in `AlertSource.Spec.Priority` (for external
sources) and in Helm values (for native). Higher value wins. Default values:

| Source | Default Priority |
|---|---|
| Native | 10 |
| Alertmanager | 90 |
| PagerDuty | 100 |
| OpsGenie | 100 |

These are defaults only. Operators choose the values appropriate for their environment.
The architecture imposes no hard ordering between external sources.

### 7.2 Resolution Algorithm

When a finding arrives for resource R with resource fingerprint F and source priority P:

```
1. Query RemediationJobs with label mechanic.io/resource-fingerprint=F[:12]
   and full annotation mechanic.io/resource-fingerprint-full=F

2. If no active (non-Failed, non-Succeeded) RJ exists:
   → Proceed through the normal pipeline (stabilisation, cascade, readiness, create RJ)

3. If an active RJ exists with source priority Q:
   a. P <= Q: suppress the incoming finding. The active RJ has equal or higher quality signal.
   b. P > Q:  annotate the active RJ with the pending finding (§11).
              The incoming finding will be processed when the active RJ completes.

4. If only a Failed RJ exists:
   → Delete it and create a new RJ from the incoming finding (existing behaviour).

5. If only a Succeeded RJ exists:
   → Create a new RJ. The new RJ inherits the previous RJ's PR URL via
     FINDING_PREVIOUS_PR_URL (§12) so the agent updates rather than duplicates.
```

### 7.3 Source Priority on RemediationJob

The active RJ records its source priority so the resolution algorithm can compare:

```
label:      mechanic.io/source-priority: "90"       # string form of int; indexed
annotation: mechanic.io/source-type: "alertmanager"
annotation: mechanic.io/resource-fingerprint-full: "<64-char sha256>"
label:      mechanic.io/resource-fingerprint: "<fp[:12]>"   # existing pattern
```

---

## 8. AlertSourceReconciler

A new reconciler (`internal/provider/alertsource/`) that manages the lifecycle of external
alert sources. It is distinct from `SourceProviderReconciler` (which is informer-based) and
`RemediationJobReconciler` (which drives the RJ lifecycle).

Responsibilities:
- Watch `AlertSource` CRDs and reconcile webhook registration and polling state
- Maintain a `DynamicMux` for registered webhook paths (a thread-safe custom handler registry,
  not `http.ServeMux` directly — see §9 and `ALERT_SOURCE_LLD.md` §8)
- Drive polling goroutines (start on enable, stop on disable or CR deletion)
- Receive incoming `Finding` objects from both webhook handlers and polling loops via a
  shared buffered channel
- Drain the channel and run the priority resolution + dedup + create pipeline for each finding
- Watch `RemediationJob` objects; when an RJ reaches a terminal state with a
  `mechanic.io/pending-alert` annotation, create the new RJ from the pending finding directly
  (no `FindingCh` is wired into `RemediationJobReconciler`)
- Update `AlertSource.Status` with health and throughput information

The reconciler implements `ctrl.Reconciler` for two resource types:
1. `AlertSource` CRDs — for webhook/poll lifecycle management
2. `RemediationJob` objects (via `Watches`) — for pending-alert processing on terminal state

See `ALERT_SOURCE_RECONCILER_LLD.md` for the full design.

---

## 9. Webhook Receiver

The webhook HTTP server is embedded in the watcher binary on port `:8082`, separate from the
metrics server (`:8080`) and health probe server (`:8081`).

**Why same binary:**
- Alert volumes are low (tens per hour in typical clusters, not thousands per second).
  Horizontal scaling of the receiver is not needed.
- The finding pipeline requires K8s API access (cascade check, dedup query, RJ creation).
  Keeping the receiver in-process avoids IPC and the failure modes that come with it.
- controller-runtime supports starting non-leader-elected goroutines alongside the controller.
  The HTTP server starts unconditionally; the controller waits for the leader lock.

**Single-replica requirement and leader election:**

The watcher must be deployed with `replicas: 1` **OR** with `LeaderElection: true` in the
manager options (currently `false` in `main.go:74` — this must be changed as part of v2).

The reason is: the webhook server (`NeedLeaderElection() = false`) starts on every replica and
accepts inbound POST requests. The finding drain loop (`NeedLeaderElection() = true`) starts
only on the leader. On non-leader replicas, the webhook server receives POSTs, posts to the
buffered `FindingCh`, and the channel fills immediately because no drain loop is consuming from
it. Within seconds, the webhook handler starts returning `503 Service Unavailable`.

Alertmanager routes webhooks to a single configured URL. In a multi-replica deployment, the
`ClusterIP` Service load-balances across all replicas — including non-leaders. The result is
intermittent `503`s and dropped alerts with no visible error.

**Resolution options (choose one and document in Helm chart):**

Option A (recommended): **Enable leader election.** Set `LeaderElection: true` and
`LeaderElectionID: "mechanic-watcher"` in the manager options. Non-leader replicas will have
the webhook server running but the drain loop waiting. The `FindingCh` buffer (default 500)
absorbs any burst between election and drain-start. The `503` risk is bounded to the leader
failover window (typically <10s).

Option B (simpler): **Run single replica.** Document `replicas: 1` as the supported
deployment mode in the Helm chart. Webhook `503` during pod restarts is handled by
Alertmanager's own retry policy (default: retry with exponential backoff for 5 hours).

**v2 ships with Option A.** The `main.go` change enabling leader election is an explicit
prerequisite, not optional.

**Receiver design:**
- `net/http` server started in `main.go` as a `Runnable` registered with `mgr.Add()`
- Route table is managed by a `DynamicMux` (see `ALERT_SOURCE_LLD.md §8`) — a thread-safe
  custom handler that supports concurrent registration and deregistration without panicking
- Each registered path has a handler that:
  1. Validates HMAC signature if `spec.webhook.hmacSecretRef` is configured
  2. Delegates to the appropriate `AlertSourceAdapter.ParseWebhook(payload)`
  3. Posts resulting `Finding`s to the shared channel
  4. Returns `202 Accepted` immediately (processing is async)

**If the binary is restarted** while a webhook push is in flight, the push is lost. Alertmanager
will retry based on its own retry policy. This is acceptable — the alternative (durable queue)
adds significant operational complexity for minimal gain at these volumes.

---

## 10. Native Provider Configuration

Native providers are configured via Helm values only. No `AlertSource` CRD is created for
them. This keeps the zero-config getting-started experience intact.

```yaml
# charts/mechanic/values.yaml
nativeProviders:
  enabled: true          # global kill switch; false disables all six providers
  priority: 10           # source priority for all native findings
  pod: true
  deployment: true
  statefulset: true
  node: true
  pvc: true
  job: true
```

The `DISABLE_NATIVE_PROVIDERS` env var provides a runtime kill switch for all native
providers simultaneously. Per-provider disabling is available via individual env vars
(`DISABLE_NATIVE_PROVIDER_POD`, etc.) or the Helm values above.

**Rationale for Helm-only config:** Native provider configuration does not change at
runtime. The set of providers to enable is an infrastructure decision made at deploy time.
Putting this in a CRD would add the overhead of creating and managing an extra manifest
for what should be an opinionated default.

---

## 11. Pending Alert Annotation Pattern

When a higher-priority finding arrives for a resource that already has an active
lower-priority RemediationJob, the incoming finding is stored as an annotation on the
active RJ rather than being silently dropped or triggering a second RJ.

```
annotation key:   mechanic.io/pending-alert
annotation value: <JSON-encoded Finding>
```

**Lifecycle:**

```
Higher-priority finding arrives
        │
        ▼
Active lower-priority RJ found
        │
        ▼
Serialize Finding → set annotation mechanic.io/pending-alert on active RJ
        │
Active RJ reaches terminal state (Succeeded or Failed)
        │
        ▼
AlertSourceReconciler Watch on RemediationJob fires reconcile
for this specific RJ (predicate: has pending-alert annotation AND is terminal)
        │
        ▼
AlertSourceReconciler.handlePendingAlert(ctx, rj):
  1. Read and deserialize annotation
  2. ATOMICALLY clear annotation via MergePatch (prevents double-processing on restart)
  3. If RJ.Status.Phase == Succeeded and RJ.Status.PRRef != "":
       set PreviousPRURL on the pending Finding
  4. Run create pipeline (dedup + readiness gate + create new RJ)
```

**Why AlertSourceReconciler handles this (not RemediationJobReconciler):**
`RemediationJobReconciler` is provider-agnostic and must remain so. Alert-source-specific
pending-alert logic does not belong there. The `AlertSourceReconciler` already holds all
necessary dependencies (dedup logic, readiness gate, `client.Client`). It uses a `Watches()`
registration on `RemediationJob` objects with a predicate that fires only when an RJ with the
`mechanic.io/pending-alert` annotation reaches a terminal phase. No `FindingCh` is wired
into `RemediationJobReconciler` — it remains unchanged.

**Annotation idempotency guarantee:**
The annotation clear (step 2) is a `MergePatch` executed before the new RJ is created. If
the process restarts between annotation-clear and new-RJ-create, the pending finding is
dropped. This is acceptable: Alertmanager will re-deliver the alert on its next retry or poll
cycle, and the next delivery will create the new RJ through the normal path (since the active
RJ is now terminal and can be superseded).

**The two-PR problem is avoided because:**
- The first (lower-priority) RJ runs to completion and may open a PR.
- The second (higher-priority) RJ is created only after the first completes.
- The second RJ receives `FINDING_PREVIOUS_PR_URL` from the first RJ's status.
- The agent prompt includes logic to update the existing PR rather than open a new one
  when `FINDING_PREVIOUS_PR_URL` is set (see §12 and `AGENT_CONTEXT_LLD.md`).

**Constraints:**
- Only one pending alert is stored. If a second higher-priority finding arrives while a
  pending alert is already annotated, the annotation is overwritten with the newer finding.
  The assumption is that the newest signal is the most relevant.
- The annotation value is bounded by Kubernetes annotation size limits (256 KiB total
  annotations per object). A Finding is typically well under 1 KiB.
- A pending finding lost between annotation-clear and new-RJ-create (process restart) will
  be re-delivered on Alertmanager's next retry or poll cycle.

---

## 12. Agent Context Enrichment

Alert-sourced findings carry richer context than native findings. This context is made
available to the agent via additional environment variables injected into the agent Job.

New env vars (added to `jobbuilder`):

| Variable | Source | Notes |
|---|---|---|
| `FINDING_SOURCE_TYPE` | `RemediationJob.Spec.SourceType` | `"native"`, `"alertmanager"`, `"pagerduty"`, etc. |
| `FINDING_ALERT_NAME` | `RemediationJob.Spec.Finding.AlertName` | e.g. `"KubeDeploymentReplicasMismatch"` |
| `FINDING_ALERT_LABELS` | `RemediationJob.Spec.Finding.AlertLabels` | JSON map of all raw alert labels |
| `FINDING_PREVIOUS_PR_URL` | Previous `RemediationJob.Status.PRRef` | Set when a pending alert creates a new RJ after a Succeeded prior RJ |

Existing `FINDING_ERRORS` continues to hold a human-readable summary. For alert-sourced
findings this is constructed as: `"<alertname>: <key resource labels>"` so the field
remains useful without duplication.

The agent prompt template is updated to:
1. Use `FINDING_ALERT_LABELS` for additional investigation context when
   `FINDING_SOURCE_TYPE` is not `"native"`
2. When `FINDING_PREVIOUS_PR_URL` is set: update that PR rather than opening a new one.
   The agent should read the existing PR, understand what was already investigated, and
   amend or extend it with the new higher-quality signal.

See `AGENT_CONTEXT_LLD.md` for the full env var specification and prompt diff.

---

## 13. Priority Tuning — Future Direction

v2 stores priority as a mutable integer in `AlertSource.Spec.Priority`. This is designed
to be updated programmatically by a future feedback controller without any schema changes.

The intended feedback signal is explicit human action, not statistical inference:

- PR merged promptly → the alert source that generated the finding was producing actionable
  signal. The feedback controller may increment the source's priority.
- PR closed without merge (labelled `noise` or similar) → the source generated noise.
  Priority may be decremented.

This is reinforcement learning from explicit human labels, not unsupervised anomaly
detection. It is tractable, auditable, and does not require statistical modelling.

**Why not anomaly detection:** Statistical anomaly detection (CloudWatch-style) requires
extensive per-signal baseline calibration, produces high false-positive rates in K8s
environments where "normal" changes frequently (deployments, scaling events), and is
difficult to explain to operators. The explicit-feedback model requires no calibration and
produces changes that can be audited in git history.

**v2 does not implement the feedback controller.** The priority field must exist and be
mutable before the controller can be built. This is the only v2 prerequisite for that
future work.

---

## 14. RemediationJob CRD Changes

### 14.1 Spec changes

New fields on `RemediationJobSpec`:

```go
type RemediationJobSpec struct {
    // ... existing fields unchanged ...

    // SourcePriority is the priority of the source that created this RJ.
    // Used for cross-source deduplication. Stored as label mechanic.io/source-priority.
    // +optional
    // +kubebuilder:default=0
    // +kubebuilder:validation:Minimum=0
    SourcePriority int `json:"sourcePriority,omitempty"`

    // ResourceFingerprint is the v2 resource-level fingerprint SHA256 hex string.
    // Computed from namespace + kind + parentObject only (no error texts).
    // Distinct from the legacy Fingerprint field which includes error texts.
    // Used as the primary dedup key in v2; the legacy Fingerprint is retained for migration.
    // +optional
    ResourceFingerprint string `json:"resourceFingerprint,omitempty"`
}
```

**Note:** `SourceType string` already exists on `RemediationJobSpec` (added in v1 at
`api/v1alpha1/remediationjob_types.go:105`). It is NOT a new field. It does not need to be
added. Alert-sourced RJs set it to the adapter's `TypeName()` (e.g. `"alertmanager"`);
native RJs continue to set it to `r.Provider.ProviderName()` (i.e. `"native"`). No change
to `SourceType` is required in v2.

New fields on `FindingSpec` (the embedded Finding):

```go
type FindingSpec struct {
    // ... existing fields unchanged ...

    // AlertName is the name of the alert that triggered this finding.
    // Empty for native-sourced findings.
    AlertName string `json:"alertName,omitempty"`

    // AlertLabels contains the raw label set from the originating alert.
    // Empty for native-sourced findings.
    // NOTE: When AlertLabels is added, the hand-written DeepCopyInto in
    // remediationjob_types.go MUST be updated to deep-copy this map.
    // A shallow copy of a map aliases both copies to the same underlying data.
    AlertLabels map[string]string `json:"alertLabels,omitempty"`

    // PreviousPRURL is the GitHub PR URL from a prior RemediationJob for the same resource.
    // Set when this RJ was created after a pending-alert annotation on a Succeeded prior RJ.
    PreviousPRURL string `json:"previousPRURL,omitempty"`
}
```

### 14.2 SourceResultRef for alert-sourced RJs

`RemediationJobSpec.SourceResultRef` is currently required and points to the native K8s
object that triggered the RJ. Alert-sourced RJs have no such native object.

**Resolution:** Make `SourceResultRef` optional in the CRD schema by adding
`+kubebuilder:validation:Optional` and `omitempty`. For alert-sourced RJs, set a synthetic
sentinel value:

```go
SourceResultRef: v1alpha1.ResultRef{
    Name:      as.Name,         // AlertSource CR name, e.g. "alertmanager"
    Namespace: as.Namespace,    // AlertSource namespace, e.g. "mechanic"
}
```

**Note:** `as.Name` is the AlertSource CR name — not the adapter TypeName. By convention
these are expected to be the same (e.g. a CR with `spec.type: alertmanager` is expected to
be named `"alertmanager"`), but this is only a convention. `createRemediationJob` receives
the AlertSource CR name via `f.SourceCRName` and must use that field — not `f.SourceType` —
when setting `SourceResultRef.Name`. This ensures the sentinel value is always the CR name
regardless of naming conventions.

The `SourceProviderReconciler` cancellation logic (`provider.go:83-84`) checks
`rjob.Spec.SourceResultRef.Name == req.Name && .Namespace == req.Namespace` against the
deleted native object's namespace/name. The `AlertSource` namespace/name will never match a
native object's namespace/name because (a) native objects live in workload namespaces and
(b) the AlertSource name is `"alertmanager"`, `"pagerduty"`, etc. — not a workload name.
This makes the sentinel value safe without changing the cancellation logic. When the
`AlertSource` CR itself is deleted, the `AlertSourceReconciler` is responsible for any
cleanup (see `ALERT_SOURCE_RECONCILER_LLD.md §3.2`).

**Naming constraint:** `AlertSourceReconciler.Reconcile` disambiguates incoming reconcile
requests by trying `Get(RemediationJob)` before `Get(AlertSource)`. Since `RemediationJob`
names are always prefixed with `"mechanic-"` (e.g. `"mechanic-abc123def456"`), AlertSource
CR names must not use the `"mechanic-"` prefix. This prefix is reserved for system-generated
`RemediationJob` names and is safe to enforce as a naming convention — human-configured
AlertSource CRs like `"alertmanager"`, `"prod-alerts"`, or `"staging-pagerduty"` will never
collide with it.

### 14.3 Labels and annotations

New labels on `RemediationJob`:

| Label | Value | Purpose |
|---|---|---|
| `mechanic.io/resource-fingerprint` | `rfp[:12]` | Fast label-selector query for resource-level dedup |
| `mechanic.io/source-priority` | `"90"` | Cross-source priority comparison |

New annotations on `RemediationJob`:

| Annotation | Value | Purpose |
|---|---|---|
| `mechanic.io/resource-fingerprint-full` | 64-char SHA256 | Exact resource fingerprint match |
| `mechanic.io/source-type` | `"alertmanager"` | Human-readable source identification |
| `mechanic.io/error-summary` | plain text | Quick human inspection without parsing JSON |
| `mechanic.io/pending-alert` | JSON Finding | Higher-priority finding waiting for this RJ to complete |

The existing `remediation.mechanic.io/fingerprint` label and `spec.Fingerprint` field are
**retained unchanged** for backward compatibility during migration. Both fingerprints are set
on every new RJ. The v2 dedup query uses the new `mechanic.io/resource-fingerprint` label
first, with a fallback to the old label for RJs created before v2. See `FINGERPRINT_LLD.md §6`
for the full migration strategy.

### 14.4 Correlation behavior for alert-sourced RJs

Alert-sourced RJs go through the same `RemediationJobReconciler` correlation window and
correlator as native RJs. This is intentional: if an Alertmanager alert fires simultaneously
for multiple pods of the same Deployment, `SameNamespaceParentRule` may correctly group them.
If this behavior is undesirable for a specific source, set a short `stabilisationWindow` on
the `AlertSource` CR and note that correlation operates on the RJ level, not the alert level.

---

## 15. RBAC Changes

### New ClusterRole additions for mechanic-watcher

| Resource | Verbs | Reason |
|---|---|---|
| `alertsources.mechanic.io` | `get`, `list`, `watch`, `update`, `patch` | AlertSourceReconciler watches CRs and updates status |
| `alertsources.mechanic.io/status` | `get`, `patch`, `update` | Status subresource updates |

### New Service

A `Service` of type `ClusterIP` (or `LoadBalancer` if external Alertmanager) is added to
expose port `8082` for the webhook receiver. The specific exposure method (ClusterIP vs.
Ingress vs. LoadBalancer) is a Helm value, defaulting to ClusterIP.

```yaml
webhookService:
  type: ClusterIP
  port: 8082
  # If using Ingress:
  ingress:
    enabled: false
    host: ""
    tls: []
```

---

## 16. Data Flow

### 16.1 External Alert Path (Alertmanager webhook example)

```
1. Alertmanager fires KubeDeploymentReplicasMismatch for deployment=test-broken-image
   → POST http://mechanic.mechanic.svc:8082/webhook/v1/alertmanager
     body: { "alerts": [{ "labels": { "alertname": "KubeDeploymentReplicasMismatch",
                                       "namespace": "default",
                                       "deployment": "test-broken-image", ... },
                          "startsAt": "2026-02-25T05:25:27Z" }] }

2. Webhook handler (port 8082)
   → validate HMAC (if configured)
   → AlertmanagerAdapter.ParseWebhook(payload)
     → resolves resource: kind=Deployment, parentObject=Deployment/test-broken-image,
       namespace=default
     → constructs Finding{
         Kind:           "Deployment",
         Namespace:      "default",
         ParentObject:   "Deployment/test-broken-image",
         Errors:         `[{"text":"KubeDeploymentReplicasMismatch: deployment=test-broken-image"}]`,
         AlertName:      "KubeDeploymentReplicasMismatch",
         AlertLabels:    { "namespace":"default", "deployment":"test-broken-image", ... },
         SourceType:     "alertmanager",
         SourcePriority: 90,
         SkipStabilisation: true,
       }
   → post to finding channel

3. AlertSourceReconciler drains channel
   → resource fingerprint = SHA256("default" + "Deployment" + "Deployment/test-broken-image")
   → query RemediationJobs with label mechanic.io/resource-fingerprint=<fp[:12]>
   → none found
   → cascade check (node healthy, namespace not saturated) → pass
   → readiness gate (GitHub secret + LLM) → pass
   → create RemediationJob "mechanic-<fp[:12]>"
       labels:
         mechanic.io/resource-fingerprint: <fp[:12]>
         mechanic.io/source-priority: "90"
       annotations:
         mechanic.io/resource-fingerprint-full: <64-char fp>
         mechanic.io/source-type: "alertmanager"
         mechanic.io/error-summary: "KubeDeploymentReplicasMismatch: deployment=test-broken-image"
       spec:
         fingerprint: <same fp>           ← backward compat
         sourcePriority: 90
         finding:
           kind: "Deployment"
           namespace: "default"
           parentObject: "Deployment/test-broken-image"
           errors: `[{"text":"KubeDeploymentReplicasMismatch: ..."}]`
           alertName: "KubeDeploymentReplicasMismatch"
           alertLabels: { ... }

4. RemediationJobReconciler (unchanged) dispatches agent Job
   → FINDING_KIND=Deployment
      FINDING_NAMESPACE=default
      FINDING_PARENT=Deployment/test-broken-image
      FINDING_ERRORS=[{"text":"KubeDeploymentReplicasMismatch: ..."}]
      FINDING_SOURCE_TYPE=alertmanager
      FINDING_ALERT_NAME=KubeDeploymentReplicasMismatch
      FINDING_ALERT_LABELS={"namespace":"default","deployment":"test-broken-image",...}

5. Agent investigates, opens PR on GitOps repo
   → RemediationJob.Status.PRRef = "https://github.com/.../pull/42"
   → RemediationJob.Status.Phase = Succeeded
```

### 16.2 Priority Collision Path

```
1. Native Deployment provider detects test-broken-image (replica mismatch)
   → creates RJ with source priority 10

2. 30s later, Alertmanager fires for same deployment
   → AlertSourceReconciler: resource fingerprint found → active RJ with priority 10 exists
   → incoming priority 90 > 10
   → annotate active RJ: mechanic.io/pending-alert = <JSON of alert Finding>

3. Active RJ (native, priority 10) runs to completion
   → agent opens PR (or fails)
   → RJ phase = Succeeded, Status.PRRef = "https://github.com/.../pull/41"

4. RemediationJobReconciler (unchanged) detects terminal RJ with pending-alert annotation
   → AlertSourceReconciler Watch fires
   → handlePendingAlert(ctx, rj):
       clears annotation → sets PreviousPRURL = "https://github.com/.../pull/41"
       → creates new RJ from pending Finding (priority 90, alertmanager source)

5. New RJ dispatched with FINDING_PREVIOUS_PR_URL set
   → agent reads existing PR #41, understands what was already tried
   → updates PR #41 with richer context from alert labels rather than opening PR #42
```

---

## 17. Failure Modes

| Failure | Behaviour |
|---|---|
| Webhook HMAC validation fails | `400 Bad Request` returned; finding not processed; error logged and counted in `AlertSource.Status` |
| Alert label mapping cannot resolve a resource | Finding created with `Kind="Alert"`, `ParentObject=alertname/value` as fallback; agent receives full `FINDING_ALERT_LABELS` to investigate |
| AlertSource CR deleted while webhook path is active | `AlertSourceReconciler` deregisters the path; in-flight requests get `404` |
| Polling source returns error | Logged; retry on next interval; error counted in `AlertSource.Status` |
| Finding channel full (back-pressure) | Webhook handler returns `503 Service Unavailable`; Alertmanager will retry |
| Pending-alert annotation exceeds size limit | Extremely unlikely (Finding is <1 KiB); if it occurs, annotation write fails, error logged, incoming finding is dropped with a warning metric |
| Two higher-priority findings arrive while a lower-priority RJ is active | Second finding overwrites the first in the pending-alert annotation; only the most recent pending finding is processed |
| Controller restart while pending-alert annotation exists | AlertSourceReconciler Watch will re-fire when the terminal RJ is next reconciled; annotation is cleared atomically before new RJ creation; a restart between clear and create causes the pending finding to be dropped (re-delivered by Alertmanager on next retry/poll) |

---

## 18. Configuration Reference

### New Helm values

```yaml
# Native provider control
nativeProviders:
  enabled: true
  priority: 10
  pod: true
  deployment: true
  statefulset: true
  node: true
  pvc: true
  job: true

# Webhook server
webhookServer:
  enabled: true
  port: 8082
  service:
    type: ClusterIP
  ingress:
    enabled: false
    host: ""
    annotations: {}
    tls: []
```

### New environment variables

All new variables must be added to `internal/config/config.go`'s `Config` struct and
`FromEnv()` function before any component that reads them can compile.

| Variable | Default | Purpose |
|---|---|---|
| `DISABLE_NATIVE_PROVIDERS` | `false` | Disable all native providers globally |
| `DISABLE_NATIVE_PROVIDER_POD` | `false` | Disable pod provider only |
| `DISABLE_NATIVE_PROVIDER_DEPLOYMENT` | `false` | Disable deployment provider only |
| `DISABLE_NATIVE_PROVIDER_STATEFULSET` | `false` | Disable statefulset provider only |
| `DISABLE_NATIVE_PROVIDER_NODE` | `false` | Disable node provider only |
| `DISABLE_NATIVE_PROVIDER_PVC` | `false` | Disable PVC provider only |
| `DISABLE_NATIVE_PROVIDER_JOB` | `false` | Disable job provider only |
| `NATIVE_PROVIDER_PRIORITY` | `10` | Source priority for all native findings |
| `WEBHOOK_PORT` | `8082` | Port for the webhook HTTP server |
| `FINDING_CHANNEL_BUFFER` | `500` | Buffered channel size for incoming alert findings |

---

## 19. What Does Not Change

The following components are **unchanged** in v2. The v1 LLDs remain authoritative for them:

- `RemediationJobReconciler` — drives RJ lifecycle (Pending → Dispatched → Running → terminal).
  It does **not** receive a `FindingCh` parameter; the pending-alert annotation pattern is
  handled entirely by `AlertSourceReconciler` via a Watch on `RemediationJob` objects.
- `batch/v1 Job` construction (`jobbuilder`) — extended with new env vars but structure unchanged
- Correlator and correlation rules — unchanged (alert-sourced RJs pass through correlation;
  see §14.4)
- Cascade suppression logic — unchanged
- Circuit breaker (self-remediation) — unchanged
- Agent Docker image and `agent-entrypoint.sh` — extended with new env var handling
- GitHub authentication flow — unchanged
- Stabilisation window for native providers — unchanged (external sources bypass it)
- `RemediationJob` TTL and cleanup — unchanged

### Implementation prerequisites (must be done before any other v2 work)

1. **Generate `AlertSource` CRD YAML** from kubebuilder markers in `api/v1alpha1/alertsource_types.go`.
   Run `make generate manifests` to produce the CRD YAML. Add the CRD YAML to
   `charts/mechanic/crds/` and commit it. The `AlertSourceReconciler` cannot be registered
   with the manager until this CRD is installed in the cluster.

2. **Register `mechanic.io/v1alpha1` scheme** in `main.go`. The `AlertSource` type lives under
   the `mechanic.io` API group (distinct from `remediation.mechanic.io` used by `RemediationJob`).
   A separate `AddAlertSourceToScheme` function must be called at startup. If omitted,
   controller-runtime will panic when setting up the `AlertSourceReconciler`.

3. **Enable leader election** in `main.go`. Change `LeaderElection: false` →
   `LeaderElection: true, LeaderElectionID: "mechanic-watcher"`. Required for correct
   multi-replica behavior (webhook server runs on all replicas; drain loop runs on leader only).
   See §9 for the full rationale.

4. **Extend `domain.Finding`** with new v2 fields before any adapter or reconciler code:
   ```go
   type Finding struct {
       // ... existing fields unchanged ...
       AlertName         string            // empty for native
       AlertLabels       map[string]string // empty for native
       SourceType        string            // "native", "alertmanager", etc.
       SourceCRName      string            // AlertSource CR name; empty for native; used for counter attribution
       SourcePriority    int               // 0 for native (overridden by config)
       SkipStabilisation bool             // true for external alert sources
       PreviousPRURL     string            // set by handlePendingAlert on Succeeded prior RJ
   }
   ```

5. **Update `DeepCopyInto`** in `api/v1alpha1/remediationjob_types.go` to deep-copy
   `FindingSpec.AlertLabels map[string]string`. The current hand-written `DeepCopyInto` does a
   shallow copy of `Spec`, which aliases the map. Add explicit map copy:
   ```go
   if in.Spec.Finding.AlertLabels != nil {
       out.Spec.Finding.AlertLabels = make(map[string]string, len(in.Spec.Finding.AlertLabels))
       for k, v := range in.Spec.Finding.AlertLabels {
           out.Spec.Finding.AlertLabels[k] = v
       }
   }
   ```

6. **Add new fields to `config.Config`** and `FromEnv()` before any component reads them.
   See §18 for the full variable list.

7. **Add new constants file** `api/v1alpha1/annotations.go` with:
   ```go
   const (
       AnnotationPendingAlert            = "mechanic.io/pending-alert"
       AnnotationResourceFingerprintFull = "mechanic.io/resource-fingerprint-full"
       AnnotationSourceType              = "mechanic.io/source-type"
       AnnotationErrorSummary            = "mechanic.io/error-summary"
       LabelResourceFingerprint          = "mechanic.io/resource-fingerprint"
       LabelSourcePriority               = "mechanic.io/source-priority"
   )
   ```
   These are referenced throughout `AlertSourceReconciler` and `SourceProviderReconciler`.
   Defining them before the reconciler code prevents magic-string errors.

8. **Update `SourceProviderReconciler` (`internal/provider/provider.go`)** to:
   - Rename the existing `FindingFingerprint` call to `FindingFingerprintV1` (for `Spec.Fingerprint`)
   - Call the new `FindingFingerprint` (resource-only) for `Spec.ResourceFingerprint`
   - Add `mechanic.io/resource-fingerprint`, `mechanic.io/source-priority` labels and
     `mechanic.io/resource-fingerprint-full` annotation to every created RJ
   - Update the dedup query to use `mechanic.io/resource-fingerprint` label first, with
     fallback to `remediation.mechanic.io/fingerprint` for pre-v2 RJs
   See `FINGERPRINT_LLD.md §4.1` for the full pseudocode.

   **CRITICAL — this step MUST be committed in the same commit as step 4 above (extending
   `domain.Finding` and renaming `FindingFingerprint`).** The rename in step 4 changes what
   `domain.FindingFingerprint` returns (resource-only, no error texts). If `provider.go` is
   not simultaneously updated, its dedup query will silently use the new resource fingerprint
   to search a label field that holds old error-text fingerprints. Active native RJs will be
   invisible to dedup and duplicate RJs will be created on every reconcile with no compile
   error or panic to indicate the regression.

---

### Recommended implementation order

Work in this sequence to ensure the codebase compiles at each step:

```
Step 0a: Add annotations.go constants file (no dependencies)
Step 0b: Extend domain.Finding with new v2 fields (no dependencies)
Step 0c: Add new fields to config.Config and FromEnv()
Step 0d: Add new fields to RemediationJobSpec and FindingSpec in remediationjob_types.go
Step 0e: Update DeepCopyInto for AlertLabels map

Step 0f: *** MUST BE IN THE SAME COMMIT AS STEP 0b ***
         Update internal/provider/provider.go:
           - Rename FindingFingerprint call → FindingFingerprintV1 (for Spec.Fingerprint)
           - Call new FindingFingerprint (resource-only) for Spec.ResourceFingerprint
           - Update dedup query to use mechanic.io/resource-fingerprint label (v2 primary)
             with v1 fallback query (remediation.mechanic.io/fingerprint label)
           - Add mechanic.io/resource-fingerprint, mechanic.io/source-priority labels
             and mechanic.io/resource-fingerprint-full annotation to every created RJ
         CRITICAL: Steps 0b and 0f MUST be a single atomic commit. If Step 0b (which
         renames domain.FindingFingerprint → FindingFingerprintV1 and adds the new
         resource-only FindingFingerprint) is committed without 0f, then provider.go:162
         silently calls the new resource-only FindingFingerprint. The dedup query at
         provider.go:254 then uses the v2 resource fingerprint prefix to query a label
         (remediation.mechanic.io/fingerprint) that holds v1 fingerprint prefixes on
         existing RJs. The query finds nothing. Active native RJs are invisible to dedup
         and duplicate RJs are created for every reconcile until 0f is applied. This
         regression is silent — no compile error, no panic, just silent duplicate creation.

Step 1a: Write alertsource_types.go (AlertSource CRD Go types) and AddAlertSourceToScheme
Step 1b: Run make generate manifests → add CRD YAML to charts/mechanic/crds/

Step 2a: Write internal/provider/alertsource/dynamic_mux.go
Step 2b: Write internal/provider/alertsource/resource.go (resolveResource helper)
Step 2c: Write internal/provider/alertsource/adapters/alertmanager.go

Step 3:  Write internal/provider/alertsource/reconciler.go (AlertSourceReconciler)
Step 3b: Write internal/provider/alertsource/webhook_server.go
Step 3c: Write internal/provider/alertsource/poller.go

Step 4:  Update internal/jobbuilder/job.go (new FINDING_* env vars)
Step 5:  Update cmd/watcher/main.go (leader election, scheme reg, wiring)
Step 6:  Update agent-entrypoint.sh and prompt template
```

---

## 20. Scope and Success Criteria

### In scope for v2

- `AlertSource` CRD definition and schema (kubebuilder markers + generated YAML)
- Scheme registration for `mechanic.io/v1alpha1`
- `AlertSourceReconciler` (CR lifecycle, webhook registration, polling, finding channel drain,
  pending-alert handling via RemediationJob Watch)
- `DynamicMux` — thread-safe custom HTTP handler for dynamic webhook path registration
- Webhook HTTP server embedded in watcher binary
- Alertmanager adapter (webhook + poll)
- Resource-level fingerprint replacing error-text fingerprint (with v1 migration fallback)
- Cross-source priority resolution
- Pending alert annotation pattern with atomic clear
- Agent env var enrichment (`FINDING_SOURCE_TYPE`, `FINDING_ALERT_NAME`, `FINDING_ALERT_LABELS`, `FINDING_PREVIOUS_PR_URL`)
- Prompt update for `FINDING_PREVIOUS_PR_URL` handling
- Native provider enable/disable (Helm values + env vars)
- RBAC additions for `AlertSource`
- Helm chart additions (webhook Service, Ingress template, new values, CRD)
- Leader election enabled in `main.go`
- `domain.Finding` extended with v2 fields
- `DeepCopyInto` updated for map fields
- `config.Config` extended with new env vars

### Out of scope for v2

- Priority feedback controller (automated tuning based on PR outcomes)
- PagerDuty adapter (webhook)
- OpsGenie adapter (poll)
- Datadog, NewRelic, Splunk integrations
- Multi-cluster signal aggregation
- Alert source adapter plugin system (dynamic loading without recompile)

### Success Criteria

- [ ] An Alertmanager webhook fires for `KubeDeploymentReplicasMismatch` → exactly one
      `RemediationJob` is created with `sourceType=alertmanager`
- [ ] The same event detected by both the native Deployment provider and Alertmanager
      produces exactly one `RemediationJob`, owned by the higher-priority source
- [ ] A higher-priority alert arriving while a lower-priority RJ is active annotates the
      active RJ; `AlertSourceReconciler` Watch fires on terminal transition; annotation is
      atomically cleared; new RJ created with `FINDING_PREVIOUS_PR_URL` set
- [ ] The new RJ receives `FINDING_PREVIOUS_PR_URL` and the agent updates the existing PR
      rather than opening a second one
- [ ] Deleting an `AlertSource` CR deregisters its webhook path within one reconcile loop
- [ ] All native providers can be disabled individually or globally via Helm values
- [ ] A second POST to a registered webhook path while an existing handler is running does
      not panic (DynamicMux concurrent safety)
- [ ] All existing v1 tests pass unchanged
