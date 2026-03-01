# Story 06: Kyverno Policy — Agent Hardening (Pod Security + Access Restriction)

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** Medium
**Status:** Not Started
**Depends on:** None (independent; references the agent ServiceAccount and Job labels
produced by the Helm chart)

---

## User Story

As a **security-conscious mechanic operator**, I want an optional Kyverno `ClusterPolicy`
that enforces server-side agent hardening — denying secret reads, write operations, exec,
and portforward for the agent ServiceAccount; restricting the agent Job container image
to an allowlisted prefix; and enforcing a minimal pod security profile (read-only root
filesystem, non-root user, no privilege escalation, no added capabilities) — so that
the agent's security posture is enforced at the Kubernetes API level independently of
RBAC and independently of the kubectl wrapper controls.

---

## Background

Epic29's kubectl wrapper controls (Tier 1 write block, Tier 2 hardened mode) and RBAC
operate at the shell-wrapper and API-server-grant layers respectively. Both are bypassed
by tools that call the Kubernetes API directly with the mounted SA token (`curl`) or by
changes to the container image that ship modified wrappers. Kyverno `ClusterPolicy`
resources operate at the API server admission webhook level — they are enforced for every
admission request regardless of what is running inside the container.

This story adds a single opt-in `ClusterPolicy` covering six distinct rules grouped into
three categories. All six rules live in one policy to minimise CRD count and Helm
template complexity.

### Why these six rules?

| Rule | Threat | Closes gap |
|------|--------|-----------|
| Secret read denial | AV-03 / AR-01 | Server-side mirror of Tier 2 hardened mode; catches `curl`-bypass path |
| exec/portforward denial | AV-03 | Blocks interactive container introspection by agent SA |
| Write denial | AV-13, AV-05 | Server-side mirror of Tier 1 kubectl wrapper; catches `curl`-bypass write path |
| Image allowlist | 2026-02-27-005 | Closes the partial fix from the 2026-02-27 pentest; prevents arbitrary container image injection via a crafted `RemediationJob` |
| readOnlyRootFilesystem | AV-02 | Prevents runtime replacement of wrapper scripts in `/usr/local/bin`, which would silently bypass the entire epic25/29 redaction chain |
| Non-root / no privilege escalation / no capabilities | AV-03, AV-14 | Removes the ability to remount the read-only `mechanic-cfg` sentinel volume, read other pods' `/proc` namespaces, or escape the container |

### curl audit rule rationale

`curl` cannot be blocked by Kyverno — Kyverno operates at admission time, not at runtime
syscall level. However, a Kyverno `Audit`-mode policy that fires whenever the agent SA
makes a direct request to the Kubernetes API (outside of the normal `kubectl`/controller
path) would surface exploitation attempts in the `PolicyReport` CR. This is weaker than
the `Enforce` rules but adds observability. The curl audit rule is therefore added as a
separate rule with `validationFailureAction: Audit`, not `Enforce`, to avoid breaking
legitimate SA requests while still creating a visible trail.

---

## Policy rule inventory

| Rule name | Category | Action | Purpose |
|-----------|----------|--------|---------|
| `deny-agent-secret-read` | A — access control | Enforce | Deny GET/LIST/WATCH on Secrets by agent SA |
| `deny-agent-pod-exec` | A — access control | Enforce | Deny pods/exec by agent SA |
| `deny-agent-pod-portforward` | A — access control | Enforce | Deny pods/portforward by agent SA |
| `deny-agent-writes` | B — write denial | Enforce | Deny all mutating verbs except RemediationJob status patch |
| `restrict-agent-image` | C — pod security | Enforce | Require agent Job containers to use the configured image prefix |
| `enforce-agent-pod-security` | C — pod security | Enforce | readOnlyRootFilesystem, runAsNonRoot, allowPrivilegeEscalation=false, no caps |
| `audit-agent-direct-api-calls` | D — observability | Audit | Surfaces direct Kubernetes API calls by agent SA in PolicyReport |

---

## Acceptance Criteria

### Helm chart — values

- [ ] `agent.kyvernoPolicy.enabled: false` exists in `values.yaml` under the `agent:` key
- [ ] `agent.kyvernoPolicy.allowedImagePrefix` exists with default
      `"ghcr.io/lenaxia/mechanic-agent"` — operators running custom images set this to
      their own registry prefix
