# Story 05: Threat Model Update

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** Medium
**Status:** Complete
**Depends on:** STORY_00, STORY_01, STORY_02, STORY_03, STORY_04

---

## User Story

As a **mendabot security reviewer**, I want `docs/SECURITY/THREAT_MODEL.md` to accurately
reflect the new kubectl wrapper controls and redaction improvements introduced in epic29,
so that the threat model remains the authoritative record of what is and is not protected.

---

## Background

`THREAT_MODEL.md` (v1.3, 2026-02-26) is the authoritative security design document.
It currently documents:

- **AV-03** (Cluster Secret exfiltration by agent) with accepted residual risk **AR-01**
  stating the agent can read all secrets cluster-wide.
- **AV-02** (Credential exposure via tool output) with **AR-02** noting regex redaction
  has false negatives.
- The "Tools deliberately NOT wrapped" table documenting why `curl`, `git`, `openssl`,
  `jq`, and `cat` are not redact-wrapped.

Epic29 makes three changes that affect the threat model:

1. The kubectl wrapper now blocks all write subcommands (Tier 1, always-on). This
   eliminates the residual risk that an LLM could mutate cluster state via `kubectl`
   even when RBAC happens to allow it.

2. The kubectl wrapper gains an opt-in hardened mode (Tier 2) that blocks
   `get/describe secret(s)`, `get all`, `exec`, and `port-forward`. This adds a
   wrapper-layer control for AV-03 in `agentRBACScope: namespace` deployments.

3. Five new built-in redact patterns plus user-extensible custom patterns reduce the
   false-negative rate for AV-02. The `age` private key blind spot is specifically
   closed.

None of the existing accepted risks for `curl`, `git`, `openssl`, or `jq` change.

---

## Acceptance Criteria

- [ ] AV-03 mitigations section updated to list:
      - Tier 1 (always-on): kubectl wrapper blocks all write subcommands
      - Tier 2 (opt-in): kubectl wrapper hardened mode blocks secret reads, exec,
        port-forward when `agent.hardenKubectl: true`
      - RBAC enforcement remains the server-side control (unaffected)
- [ ] AR-01 updated to note that `agent.hardenKubectl: true` eliminates the
      wrapper-layer path to secret reads; cluster-scoped RBAC (no `secrets` permission)
      plus hardened mode together provide two independent controls
- [ ] AV-02 mitigations section updated to list five new built-in patterns and
      user-extensible custom pattern support (`agent.extraRedactPatterns`)
- [ ] AR-02 updated to reflect the reduced false-negative surface (age key blind spot
      closed; sk-*, AKIA, JWT, non-Bearer Authorization now covered)
- [ ] "Tools deliberately NOT wrapped" table gains a new companion section
      "kubectl write-blocking" documenting Tier 1 and Tier 2 as a separate category
      from redact-wrapping
- [ ] Version bumped (e.g. `v1.3` → `v1.4`) and date updated to current date
- [ ] No other sections are changed — scope is strictly the above five changes

---

## Technical Implementation

### Changes to `docs/SECURITY/THREAT_MODEL.md`

#### 1. Version header

Update the version line and date:
```
v1.3, 2026-02-26  →  v1.4, <implementation date>
```

#### 2. AV-03 — Cluster Secret exfiltration by agent

Add to the mitigations list:

```markdown
**Mitigations (epic29):**
- **kubectl Tier 1 (always-on):** The `kubectl` wrapper blocks all write subcommands
  (`apply`, `create`, `delete`, `edit`, `patch`, `replace`, `rollout restart`,
  `rollout undo`, `scale`, `set`, `label`, `annotate`, `taint`, `drain`, `cordon`,
  `uncordon`) before the real binary is invoked. This is enforced at the tool layer
  regardless of RBAC.
- **kubectl Tier 2 (opt-in, `agent.hardenKubectl: true`):** When the hardened flag is
  set, the wrapper additionally blocks `kubectl get/describe secret(s)`, `kubectl get
  all`, `kubectl exec`, and `kubectl port-forward`. The flag is enforced via a
  `chmod 444` sentinel file at `/mechanic-cfg/harden-kubectl` mounted read-only into
  the agent container — it cannot be unset from within the container. Three-layer
  detection (sentinel file → `/proc/1/environ` → env var) mirrors the epic20 dry-run
  enforcement pattern.
- **RBAC (unchanged):** `clusterrole-agent.yaml` grants only `get/list/watch` on an
  explicit allowlist that excludes `secrets` in `agentRBACScope: cluster` (default).
  RBAC is the server-side control; Tier 1 and Tier 2 are wrapper-layer defense-in-depth.
```

