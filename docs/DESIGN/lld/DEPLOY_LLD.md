# Domain: Deployment & RBAC — Low-Level Design

**Version:** 3.0
**Date:** 2026-02-20
**Status:** Implementation Ready
**HLD Reference:** [Sections 8, 14](../HLD.md)

---

## 1. Overview

### 1.1 Purpose

Defines every Kubernetes resource needed to run the mendabot-watcher and mendabot-agent
in a cluster. All resources are managed via Kustomize in `deploy/kustomize/`.

### 1.2 Design Principles

- **Least privilege** — watcher and agent have only the permissions they actually need
- **Namespace isolation** — all workloads run in `mendabot`; agent Jobs are also
  created in this namespace
- **No secrets committed** — secret manifests in the repo are placeholders only; real
  values are applied out-of-band or via Sealed Secrets / SOPS
- **Flux compatible** — the kustomize directory can be referenced directly from a Flux
  `Kustomization` resource

---

## 2. Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: mendabot
```

---

## 2.1 CRD

**Minimum Kubernetes version:** 1.25 (`x-kubernetes-validations` CEL rules entered beta
in 1.25; GA in 1.29). Kubernetes 1.29+ is recommended for production deployments where
CEL validation stability is required.

**CEL transition rules:** The immutability rules use `!has(oldSelf.field)` guards so
they only enforce immutability on UPDATE (where `oldSelf` is populated), not on CREATE
(where `oldSelf` fields are absent). Without this guard the rules would reject CREATE
requests where the field is set for the first time.

```yaml
# crd-remediationjob.yaml
# Hand-written until code generation is adopted.
# Full schema defined in REMEDIATIONJOB_LLD.md §2.
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: remediationjobs.remediation.mendabot.io
spec:
  group: remediation.mendabot.io
  names:
    kind: RemediationJob
    listKind: RemediationJobList
    plural: remediationjobs
    singular: remediationjob
    shortNames: [rjob]
  scope: Namespaced
  versions:
  - name: v1alpha1
    served: true
    storage: true
    subresources:
      status: {}
    additionalPrinterColumns:
    - name: Phase
      type: string
      jsonPath: .status.phase
    - name: Kind
      type: string
      jsonPath: .spec.finding.kind
    - name: Parent
      type: string
      jsonPath: .spec.finding.parentObject
    - name: Job
      type: string
      jsonPath: .status.jobRef
    - name: PR
      type: string
      jsonPath: .status.prRef
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            required: [fingerprint, sourceType, sinkType, finding, gitOpsRepo, gitOpsManifestRoot, agentImage, agentSA, sourceResultRef]
            x-kubernetes-validations:
            - rule: "!has(oldSelf.fingerprint) || self.fingerprint == oldSelf.fingerprint"
              message: "spec.fingerprint is immutable"
            - rule: "!has(oldSelf.sourceType) || self.sourceType == oldSelf.sourceType"
              message: "spec.sourceType is immutable"
            - rule: "!has(oldSelf.sinkType) || self.sinkType == oldSelf.sinkType"
              message: "spec.sinkType is immutable"
            properties:
              fingerprint:
                type: string
              sourceType:
                type: string
                description: "Which source provider created this object, e.g. k8sgpt"
              sinkType:
                type: string
                description: "Which sink the agent should use, e.g. github"
              sourceResultRef:
                type: object
                required: [name, namespace]
                properties:
                  name: {type: string}
                  namespace: {type: string}
              finding:
                type: object
                required: [kind, name, namespace, parentObject]
                properties:
                  kind: {type: string}
                  name: {type: string}
                  namespace: {type: string}
                  parentObject: {type: string}
                  errors: {type: string}
                  details: {type: string}
              gitOpsRepo: {type: string}
              gitOpsManifestRoot: {type: string}
              agentImage: {type: string}
              agentSA: {type: string}
          status:
            type: object
            properties:
              phase: {type: string}
              jobRef: {type: string}
              prRef: {type: string}
              message: {type: string}
              dispatchedAt: {type: string, format: date-time}
              completedAt: {type: string, format: date-time}
              conditions:
                type: array
                items:
                  type: object