- [ ] Inline comments explain both fields, Kyverno version requirement, and that
      `allowedImagePrefix` should be set when using a fork or private mirror
- [ ] When `agent.kyvernoPolicy.enabled: false` (default), `helm template` emits **zero**
      `kyverno.io/v1` resources — existing deployments completely unaffected

### Helm chart — policy structure

- [ ] A single `ClusterPolicy` is emitted (not multiple separate policies)
- [ ] Policy name: `{{ include "mechanic.fullname" . }}-agent-hardening`
- [ ] Policy carries the standard `mechanic.labels`
- [ ] `spec.background: false` on the enforce rules (admission-only)
- [ ] Enforce rules use `validationFailureAction: Enforce`
- [ ] The audit rule uses `validationFailureAction: Audit`

### Category A — access control (Enforce)

- [ ] `GET`/`LIST`/`WATCH` on `secrets` by the agent SA is denied;
      message: `"mechanic-agent is not permitted to read secrets"`
- [ ] `GET` on pods by the agent SA is **not** denied (legitimate investigation read)
- [ ] `GET` on configmaps by the agent SA is **not** denied
- [ ] `pods/exec` by the agent SA is denied;
      message: `"mechanic-agent is not permitted to exec into pods"`
- [ ] `pods/portforward` by the agent SA is denied;
      message: `"mechanic-agent is not permitted to port-forward"`

### Category B — write denial (Enforce)

- [ ] `CREATE`, `UPDATE`, `PATCH`, `DELETE`, `DELETECOLLECTION` on any resource by
      the agent SA is denied; message:
      `"mechanic-agent is not permitted to mutate cluster resources"`
- [ ] `RemediationJob` and `RemediationJob/status` are **excluded** from the write-deny
      rule — the agent must be able to `patch` its own `remediationjobs/status`
- [ ] `GET` on any resource is **not** denied by Category B

### Category C — pod security (Enforce)

**Image allowlist:**
- [ ] A `Job` spec whose agent container image starts with
      `{{ .Values.agent.kyvernoPolicy.allowedImagePrefix }}` is admitted
- [ ] A `Job` spec whose agent container image does NOT start with the configured prefix
      is denied; message:
      `"mechanic-agent Job image must use the configured allowedImagePrefix"`
- [ ] The rule matches only Jobs in `.Release.Namespace` bearing the label
      `app.kubernetes.io/managed-by: mechanic-watcher` so it does not affect unrelated Jobs
- [ ] When `allowedImagePrefix` is empty string (`""`), the image rule is **skipped**
      entirely — operators can disable image enforcement independently of the rest of the policy

**Pod security profile:**
- [ ] Agent Job pods are denied if `securityContext.readOnlyRootFilesystem` is not `true`;
      message: `"mechanic-agent containers must use a read-only root filesystem"`
- [ ] Agent Job pods are denied if `securityContext.runAsNonRoot` is not `true`;
      message: `"mechanic-agent containers must not run as root"`
- [ ] Agent Job pods are denied if `securityContext.allowPrivilegeEscalation` is not `false`;
      message: `"mechanic-agent containers must not allow privilege escalation"`
- [ ] Agent Job pods are denied if `securityContext.capabilities.drop` does not include
      `"ALL"`; message:
      `"mechanic-agent containers must drop all Linux capabilities"`
- [ ] The pod security rules match only pods in `.Release.Namespace` bearing the label
      `app.kubernetes.io/managed-by: mechanic-watcher`
- [ ] A compliant pod spec (all four fields correctly set) is admitted without error

### Category D — curl audit (Audit)

- [ ] A rule with `validationFailureAction: Audit` is emitted that fires when the agent SA
      makes a `GET`/`LIST`/`WATCH` request to any resource **other than** those in the
      agent ClusterRole allowlist (pods, nodes, pvcs, namespaces, events, services,
      endpoints, deployments, statefulsets, replicasets, daemonsets, batch/jobs, cronjobs,
      remediationjobs)
- [ ] Violations appear in `PolicyReport` CRs and Kyverno policy events — not in
      admission denials (Audit mode only)
- [ ] Normal `kubectl get pods/deployments/etc.` by the agent SA does **not** produce
      a PolicyReport entry (not a false positive)

### Threat model

- [ ] `STORY_05_threat_model_update.md` (or the threat model directly) is updated to
      reference Category C pod security rules as a mitigation for the wrapper-replacement
      attack surface (readOnlyRootFilesystem closes the path where an attacker modifies
      `/usr/local/bin/kubectl` at runtime to bypass the redaction wrapper)