#### 3. AR-01 — Agent can read all Secrets cluster-wide

Update the acceptance rationale:

```markdown
**AR-01 (updated):** In `agentRBACScope: cluster` (default), the agent SA has no
`secrets` RBAC permission — the API server denies secret reads at the server side.
The kubectl Tier 1 wrapper-layer control additionally prevents `kubectl get secret`
even if RBAC were misconfigured.

In `agentRBACScope: namespace`, the wildcard `resources: ["*"]` Role grants secret
reads in watched namespaces. Operators who consider this unacceptable should set
`agent.hardenKubectl: true` (kubectl Tier 2) to add a wrapper-layer block. The
combination of namespace-scoped RBAC + Tier 2 hardened mode provides two independent
controls.

`curl` with the mounted SA bearer token bypasses both wrapper-layer controls and
reaches the API server directly; server-side RBAC is the sole control for that path
(documented in AR-07).
```

#### 4. AV-02 — Credential exposure via tool output

Add to the mitigations list:

```markdown
**Mitigations (epic29):**
- Five new built-in patterns in `domain.RedactSecrets` / `domain.Redactor`:
  - `age` private key (`AGE-SECRET-KEY-1...` bech32 format) → `[REDACTED-AGE-KEY]`
  - `sk-*` API keys (OpenAI `sk-proj-...`, Anthropic `sk-ant-...`) → `[REDACTED-SK-KEY]`
  - AWS access key ID (`AKIA[A-Z0-9]{16}`) → `[REDACTED-AWS-KEY]`
  - JWT two-segment (`ey....ey....`) → `[REDACTED-JWT]`
  - Non-Bearer Authorization header schemes (`Token`, `Basic`, `Digest`, etc.)
- User-extensible custom patterns via `agent.extraRedactPatterns` (Helm) /
  `EXTRA_REDACT_PATTERNS` (env var). Custom patterns are applied by both the watcher's
  finding redaction and the `redact` binary inside agent Jobs.
```

#### 5. AR-02 — Regex redaction has false negatives

Update:

```markdown
**AR-02 (updated):** The built-in pattern set (now 16 patterns) covers the most common
credential formats. Known remaining gaps:
- Short Kubernetes Secret values (< 30 raw bytes) whose YAML key name is not a
  named pattern — unchanged from prior versions.
- Custom application-specific credential formats — mitigated by `agent.extraRedactPatterns`.
- The `age` private key blind spot is **closed** (epic29 STORY_02).
- `curl`/`jq`/`openssl` output is not redacted (by design — see "Tools deliberately
  NOT wrapped"). Residual risk unchanged.
- `git` output is not redacted (by design — see "Tools deliberately NOT wrapped").
  Residual risk unchanged.
```

#### 6. "Tools deliberately NOT wrapped" table

Add a new companion section after the table:

```markdown
### kubectl write-blocking (distinct from redact-wrapping)

The `kubectl` wrapper applies both output redaction (via `redact` binary — epic25) and
subcommand-level blocking (epic29). These are two independent mechanisms:

| Tier | Condition | Blocked operations |
|------|-----------|--------------------|
| Tier 1 (always-on) | All deployments | apply, create, delete, edit, patch, replace, rollout restart/undo, scale, set, label, annotate, taint, drain, cordon, uncordon |
| Tier 2 (opt-in) | `agent.hardenKubectl: true` | get/describe secret(s), get all, exec, port-forward |

Tier 1 enforces the agent's read-only design contract at the tool layer. Tier 2 adds
defence-in-depth for operators using `agentRBACScope: namespace` who want to prevent
secret exfiltration through the namespace wildcard RBAC.

The blocking is distinct from the redact pipeline: blocked calls exit immediately
with an error message to stderr — `kubectl.real` is never invoked and no output
reaches the `redact` filter.
```

---

## Definition of Done

- [ ] `docs/SECURITY/THREAT_MODEL.md` version bumped and date updated
- [ ] AV-03 mitigations section updated (Tier 1 + Tier 2 + RBAC note)
- [ ] AR-01 updated (cluster vs. namespace scope; hardenKubectl + RBAC dual control)
- [ ] AV-02 mitigations section updated (five new patterns + custom patterns)
- [ ] AR-02 updated (age blind spot closed; remaining gaps documented)
- [ ] New "kubectl write-blocking" section added after "Tools deliberately NOT wrapped"
      table
- [ ] No other sections modified
- [ ] Document is internally consistent (no contradictions with updated sections)
