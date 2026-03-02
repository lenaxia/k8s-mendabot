# Worklog 0103 — Epic 29 In-Cluster Validation

**Date:** 2026-03-02
**Session:** Full in-cluster validation of all 7 epic29 stories against v0.3.37
**Status:** Complete

---

## Objective

Systematically validate every epic29 acceptance criterion against the live cluster
running `ghcr.io/lenaxia/mechanic-agent:v0.3.37` / `mechanic-watcher:v0.3.37`.

---

## Work Completed

### Test environment

- Watcher: `ghcr.io/lenaxia/mechanic-watcher:v0.3.37`
- Agent image: `ghcr.io/lenaxia/mechanic-agent:v0.3.37`
- Helm values: `agent.hardenKubectl: true`, `agent.kyvernoPolicy.enabled: false`
- Test pods: `mechanic-epic29-val` (SA: `mechanic-agent`, no HARDEN_KUBECTL) and
  `mechanic-epic29-l2` (SA: `mechanic-agent`, `HARDEN_KUBECTL=true` in PID-1 env)

### STORY_00: Tier 1 write blocking

22/22 tests passed. All 16 write subcommands blocked (apply, create, delete, edit,
patch, replace, scale, set, label, annotate, taint, drain, cordon, uncordon,
rollout restart, rollout undo). All 6 read pass-throughs confirmed not blocked.
Error message: `[KUBECTL] kubectl <cmd> blocked — write operations are not permitted
in the mechanic-agent`. Exit code 1 confirmed.

### STORY_01: Tier 2 hardened mode + sentinel

13/13 Tier 2 tests passed (Layer 3 env var). 3/3 non-hardened-mode tests confirmed
Tier 2 does not activate without HARDEN_KUBECTL.

Three-layer sentinel validation:
- **Layer 1** (sentinel file): wrote `echo true > /mechanic-cfg/harden-kubectl &&
  chmod 444` → `kubectl get secret` blocked. Removed file → no longer blocked. PASS.
- **Layer 2** (/proc/1/environ): `mechanic-epic29-l2` pod has `HARDEN_KUBECTL=true`
  in PID-1 environ. Unset from shell env; wrapper still blocks. PASS.
- **Layer 3** (shell env): `HARDEN_KUBECTL=true kubectl get secret` → blocked. PASS.

Live job inspection (`mechanic-agent-010e57d2be71`):
- `dry-run-gate` init container present with correct sentinel write command
- `mechanic-cfg` emptyDir volume present
- Main container mounts `mechanic-cfg`
- `HARDEN_KUBECTL=true` env var set in main container

### STORY_02: Redact patterns

All primary patterns confirmed working:
- `sk-proj-*` / `sk-ant-*` long keys → `[REDACTED-SK-KEY]`
- `AKIAIOSFODNN7EXAMPLE` → `[REDACTED-AWS-KEY]`
- JWT two-segment → `[REDACTED-JWT]`
- `X-API-Key: ...` → `[REDACTED]`
- Base64 ≥40 chars → `[REDACTED-BASE64]`
- PEM full block (BEGIN/END) → `[REDACTED-PEM-KEY]`
- `AGE-SECRET-KEY-1` + 40+ chars → `[REDACTED-AGE-KEY]` (uppercase and lowercase)
- Short `sk-abc` (below threshold) → not redacted (correct)
- Normal text → not redacted

Note: full-length age public key (`age1` + 44 chars) is caught by the base64 catch-all.
This is documented and accepted behaviour in STORY_02 — the unit test uses a truncated
example (32 chars after `age1`) to demonstrate non-redaction at the boundary.

### STORY_03: Extra redact patterns

`EXTRA_REDACT_PATTERNS="MY-SECRET-[0-9]+" echo "MY-SECRET-12345" | redact` →
`[REDACTED-CUSTOM]`. PASS.

Invalid pattern: redact binary warns + skips (exit 0). This matches the story spec:
"logged warning + skip (redact binary, which cannot abort a running agent Job)". PASS.

Watcher config validates at startup (`domain.New` returns error on invalid RE2). PASS
(confirmed by unit test `invalid extra pattern returns error`).

### STORY_04: Config and deploy wiring

- `HARDEN_AGENT_KUBECTL` env var present in live watcher deployment
- `agent.hardenKubectl: false` default in `charts/mechanic/values.yaml` confirmed
- Live job (`mechanic-agent-010e57d2be71`) has:
  - `dry-run-gate` init container with correct sentinel command
  - `mechanic-cfg` emptyDir volume
  - Main container mounts `mechanic-cfg`
  - `HARDEN_KUBECTL=true` in main container env

### STORY_05: Threat model

`docs/SECURITY/THREAT_MODEL.md` contains all epic29 sections: kubectl wrapper controls
(Tier 1, Tier 2), new redact patterns (PEM, X-API-Key, age), Kyverno policy descriptions
including finding `2026-02-27-005` closure. Status: Authoritative.

### STORY_06: Kyverno policy

`charts/mechanic/templates/kyverno-policy-agent.yaml` contains all 9 rules:
`deny-agent-secret-read`, `deny-agent-pod-exec`, `deny-agent-pod-portforward`,
`deny-agent-writes`, `restrict-agent-image`, `enforce-agent-readonly-root`,
`enforce-agent-nonroot`, `enforce-agent-no-privilege-escalation`,
`enforce-agent-drop-capabilities`, `audit-agent-direct-api-calls`.
Default: `kyvernoPolicy.enabled: false`. Not applied to live cluster (confirmed).

---

## Key Decisions

None — this was a validation-only session. No code changes.

---

## Blockers

None.

---

## Tests Run

- `go test -timeout 30s -race ./...` — all 18 packages PASS (cached)
- In-cluster: 22/22 STORY_00 tests, 13/13 STORY_01 Tier 2 tests, 3 sentinel layers,
  redact binary pattern tests

---

## Next Steps

Epic 27 (PR feedback iteration) — design review before implementation.
Read all relevant existing code (interfaces, CRD types, jobbuilder, controller,
config, main.go) to validate the epic27 story designs for consistency with the
existing codebase before any code is written.

---

## Files Modified

- `docs/BACKLOG/epic29-agent-hardening/STORY_01_kubectl_hardened_mode.md` — acceptance
  criteria checkboxes updated to [x] (were []) after in-cluster validation
- `docs/WORKLOGS/0103_2026-03-02_epic29-in-cluster-validation.md` — this file