- [ ] The curl audit rule is documented as closing the observability gap for EX-001

---

## Technical Implementation

### Two ClusterPolicy resources or one?

Kyverno supports mixing `validationFailureAction` per-rule since v1.9 by setting
`validationFailureAction` inside each rule's `validate:` block rather than at
`spec.validationFailureAction`. Use this to keep all rules in one `ClusterPolicy`:

```yaml
spec:
  rules:
  - name: deny-agent-secret-read
    # ...
    validate:
      validationFailureAction: Enforce
      # ...
  - name: audit-agent-direct-api-calls
    # ...
    validate:
      validationFailureAction: Audit
      # ...
```

Remove `spec.validationFailureAction` (the top-level field) when per-rule values are
used. This requires Kyverno v1.9+. Update the minimum version comment in `values.yaml`
accordingly.

### Helm template structure

```
charts/mechanic/templates/kyverno-policy-agent.yaml
```

Top-level guard:

```yaml
{{- if .Values.agent.kyvernoPolicy.enabled }}
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: {{ include "mechanic.fullname" . }}-agent-hardening
  labels:
    {{- include "mechanic.labels" . | nindent 4 }}
  annotations:
    policies.kyverno.io/title: "mechanic-agent hardening"
    policies.kyverno.io/category: "Security"
    policies.kyverno.io/severity: "high"
    policies.kyverno.io/minversion: "1.9.0"
    policies.kyverno.io/description: >-
      Enforces mechanic-agent access restrictions, pod security profile,
      and agent image allowlist. Requires Kyverno v1.9+.
spec:
  background: false
  rules:
  ...
{{- end }}
```

### Rule: `deny-agent-secret-read`

```yaml
- name: deny-agent-secret-read
  match:
    any:
    - resources:
        kinds: ["Secret"]
      subjects:
      - kind: ServiceAccount
        name: {{ include "mechanic.fullname" . }}-agent
        namespace: {{ .Release.Namespace }}
  validate:
    validationFailureAction: Enforce
    message: "mechanic-agent is not permitted to read secrets"
    deny:
      conditions:
        any:
        - key: "{{ "{{" }} request.operation {{ "}}" }}"
          operator: AnyIn
          value: ["GET", "LIST", "WATCH"]
```

### Rule: `deny-agent-pod-exec` and `deny-agent-pod-portforward`

```yaml
- name: deny-agent-pod-exec
  match:
    any:
    - resources:
        kinds: ["Pod/exec"]
      subjects:
      - kind: ServiceAccount
        name: {{ include "mechanic.fullname" . }}-agent
        namespace: {{ .Release.Namespace }}
  validate:
    validationFailureAction: Enforce
    message: "mechanic-agent is not permitted to exec into pods"
    deny: {}

- name: deny-agent-pod-portforward
  match:
    any:
    - resources:
        kinds: ["Pod/portforward"]
      subjects:
      - kind: ServiceAccount
        name: {{ include "mechanic.fullname" . }}-agent
        namespace: {{ .Release.Namespace }}
  validate:
    validationFailureAction: Enforce
    message: "mechanic-agent is not permitted to port-forward"
    deny: {}
```

### Rule: `deny-agent-writes`

The `exclude` block exempts `RemediationJob` and its status subresource so the agent
can still patch its own job status:

```yaml
- name: deny-agent-writes
  match:
    any:
    - resources:
        kinds: ["*"]
      subjects:
      - kind: ServiceAccount
        name: {{ include "mechanic.fullname" . }}-agent
        namespace: {{ .Release.Namespace }}
  exclude:
    any:
    - resources:
        kinds: ["RemediationJob", "RemediationJob/status"]
  validate:
    validationFailureAction: Enforce
    message: "mechanic-agent is not permitted to mutate cluster resources"
    deny:
      conditions:
        any:
        - key: "{{ "{{" }} request.operation {{ "}}" }}"
          operator: AnyIn
          value: ["CREATE", "UPDATE", "PATCH", "DELETE", "DELETECOLLECTION"]
```

### Rule: `restrict-agent-image`

Matches `Job` resources in the mechanic namespace bearing the watcher-managed label.
Uses a Kyverno pattern with a wildcard suffix. The rule is skipped entirely when
`allowedImagePrefix` is empty.

