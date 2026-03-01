# 0098 — epic29 STORY_01 + STORY_05: kubectl Hardened Mode and Threat Model Update

**Date:** 2026-02-28
**Epic:** epic29-agent-hardening
**Stories:** STORY_01 (kubectl Tier 2 hardened mode), STORY_05 (threat model update)
**Status:** Complete

---

## What Was Done

### STORY_01: kubectl Wrapper Tier 2 Hardened Mode

**File:** `docker/scripts/redact-wrappers/kubectl`

Added three-layer sentinel detection and Tier 2 block logic to the kubectl wrapper:

**Sentinel detection (mirrors epic20 dry-run pattern exactly):**
- Layer 1: `/mechanic-cfg/harden-kubectl` file (chmod 444, read-only volume mount)
- Layer 2: `/proc/1/environ` (PID-1 env — immutable from within container)
- Layer 3: `$HARDEN_KUBECTL` env var (fallback for local testing)

**Tier 2 blocked operations (when `_harden_kubectl=true`):**
- `kubectl exec` — exits 1 with `[KUBECTL-HARDENED]` message
- `kubectl port-forward` — exits 1 with `[KUBECTL-HARDENED]` message
- `kubectl get/describe secret`, `kubectl get/describe secrets` — exits 1
- `kubectl get/describe secret/<name>` (slash notation) — exits 1
- `kubectl get all` / `kubectl get all -n <ns>` — exits 1
- Multi-resource comma list containing `secret`/`secrets` (e.g. `pods,secrets`) — exits 1

**Not blocked (even in hardened mode):**
- `kubectl get pods`, `kubectl get configmaps`, `kubectl describe deployment`, `kubectl logs`
- `kubectl get pods,configmaps` (comma list without secrets)

**Implementation note:** The comma-splitting uses a pure string loop (`_comma_list="${_arg},"` + `%%,*` / `#*,`) rather than `set --`, so the original `"$@"` positional parameters are never clobbered. The real binary call at the end remains correct.

**File:** `docker/scripts/wrapper-test.sh`

Added 19 new hardened mode test cases:
- `check_hardened_blocked`: 12 cases (all blocked operations)
- `check_hardened_allowed`: 5 cases (non-blocked read operations)
- Default-off test: `kubectl get secret` with no `HARDEN_KUBECTL` passes through
- Sentinel file test: `kubectl get secret` blocked via file even when `HARDEN_KUBECTL` is unset

---

### STORY_05: Threat Model Update

**File:** `docs/SECURITY/THREAT_MODEL.md`

Version bumped from v1.5 to v1.6.

**Changes:**

1. **AV-02 controls updated** — Added "New patterns and extensibility (epic29)" section
   listing five new built-in patterns and `agent.extraRedactPatterns` custom pattern support.

2. **AV-02 residual risk updated** — Added bullets noting the `age` blind spot is closed
   and `sk-*`, `AKIA*`, JWT, and non-Bearer Authorization formats are now covered.

3. **AV-03 controls updated** — Added "epic29 controls" section documenting:
   - kubectl Tier 1 (always-on write blocking)
   - kubectl Tier 2 (opt-in hardened mode via sentinel file)
   - RBAC unchanged (server-side control)

4. **AR-01 updated** — Replaced the brief original note with detailed description of
   cluster-scope vs. namespace-scope RBAC, how Tier 1 + Tier 2 interact with each scope,
   and the curl bypass residual risk (AR-07).

5. **AR-02 updated** — Updated to mention the 16-pattern built-in set, the closed `age`
   blind spot, the new format coverage, and `agent.extraRedactPatterns` as the mitigation
   for custom application formats.

6. **New "kubectl write-blocking" section** — Added after the "Tools deliberately NOT
   wrapped" table. Documents Tier 1 vs. Tier 2 in a comparison table; explains that
   blocking is distinct from the redact pipeline (blocked calls exit before `kubectl.real`
   is ever invoked).

---

## Verification

```
go test -count=1 -timeout 30s -race ./...   # all 18 test packages: PASS
bash -n docker/scripts/redact-wrappers/kubectl   # Syntax OK
bash -n docker/scripts/wrapper-test.sh           # Syntax OK
```

---

## What Comes Next

- **STORY_06** — Kyverno ClusterPolicy (optional; `agent.kyvernoPolicy.enabled: false` default)
  - New file: `charts/mechanic/templates/kyverno-policy-agent.yaml`
  - Depends on: STORY_05 complete (AV-03 / AR-07 documentation referenced)
  - Priority: Medium
