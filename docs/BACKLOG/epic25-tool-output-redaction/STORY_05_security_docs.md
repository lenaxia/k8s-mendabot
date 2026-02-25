# Story 05: Security Documentation

**Epic:** [epic25-tool-output-redaction](README.md)
**Priority:** Medium
**Status:** Not Started

---

## Acceptance Criteria

- [ ] New finding `2026-02-24-P-010` added to `docs/SECURITY/2026-02-24_pentest_report/findings.md`
- [ ] Phase 03 addendum added to `docs/SECURITY/2026-02-24_pentest_report/phase03_redaction.md`
- [ ] Phase 03 addendum added to `docs/SECURITY/2026-02-24_security_report/phase03_redaction.md`
- [ ] `docs/SECURITY/THREAT_MODEL.md` updated with the tool-call output attack vector

---

## Finding P-010 content

**Severity:** HIGH
**Status:** Remediated (epic25)
**Phase:** 3
**Attack Vector:** AV-02 (credential exposure via tool call output to external LLM API)

The LLM agent calls tools (kubectl, helm, etc.) via OpenCode's bash tool which uses
`child_process.spawn(command, { shell })`. The full stdout+stderr of every tool call is
buffered and returned verbatim to the LLM context, which is then sent to the external
LLM API. `domain.RedactSecrets` only runs at source in the native providers — it has
zero visibility into tool call output. A single `kubectl get secret <name> -o yaml`
call sends raw base64-encoded secret values to the external LLM API with no filtering.

**Resolution:** `cmd/redact` binary installed at `/usr/local/bin/redact`. Shell wrappers
for all 12 cluster/GitOps tools replace the originals at `/usr/local/bin/<tool>`, calling
`<tool>.real` and piping all output through `redact` before returning to the caller.
Imports `internal/domain.RedactSecrets` directly — same compiled patterns, zero drift.
Wrappers hard-fail (exit 1) if `redact` binary is absent at runtime.