```yaml
{{- if .Values.agent.kyvernoPolicy.allowedImagePrefix }}
- name: restrict-agent-image
  match:
    any:
    - resources:
        kinds: ["Job"]
        namespaces: [{{ .Release.Namespace | quote }}]
        selector:
          matchLabels:
            app.kubernetes.io/managed-by: mechanic-watcher
  validate:
    validationFailureAction: Enforce
    message: "mechanic-agent Job image must use the configured allowedImagePrefix"
    pattern:
      spec:
        template:
          spec:
            containers:
            - name: mechanic-agent
              image: "{{ .Values.agent.kyvernoPolicy.allowedImagePrefix }}*"
{{- end }}
```

### Rule: `enforce-agent-pod-security`

Matches pods in the mechanic namespace bearing the watcher-managed label. Validates
all four security context fields. Uses Kyverno's `pattern` syntax for required values:

```yaml
- name: enforce-agent-pod-security
  match:
    any:
    - resources:
        kinds: ["Pod"]
        namespaces: [{{ .Release.Namespace | quote }}]
        selector:
          matchLabels:
            app.kubernetes.io/managed-by: mechanic-watcher
  validate:
    validationFailureAction: Enforce
    message: "mechanic-agent pods must enforce the required security context"
    pattern:
      spec:
        containers:
        - name: mechanic-agent
          securityContext:
            readOnlyRootFilesystem: true
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
```

### Rule: `audit-agent-direct-api-calls`

This rule fires in Audit mode when the agent SA accesses resource kinds that are NOT
in the ClusterRole allowlist. Because Kyverno `match` cannot express a "not in list"
directly, use an `exclude` block to carve out all permitted resource kinds and match
on `*` — violations are everything not explicitly excluded:

```yaml
- name: audit-agent-direct-api-calls
  match:
    any:
    - resources:
        kinds: ["*"]
      subjects:
      - kind: ServiceAccount
        name: {{ include "mechanic.fullname" . }}-agent
        namespace: {{ .Release.Namespace }}
  exclude:
    any:
    - resources:
        kinds:
        - Pod
        - Node
        - PersistentVolumeClaim
        - Namespace
        - Event
        - Service
        - Endpoints
        - Deployment
        - StatefulSet
        - ReplicaSet
        - DaemonSet
        - Job
        - CronJob
        - RemediationJob
        - RemediationJob/status
        - Pod/exec
        - Pod/portforward
        - Secret
  validate:
    validationFailureAction: Audit
    message: "mechanic-agent accessed a resource outside its standard ClusterRole allowlist — potential curl/SA-token bypass"
    deny:
      conditions:
        any:
        - key: "{{ "{{" }} request.operation {{ "}}" }}"
          operator: AnyIn
          value: ["GET", "LIST", "WATCH"]
```

Note: `Secret`, `Pod/exec`, and `Pod/portforward` are excluded from the audit rule
because they are already covered by the Enforce rules above and would generate
double-entries in the PolicyReport.

### Helm values additions

```yaml
agent:
  # ...existing fields...

  # Optional Kyverno ClusterPolicy enforcing agent hardening rules:
  # secret read denial, write denial, pod exec/portforward denial,
  # agent image allowlist, pod security profile, and a curl-bypass audit rule.
  #
  # Requires Kyverno v1.9+ installed in the cluster.
  # Default: false (Kyverno is not a required dependency).
  #
  # If agent.kyvernoPolicy.enabled is true and Kyverno is not installed,
  # helm install will fail with a CRD-not-found error for kyverno.io/v1/ClusterPolicy.
  kyvernoPolicy:
    enabled: false
    # Image prefix enforced on all mechanic-agent Job containers.
    # Jobs whose agent container image does not start with this prefix are denied.
    # Set to "" to disable image enforcement while keeping other rules active.
    # Override when using a fork, private mirror, or custom build of the agent image.
    allowedImagePrefix: "ghcr.io/lenaxia/mechanic-agent"
```

### readOnlyRootFilesystem: jobbuilder impact

Enforcing `readOnlyRootFilesystem: true` at the Kyverno layer requires that the agent
Job spec already sets this field — otherwise Kyverno will deny the pod at admission.
The jobbuilder (`internal/jobbuilder/job.go`) must be updated to set
`ReadOnlyRootFilesystem: ptr(true)` in the main container's `SecurityContext`.

