# docs/SECURITY/

## Purpose

This folder contains the security review process for mechanic. Every review is
repeatable, consistent, and produces an evidence-based report. The process is
designed to be run periodically, after major changes, or before any production
deployment.

## Folder Contents

| File / Folder | Purpose |
|---------------|---------|
| [README.md](README.md) | This file — overview and quick-start |
| [THREAT_MODEL.md](THREAT_MODEL.md) | Authoritative threat model for mechanic |
| [PROCESS.md](PROCESS.md) | Step-by-step repeatable review process |
| [CHECKLIST.md](CHECKLIST.md) | Tick-off checklist — one copy per review run |
| [EXFIL_LEAK_REGISTRY.md](EXFIL_LEAK_REGISTRY.md) | Known LLM exfil paths — status, acceptance rationale, red team instructions |
| [REPORT_TEMPLATE/](REPORT_TEMPLATE/) | Template folder — copy to start a new report |
| `YYYY-MM-DD_security_report/` | Completed report folders (one per review run) |
| `YYYY-MM-DD_pentest_report/` | Completed pentest report folders |

## When to Run a Security Review

Run a full security review:

- Before any production deployment
- After any change to RBAC manifests, Dockerfiles, or the agent entrypoint script
- After any change to `internal/domain/` (redaction, injection detection)
- After any dependency upgrade (Go modules, tool versions in Dockerfile)
- After any change to the prompt template (`configmap-prompt.yaml`)
- After adding a new native provider
- On a scheduled cadence (minimum: quarterly)

Run Phase 11 (Exfil Test) additionally:

- After any change to the PATH-shadowing redaction wrappers in `docker/scripts/redact-wrappers/`
- After any change to the prompt hard rules
- After adding a new tool to the agent image (the new tool may bypass redaction)
- After any `EXFIL_LEAK_REGISTRY.md` entry transitions from `needs_remediation` to `remediated`
  (verify the fix holds)
- Whenever a new exfil technique is identified in the wild for LLM agents

## How to Run a Review

1. Read [THREAT_MODEL.md](THREAT_MODEL.md) to understand what you are defending
2. Follow [PROCESS.md](PROCESS.md) exactly — do not skip phases
3. Copy the template folder to start a new report:
   ```bash
   cp -r docs/SECURITY/REPORT_TEMPLATE docs/SECURITY/YYYY-MM-DD_security_report
   ```
4. Work through each phase of PROCESS.md, writing output into the corresponding
   file inside the report folder as you go
5. Use the copied `checklist.md` inside the report folder to track progress
6. Commit the completed report folder when the review is closed

## Report Naming

```
docs/SECURITY/YYYY-MM-DD_security_report/
```

Use the date the review was **completed**, not started. If the review spans multiple
days, use the completion date. Use lowercase with underscores — no spaces.

## Rules

- Reports are immutable once committed — never edit a historical report
- Every finding must be triaged: Remediated, Accepted (with rationale), or Deferred (with ticket)
- No finding rated HIGH or CRITICAL may be left in Accepted or Deferred state without
  explicit written sign-off from the project owner in the report
- The CHECKLIST must be fully completed (no unchecked items) before the report is closed
- If a test could not be run (e.g. no live cluster), it must be documented as SKIPPED
  with the specific reason — not silently omitted
- Phase 11 (Exfil Test) MUST NOT be permanently skipped: if a cluster is unavailable,
  document it as SKIPPED with reason, but the test must be run in the next review that
  does have a cluster
- Exfil test sentinel values MUST NOT be committed to this repository under any circumstances

## Severity Definitions

| Severity | Definition |
|----------|-----------|
| CRITICAL | Exploitable remotely with no authentication; leads to cluster compromise, data exfiltration, or persistent code execution |
| HIGH | Exploitable with limited access; significant data exposure or privilege escalation |
| MEDIUM | Exploitable under specific conditions; partial data exposure or limited privilege gain |
| LOW | Hardening gap with no direct exploitability; defence-in-depth improvement |
| INFO | Observation with no security impact; best-practice recommendation |
