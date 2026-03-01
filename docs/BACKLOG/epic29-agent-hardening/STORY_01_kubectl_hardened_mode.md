# Story 01: kubectl Wrapper — Tier 2 Hardened Mode + Sentinel + Helm Flag

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** High
**Status:** Not Started
**Depends on:** STORY_00 (kubectl wrapper Tier 1), STORY_04 (config + jobbuilder wiring)

---

## User Story

As a **security-conscious mendabot operator**, I want to opt into a hardened agent mode
that additionally blocks `kubectl get/describe secret(s)`, `kubectl get all`, `kubectl
exec`, and `kubectl port-forward`, so that even in `agentRBACScope: namespace` (where
the agent SA has wildcard resource access), the agent cannot read Kubernetes Secret
values or establish sessions into running pods.

---

## Background

In the default `agentRBACScope: cluster` mode, the agent SA has no `secrets` RBAC
permission — the API server denies secret reads server-side. In `agentRBACScope: namespace`
mode, `role-agent-ns.yaml` grants `resources: ["*"]` `get/list/watch` in watched
namespaces, which includes `secrets`. An adversarial LLM could exploit this to read
Secret values and exfiltrate them via tool output.

The hardened mode adds a wrapper-layer block as defense-in-depth. Unlike Tier 1 (always-on
write blocking), Tier 2 is opt-in because it trades capability for security: legitimate
investigation may sometimes benefit from `kubectl exec` (e.g. inspecting a running
container's state) or `kubectl get all` (broad resource sweep), and blocking these by
default would be over-restrictive for most deployments.

The hardened flag is made immutable using the same three-layer sentinel pattern
established in epic20 for dry-run enforcement. The sentinel file is written by the
`dry-run-gate` init container (extended in STORY_04) with `chmod 444` and mounted
read-only into the main container. The LLM cannot `unset` or overwrite it.

### Why these four operations?

| Operation | Risk |
|-----------|------|
| `kubectl get secret(s)[/name]` | Directly reads Secret values; base64-encoded data in output |
| `kubectl describe secret(s)[/name]` | In some clusters returns data fields; always reveals key names |
| `kubectl get all` | Returns all resources including `Secret` objects in some configurations |
| `kubectl exec` | Allows the agent to run arbitrary commands inside running pods, potentially reading mounted secrets or env vars |
| `kubectl port-forward` | Exposes internal cluster services to the agent container's network context, enabling exfiltration via non-kubectl paths |

---

## Acceptance Criteria

- [ ] `kubectl get secret` exits 1 with `[KUBECTL-HARDENED]` message when hardened mode
      is active
- [ ] `kubectl get secrets` (plural) exits 1 with `[KUBECTL-HARDENED]` message
- [ ] `kubectl get secret/my-secret` (slash notation) exits 1
- [ ] `kubectl get secret my-secret -n foo` exits 1
- [ ] `kubectl describe secret my-secret` exits 1
- [ ] `kubectl describe secrets` exits 1
- [ ] `kubectl get all` exits 1 with `[KUBECTL-HARDENED]` message
- [ ] `kubectl exec my-pod -- /bin/sh` exits 1 with `[KUBECTL-HARDENED]` message
- [ ] `kubectl port-forward svc/my-svc 8080:80` exits 1
- [ ] `kubectl get pods` is **not** blocked even in hardened mode
- [ ] `kubectl get configmaps` is **not** blocked
- [ ] `kubectl get all -n kube-system` is blocked (the `-n` flag does not bypass)
- [ ] `kubectl get pods,secrets` (multi-resource) exits 1 (contains `secret`)
- [ ] When hardened mode is **not** active, none of the Tier 2 blocks apply
- [ ] Sentinel file `/mendabot-cfg/harden-kubectl` is created with permissions `444`
- [ ] Sentinel file is mounted read-only in the main container — the agent cannot modify it
- [ ] Three-layer detection:
      - Layer 1: `/mendabot-cfg/harden-kubectl` contains `true`
      - Layer 2: `HARDEN_KUBECTL=true` in `/proc/1/environ`
      - Layer 3: `$HARDEN_KUBECTL` env var fallback
- [ ] `agent.hardenKubectl: false` in `values.yaml` is the default — existing deployments
      are unaffected
- [ ] `shellcheck docker/scripts/redact-wrappers/kubectl` passes with no errors

---

## Technical Implementation

### Sentinel detection (three-layer, mirrors epic20 dry-run pattern exactly)

```bash
# ── Hardened mode sentinel detection ─────────────────────────────────────────
_harden_kubectl="false"

# Layer 1: sentinel file (chmod 444, read-only volume mount — tamper-proof)
if [ -f /mendabot-cfg/harden-kubectl ] && \
   [ "$(cat /mendabot-cfg/harden-kubectl 2>/dev/null)" = "true" ]; then
    _harden_kubectl="true"
fi

# Layer 2: /proc/1/environ (PID-1 env — immutable from within container)
if [ "$_harden_kubectl" = "false" ] && [ -r /proc/1/environ ]; then
    if tr '\0' '\n' < /proc/1/environ 2>/dev/null | grep -q '^HARDEN_KUBECTL=true$'; then
        _harden_kubectl="true"
    fi
fi

# Layer 3: current shell env var (fallback / local testing)
if [ "$_harden_kubectl" = "false" ] && [ "${HARDEN_KUBECTL:-false}" = "true" ]; then
    _harden_kubectl="true"
fi
```

### Tier 2 block logic

Resource detection must handle all of the following forms:
- `kubectl get secret` — resource as positional arg after subcommand
- `kubectl get secrets` — plural form
- `kubectl get secret/my-name` — slash notation (resource/name)
- `kubectl get pod,secret,configmap` — comma-separated multi-resource list
- `kubectl get all` — wildcard that may include secrets
- `kubectl describe secret [name]`
- `kubectl exec pod [args]`
- `kubectl port-forward resource [port]`

```bash
if [ "$_harden_kubectl" = "true" ]; then
    _tier2_blocked="false"
    _tier2_reason=""

    case "$_subcmd" in
        exec)
            _tier2_blocked="true"
            _tier2_reason="exec is disabled in hardened mode"
            ;;
        port-forward)
            _tier2_blocked="true"
            _tier2_reason="port-forward is disabled in hardened mode"
            ;;
        get|describe)
            # Inspect all positional arguments for secret/secrets/secret/* patterns
            # and the 'all' resource shorthand.
            for _arg in "$@"; do
                case "$_arg" in
                    secret|secrets|secret/*|secrets/*)
                        _tier2_blocked="true"
                        _tier2_reason="access to secrets is disabled in hardened mode"
                        break
                        ;;
                    all)
                        _tier2_blocked="true"
                        _tier2_reason="'get all' is disabled in hardened mode (may include secrets)"
                        break
                        ;;
                    *secret*,*|*,*secret*)
                        # comma-separated multi-resource: foo,secret,bar
                        _tier2_blocked="true"
                        _tier2_reason="access to secrets is disabled in hardened mode"
                        break
                        ;;
                esac
            done
            ;;
    esac

    if [ "$_tier2_blocked" = "true" ]; then
        echo "[KUBECTL-HARDENED] kubectl $* blocked — ${_tier2_reason}" >&2
        exit 1
    fi
fi
```

### Edge case: `kubectl get pods,secrets`

The comma-separated multi-resource pattern `*secret*,*|*,*secret*` catches `pods,secrets`
and `secrets,pods` but not `pods,secrets,configmaps` (three-way). A more robust approach
splits on comma and checks each element:

```bash
# For the multi-resource comma case, split on comma and check each token
_res_list="${_arg}"
IFS=',' read -ra _resources <<< "$_res_list"
for _res in "${_resources[@]}"; do
    case "$_res" in
        secret|secrets|secret/*|secrets/*)
            _tier2_blocked="true"
            _tier2_reason="access to secrets is disabled in hardened mode"
            break 2
            ;;
    esac
done
```

The implementation should use this approach for robustness.

### Full wrapper structure after STORY_00 + STORY_01

```
[redact availability check]
[mktemp + trap]
[Tier 1: write-subcommand block — always-on]   ← STORY_00
[harden-kubectl sentinel detection]             ← STORY_01
[Tier 2: secret/exec/port-forward block]       ← STORY_01
[kubectl.real "$@" > tmpfile 2>&1]
[redact < tmpfile + exit code propagation]
```

### Sentinel init container (STORY_04 wires this — referenced here for context)

The `dry-run-gate` init container command (in `internal/jobbuilder/job.go`) is extended
to write `/mendabot-cfg/harden-kubectl` when `cfg.HardenAgentKubectl` is true. The
`mendabot-cfg` emptyDir volume is created when either `DryRun` or `HardenAgentKubectl`
is set. The main container always mounts it read-only.

---

## Definition of Done

- [ ] `docker/scripts/redact-wrappers/kubectl` updated with sentinel detection and Tier 2
      block logic
- [ ] All blocked operations in acceptance criteria verified to exit 1 with
      `[KUBECTL-HARDENED]` message
- [ ] All pass-through operations in acceptance criteria verified to succeed
- [ ] Multi-resource `kubectl get pods,secrets` blocked correctly
- [ ] `kubectl get pods,configmaps` (no secrets) passes through
- [ ] Hardened mode off by default — `kubectl get secret` passes through without the flag
- [ ] `shellcheck docker/scripts/redact-wrappers/kubectl` passes with no errors
- [ ] `docker/scripts/wrapper-test.sh` extended with hardened-mode test cases
- [ ] `go test -timeout 30s -race ./...` passes (Go changes are in STORY_04)