The agent writes to two paths that must remain writable:
- `/tmp` — used by the kubectl wrapper for `mktemp` tmpfiles
- `/workspace` — git clone target, investigation report output

Both must be backed by `emptyDir` volumes (already the case for `/workspace`). Add
a dedicated `emptyDir` volume for `/tmp` mounted at `/tmp`. This is a **Go code change**
in addition to the Helm chart change — note it as a jobbuilder dependency.

Similarly, `runAsNonRoot: true` and `allowPrivilegeEscalation: false` must be set
in the jobbuilder Job spec. `capabilities.drop: ["ALL"]` must also be set.

**This means STORY_06 has a Go code component in `internal/jobbuilder/job.go`** — it is
not purely a Helm chart story. The Go changes are small (four fields on the
`SecurityContext` struct) but require TDD: write tests for the new security context
fields before implementing.

### Dependency: agent Dockerfile

`readOnlyRootFilesystem: true` requires the agent's process (opencode) to not write to
the root filesystem at startup. Verify that opencode's config and cache directories are
configurable or already point to `/workspace` or `/tmp`. If not, additional `emptyDir`
mounts for opencode's cache paths may be needed (e.g. `~/.config/opencode`,
`~/.cache/opencode`). This is a runtime verification item, not a code item — mark it
in the DoD as a manual check.

---

## Definition of Done

### Helm chart
- [ ] `charts/mechanic/templates/kyverno-policy-agent.yaml` created with all seven rules
- [ ] `charts/mechanic/values.yaml` has `agent.kyvernoPolicy.enabled: false` and
      `agent.kyvernoPolicy.allowedImagePrefix: "ghcr.io/lenaxia/mechanic-agent"` with
      comments explaining both fields and the Kyverno v1.9 requirement
- [ ] `helm template` with `enabled: false` emits zero `kyverno.io/v1` resources
- [ ] `helm template` with `enabled: true` emits a valid single `ClusterPolicy`
- [ ] `helm template` with `allowedImagePrefix: ""` emits a policy with no
      `restrict-agent-image` rule
- [ ] Helm template JMESPath escaping verified in rendered output
- [ ] `helm lint charts/mechanic/` passes with no errors

### Go (jobbuilder)
- [ ] `internal/jobbuilder/job.go` sets on the main agent container `SecurityContext`:
      `ReadOnlyRootFilesystem: true`, `RunAsNonRoot: true`,
      `AllowPrivilegeEscalation: false`, `Capabilities.Drop: ["ALL"]`
- [ ] A dedicated `emptyDir` volume is added for `/tmp` (in addition to the existing
      `/workspace` volume) so the kubectl wrapper's `mktemp` continues to work
- [ ] Tests in `internal/jobbuilder/job_test.go` verify all four `SecurityContext`
      fields and the `/tmp` volume mount
- [ ] Tests are written before the implementation (TDD)
- [ ] `go test -timeout 30s -race ./internal/jobbuilder/...` passes

### Runtime verification (manual)
- [ ] Agent Job pod starts successfully with `readOnlyRootFilesystem: true` — opencode
      does not write to the root filesystem at startup
- [ ] If opencode writes to `~/.config` or `~/.cache`, additional `emptyDir` mounts
      are added for those paths and documented in the jobbuilder changes

### Access control rules (manual verification or Policy test)
- [ ] `deny-agent-secret-read`: `curl` with agent SA token against secrets endpoint
      returns a Kyverno-generated 403 when hardening is enabled
- [ ] `deny-agent-writes`: `curl -X DELETE` with agent SA token is denied
- [ ] `restrict-agent-image`: a Job with a non-matching image is denied at admission
- [ ] `enforce-agent-pod-security`: a pod without `readOnlyRootFilesystem: true` is denied
- [ ] `audit-agent-direct-api-calls`: a novel `curl` request by the agent SA produces
      a `PolicyReport` entry; normal `kubectl get pods` does not

### Threat model
- [ ] `STORY_05_threat_model_update.md` or the threat model document is updated to:
      - Add readOnlyRootFilesystem as a mitigation for the wrapper-replacement attack
        (the path where an attacker modifies `/usr/local/bin/kubectl` to bypass redaction)
      - Add the curl audit rule as closing the observability gap for EX-001 (accepted
        HIGH path in the Exfil Leak Registry)
      - Reference finding 2026-02-27-005 as fully closed (image allowlist now enforced)
- [ ] `go test -timeout 30s -race ./...` passes
