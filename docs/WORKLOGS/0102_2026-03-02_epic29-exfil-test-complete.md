# Worklog 0102 — Epic 29 Exfil Test Complete + Epic 27 Kickoff

**Date:** 2026-03-02
**Epic:** 29 (post-impl verification) / 27 (kickoff)

## Summary

Completed the interrupted Tier 2 exfil test from the previous session (worklog 0101).
The prior session used the `default` SA instead of `mechanic-agent` SA, and also used
`HARDEN_AGENT_KUBECTL=true` instead of the correct `HARDEN_KUBECTL=true` env var name.
Both issues fixed this session; all tests passed.

## Tests Run

### Setup

Pod: `mechanic-sec-test` (image: `ghcr.io/lenaxia/mechanic-agent:v0.3.37`)
SA: `mechanic-agent`
Env: `HARDEN_KUBECTL=true`

### Tier 1: Write blocking (always-on)

All three write subcommand tests blocked at wrapper layer:
- `kubectl apply` → `[KUBECTL] ... blocked — write operations are not permitted`
- `kubectl delete` → `[KUBECTL] ... blocked — write operations are not permitted`
- `kubectl patch` → `[KUBECTL] ... blocked — write operations are not permitted`

### Tier 2: Hardened mode (`HARDEN_KUBECTL=true`)

All four hardened-mode blocks confirmed:
- `kubectl get secret` → `[KUBECTL-HARDENED] ... access to secrets is disabled in hardened mode`
- `kubectl exec somepod -- ls` → `[KUBECTL-HARDENED] ... exec is disabled in hardened mode`
- `kubectl port-forward pod/foo 8080:80` → `[KUBECTL-HARDENED] ... port-forward is disabled in hardened mode`
- `kubectl get all` → `[KUBECTL-HARDENED] ... 'get all' is disabled in hardened mode`

Allow-listed read confirmed working:
- `kubectl get pods` → returned live pod list (PASS)

### RBAC verification

- `kubectl get secret` with mechanic-agent SA (no hardened mode): blocked by RBAC
  `secrets is forbidden: User "system:serviceaccount:default:mechanic-agent" cannot get resource "secrets"`
  — confirms the ClusterRole does NOT grant secret reads (correct by design)

### Redact binary unit check

- `AKIAIOSFODNN7EXAMPLE` → `[REDACTED-AWS-KEY]` ✓
- `sk-*` short value (< 20 chars after prefix) → not redacted (correct — too short to be real key)
- `sk-*` long value (40+ chars) → `[REDACTED-SK-KEY]` ✓

### Notes

- `HARDEN_AGENT_KUBECTL` is not a valid env var for the wrapper — correct var is `HARDEN_KUBECTL`.
  The sentinel detection uses three layers: `/mechanic-cfg/harden-kubectl` file (Layer 1),
  `/proc/1/environ` scan for `HARDEN_KUBECTL=true` (Layer 2), current shell env `HARDEN_KUBECTL`
  (Layer 3). Only Layer 3 was in scope for the pod test.

## Finding 2026-02-27-005 Status

Reviewed THREAT_MODEL.md. The finding is closed by the Kyverno `restrict-agent-image` +
`enforce-agent-pod-security` policies (opt-in, `agent.kyvernoPolicy.enabled: true`).
No standalone finding entry exists in the threat model — it is correctly described inline
at the Kyverno section (line 316). No update to THREAT_MODEL.md required.

## Next

Epic 27 (PR feedback iteration) — reading README-LLM.md and starting Story 00.
