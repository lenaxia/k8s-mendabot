# Story 00: kubectl Wrapper — Tier 1 Write Blocking (Always-On)

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** Critical
**Status:** Complete

---

## User Story

As a **mendabot operator**, I want the `kubectl` wrapper to unconditionally block all
write subcommands before they reach the real binary, so that an adversarial or confused
LLM agent cannot mutate cluster state via `kubectl` even if RBAC were misconfigured or
the agent SA were over-permissioned.

---

## Background

The mendabot agent is architecturally read-only on the cluster. HLD §2 explicitly lists
`kubectl apply` as a design non-goal, and `clusterrole-agent.yaml` grants only
`get/list/watch` across all resources. However, the current `kubectl` wrapper
(`docker/scripts/redact-wrappers/kubectl`) has no awareness of the subcommand being
called — it forwards all arguments to `kubectl.real` unconditionally.

This means:
- If the LLM constructs `kubectl apply -f -` or `kubectl delete pod foo`, the wrapper
  will run it (RBAC would then deny it server-side, but the attempt is made).
- In `agentRBACScope: namespace` mode, where `role-agent-ns.yaml` grants
  `resources: ["*"]` including write-adjacent resources, RBAC alone may not be
  sufficient.
- Defense-in-depth requires the tool layer to enforce the same contract as the RBAC.

The blocked subcommand list is derived from the full set of `kubectl` write verbs:
`apply`, `create`, `delete`, `edit`, `patch`, `replace`, `rollout restart`,
`rollout undo`, `scale`, `set`, `label`, `annotate`, `taint`, `drain`, `cordon`,
`uncordon`. Read-adjacent commands like `exec`, `port-forward` are handled in STORY_01.

Note: `rollout status`, `rollout history`, and `rollout pause/resume` are **not** blocked
here. `rollout pause` and `rollout resume` are write operations but are left to STORY_01's
scope review (they are less common and can be added if needed). `rollout restart` and
`rollout undo` are blocked as they trigger pod restarts and rollbacks respectively.

---

## Acceptance Criteria

- [x] `kubectl apply [...]` exits 1 with message `[KUBECTL] kubectl apply blocked — write
      operations are not permitted in the mendabot agent` to stderr
- [x] `kubectl create [...]` is blocked
- [x] `kubectl delete [...]` is blocked
- [x] `kubectl edit [...]` is blocked
- [x] `kubectl patch [...]` is blocked
- [x] `kubectl replace [...]` is blocked
- [x] `kubectl rollout restart [...]` is blocked
- [x] `kubectl rollout undo [...]` is blocked
- [x] `kubectl rollout status [...]` is **not** blocked (read-only)
- [x] `kubectl rollout history [...]` is **not** blocked (read-only)
- [x] `kubectl scale [...]` is blocked
- [x] `kubectl set [...]` is blocked
- [x] `kubectl label [...]` is blocked
- [x] `kubectl annotate [...]` is blocked
- [x] `kubectl taint [...]` is blocked
- [x] `kubectl drain [...]` is blocked
- [x] `kubectl cordon [...]` is blocked
- [x] `kubectl uncordon [...]` is blocked
- [x] `kubectl get pods` is **not** blocked (legitimate read)
- [x] `kubectl describe deployment foo` is **not** blocked (legitimate read)
- [x] `kubectl logs foo` is **not** blocked
- [x] `kubectl diff [...]` is **not** blocked (read-only diff)
- [x] Blocked calls exit with code 1 (not 0 — the LLM must see a failure)
- [x] All blocked calls write the error to stderr only (not stdout — preserves redact
      pipeline integrity for stdout)
- [x] `shellcheck docker/scripts/redact-wrappers/kubectl` passes with no errors
- [x] The redact pipeline (tmpfile → `redact < tmpfile`) remains intact for all
      non-blocked calls — this story does not change the output filtering behaviour

---

## Technical Implementation

### Wrapper structure after this story

The write-block logic is inserted **after** the `redact` availability check and `mktemp`
setup, and **before** the `kubectl.real` invocation:

```bash
#!/usr/bin/env bash
# kubectl wrapper — blocks write subcommands; pipes output through redact.
# Does NOT use set -e: the real binary may exit non-zero legitimately.

if ! command -v redact > /dev/null 2>&1; then
    echo "[ERROR] redact binary not found in PATH — aborting to prevent unredacted output" >&2
    exit 1
fi

_tmpfile=$(mktemp) || { echo "[ERROR] mktemp failed — aborting" >&2; exit 1; }
trap 'rm -f "$_tmpfile"' EXIT

# ── Tier 1: always-on write-subcommand block ─────────────────────────────────
_subcmd="${1:-}"
_blocked_write="false"

case "$_subcmd" in
    apply|create|delete|edit|patch|replace|scale|set|label|annotate|taint|drain|cordon|uncordon)
        _blocked_write="true"
        ;;
    rollout)
        case "${2:-}" in
            restart|undo)
                _blocked_write="true"
                ;;
        esac
        ;;
esac

if [ "$_blocked_write" = "true" ]; then
    echo "[KUBECTL] kubectl $* blocked — write operations are not permitted in the mendabot agent" >&2
    exit 1
fi
# ─────────────────────────────────────────────────────────────────────────────

kubectl.real "$@" > "$_tmpfile" 2>&1
_rc=$?

redact < "$_tmpfile"
_rr=$?
[ "$_rr" -ne 0 ] && exit "$_rr"
exit "$_rc"
```

### Design decisions

**Exit code 1 (not 0):** Blocked calls must fail so the LLM's tool-call result indicates
an error. A silent exit 0 would let the LLM believe the write succeeded, which could cause
incorrect follow-up behaviour. This matches the `gh` and `git` dry-run blocking pattern
which also exit 0 — however, dry-run blocking is intentionally silent because the agent
knows it is in dry-run mode. Write blocking is an enforcement action; the LLM should
receive an error.

**stderr only for the block message:** The blocked message goes to stderr. Since the
wrapper captures `kubectl.real` stdout+stderr into `$_tmpfile`, and the block exits before
reaching `kubectl.real`, `$_tmpfile` is empty. The `trap` cleans up the empty file. The
redact pipeline never runs for blocked calls. The LLM receives only the stderr message as
the tool result (OpenCode merges stdout+stderr — stderr written before exit is captured).

**`rollout` two-level check:** `rollout` is not universally blocked — `rollout status`,
`rollout history`, `rollout pause`, and `rollout resume` are legitimate read or
pause-only operations. Only `restart` and `undo` are write operations that trigger pod
replacement or rollback. The nested `case` on `${2:-}` handles this.

**No `set -euo pipefail`:** Consistent with all other wrappers — the real binary may exit
non-zero legitimately and the wrapper must preserve that code.

### `docker/scripts/wrapper-test.sh` extension

The existing wrapper test script should be extended to verify that write subcommands are
blocked. Add test cases for at minimum: `apply`, `delete`, `create`, `rollout restart`,
`rollout status` (must pass through).

---

## Definition of Done

- [x] `docker/scripts/redact-wrappers/kubectl` updated with Tier 1 write-block logic
- [x] All blocked subcommands from the acceptance criteria verified to exit 1
- [x] All read subcommands from the acceptance criteria verified to pass through
- [x] `shellcheck docker/scripts/redact-wrappers/kubectl` passes with no errors
- [x] `docker/scripts/wrapper-test.sh` extended with write-block test cases
- [x] `go test -timeout 30s -race ./...` still passes (no Go changes in this story)