```

---

## 3. ServiceAccounts

### 3.1 mendabot-watcher

```yaml
# serviceaccount-watcher.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mendabot-watcher
  namespace: mendabot
```

### 3.2 mendabot-agent

```yaml
# serviceaccount-agent.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mendabot-agent
  namespace: mendabot
```

---

## 4. RBAC — Watcher

### 4.1 ClusterRole (read Result CRDs, manage RemediationJobs, read Namespaces)

```yaml
# clusterrole-watcher.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mendabot-watcher
rules:
- apiGroups: ["core.k8sgpt.ai"]
  resources: ["results"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list"]
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch", "update"]
```

### 4.2 ClusterRoleBinding

```yaml
# clusterrolebinding-watcher.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mendabot-watcher
subjects:
- kind: ServiceAccount
  name: mendabot-watcher
  namespace: mendabot
roleRef:
  kind: ClusterRole
  name: mendabot-watcher
  apiGroup: rbac.authorization.k8s.io
```

### 4.3 Role (create Jobs and read pods in own namespace)

```yaml
# role-watcher.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mendabot-watcher
  namespace: mendabot
rules:
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "create", "watch", "delete"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
```

### 4.4 RoleBinding

```yaml
# rolebinding-watcher.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mendabot-watcher
  namespace: mendabot
subjects:
- kind: ServiceAccount
  name: mendabot-watcher
  namespace: mendabot
roleRef:
  kind: Role
  name: mendabot-watcher
  apiGroup: rbac.authorization.k8s.io
```

---

## 5. RBAC — Agent

The agent needs cluster-wide read access for investigation. This mirrors the permissions
already granted to the k8sgpt Deployment by its own Helm chart.

### 5.1 ClusterRole

```yaml
# clusterrole-agent.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mendabot-agent
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
```

The agent ClusterRole explicitly omits all mutating verbs: `create`, `update`, `patch`,
`delete`, `deletecollection`, `apply`, `escalate`, `bind`, `impersonate`.
`describe` is not a Kubernetes API verb (it is implemented client-side by kubectl) and
must not appear in RBAC rules.

### 5.2 ClusterRoleBinding

```yaml
# clusterrolebinding-agent.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mendabot-agent
subjects:
- kind: ServiceAccount
  name: mendabot-agent
  namespace: mendabot
roleRef:
  kind: ClusterRole
  name: mendabot-agent
  apiGroup: rbac.authorization.k8s.io
```

### 5.3 Role (status writeback in mendabot namespace)

```yaml
# role-agent.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: mendabot-agent
  namespace: mendabot
rules:
- apiGroups: ["remediation.mendabot.io"]
  resources: ["remediationjobs/status"]
  verbs: ["get", "patch"]
```

### 5.4 RoleBinding

```yaml
# rolebinding-agent.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mendabot-agent
  namespace: mendabot
subjects:
- kind: ServiceAccount
  name: mendabot-agent
  namespace: mendabot
roleRef:
  kind: Role
  name: mendabot-agent
  apiGroup: rbac.authorization.k8s.io
```

---

## 6. Secrets (Placeholders)

These files contain only the structure — no real values. They must be filled out of band
before applying the manifests. They should never contain real values in git.

The `-placeholder` suffix is intentional: `.gitignore` ignores `secret-*.yaml` but
explicitly un-ignores `secret-*-placeholder.yaml` via a negation rule. This means the
placeholder files are committed to git (so `kubectl apply -k` finds them), while any
file without the suffix (e.g. a copy renamed to `secret-github-app.yaml` after filling in
real values) is gitignored and safe from accidental commit.

**Operator instructions:** Copy the placeholder file, rename it to remove `-placeholder`,
and fill in real values. Never commit the renamed file.

### 6.1 github-app

```yaml
# secret-github-app-placeholder.yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-app
  namespace: mendabot
type: Opaque
stringData:
  app-id: "REPLACE_ME"
  installation-id: "REPLACE_ME"
  private-key: |
    REPLACE_ME
```

### 6.2 llm-credentials

```yaml
# secret-llm-placeholder.yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
  namespace: mendabot
type: Opaque
stringData:
  api-key: "REPLACE_ME"
  base-url: ""      # leave empty for OpenAI default
  model: ""         # leave empty to use OpenCode's default
```

---

## 6.3 Required GitHub Label

The agent applies the label `needs-human-review` to low-confidence PRs. This label must
be created in the target GitOps repository before the agent runs. It is a one-time manual
setup step:

```bash
gh label create needs-human-review \
  --repo lenaxia/talos-ops-prod \
  --description "PR opened by mendabot-agent requires human review before merge" \
  --color "e11d48"
```

If this label does not exist when the agent runs `gh pr edit --add-label`, the command
will fail non-fatally — the PR will still exist, but without the label. The agent will
not retry the label addition.

---

## 7. ConfigMap — Prompt

The prompt template is stored in a ConfigMap and mounted into the agent Job at `/prompt/prompt.txt`.
See [PROMPT_LLD.md](PROMPT_LLD.md) for the full prompt content.

```yaml
# configmap-prompt.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: opencode-prompt
  namespace: mendabot
data:
  prompt.txt: |
    <see PROMPT_LLD.md for content>
```

---

## 8. Watcher Deployment

```yaml
# deployment-watcher.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mendabot-watcher
  namespace: mendabot
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mendabot-watcher
  template:
    metadata:
      labels:
        app: mendabot-watcher
    spec:
      serviceAccountName: mendabot-watcher
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: watcher
        image: ghcr.io/lenaxia/mendabot-watcher:latest
        env:
        - name: GITOPS_REPO
          value: "lenaxia/talos-ops-prod"
        - name: GITOPS_MANIFEST_ROOT
          value: "kubernetes"
        - name: AGENT_IMAGE
          value: "ghcr.io/lenaxia/mendabot-agent:latest"
        - name: AGENT_NAMESPACE
          value: "mendabot"  # must equal the watcher's own namespace
        - name: AGENT_SA
          value: "mendabot-agent"
        - name: SINK_TYPE
          value: "github"
        - name: LOG_LEVEL
          value: "info"
        - name: MAX_CONCURRENT_JOBS
          value: "3"
        - name: REMEDIATION_JOB_TTL_SECONDS
          value: "604800"
        ports:
        - name: metrics
          containerPort: 8080
        - name: healthz
          containerPort: 8081
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop: ["ALL"]
```

---

## 9. Kustomization

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- namespace.yaml
- crd-remediationjob.yaml
- serviceaccount-watcher.yaml
- serviceaccount-agent.yaml
- clusterrole-watcher.yaml
- clusterrole-agent.yaml
- clusterrolebinding-watcher.yaml
- clusterrolebinding-agent.yaml
- role-watcher.yaml
- rolebinding-watcher.yaml
- role-agent.yaml
- rolebinding-agent.yaml
- configmap-prompt.yaml
- secret-github-app-placeholder.yaml
- secret-llm-placeholder.yaml
- deployment-watcher.yaml
```

---

## 10. Flux Integration

To deploy via Flux from the GitOps repo, add to `lenaxia/talos-ops-prod`:

```yaml
# kubernetes/apps/mendabot/ks.yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: mendabot-watcher
  namespace: flux-system
spec:
  interval: 10m
  path: ./deploy/kustomize
  prune: true
  sourceRef:
    kind: GitRepository
    name: k8s-mendabot   # GitRepository pointing at this repo
  dependsOn:
  - name: k8sgpt-operator          # ensure operator is deployed first
```

---

## 11. Applying Manually

```bash
# Dry run first
kubectl apply -k deploy/kustomize/ --dry-run=client

# Apply
kubectl apply -k deploy/kustomize/

# Verify
kubectl -n mendabot get all
kubectl -n mendabot get remediationjobs
kubectl get clusterrole mendabot-watcher mendabot-agent
```
