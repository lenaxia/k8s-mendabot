# docs/SECURITY/

## Purpose

This folder contains the security review process for mendabot. Every review is
repeatable, consistent, and produces an evidence-based report. The process is
designed to be run periodically, after major changes, or before any production
deployment.

## Folder Contents

| File | Purpose |
|------|---------|
| [README.md](README.md) | This file — overview and quick-start |
| [THREAT_MODEL.md](THREAT_MODEL.md) | Authoritative threat model for mendabot |
| [PROCESS.md](PROCESS.md) | Step-by-step repeatable review process |
| [CHECKLIST.md](CHECKLIST.md) | Tick-off checklist — one copy per review run |
| [REPORT_TEMPLATE.md](REPORT_TEMPLATE.md) | Template for the output report |
| `YYYY-MM-DD_Security_Report.md` | Completed reports (one per review run) |

## When to Run a Security Review

Run a full security review:

- Before any production deployment
- After any change to RBAC manifests, Dockerfiles, or the agent entrypoint script
- After any change to `internal/domain/` (redaction, injection detection)
- After any dependency upgrade (Go modules, tool versions in Dockerfile)
- After any change to the prompt template (`configmap-prompt.yaml`)
- After adding a new native provider
- On a scheduled cadence (minimum: quarterly)

## How to Run a Review

1. Read [THREAT_MODEL.md](THREAT_MODEL.md) to understand what you are defending
2. Follow [PROCESS.md](PROCESS.md) exactly — do not skip phases
3. Use [CHECKLIST.md](CHECKLIST.md) to track progress (copy it, do not edit the master)
4. Record every finding, evidence, and outcome in a new report file:
   ```
   docs/SECURITY/YYYY-MM-DD_Security_Report.md
   ```
   Use [REPORT_TEMPLATE.md](REPORT_TEMPLATE.md) as the starting point
5. Commit the completed report

## Report Naming

```
YYYY-MM-DD_Security_Report.md
```

Use the date the review was completed, not started. If the review spans multiple
days, use the completion date.

## Rules

- Reports are immutable once committed — never edit a historical report
- Every finding must be triaged: Remediated, Accepted (with rationale), or Deferred (with ticket)
- No finding rated HIGH or CRITICAL may be left in Accepted or Deferred state without
  explicit written sign-off from the project owner in the report
- The CHECKLIST must be fully completed (no unchecked items) before the report is closed
- If a test could not be run (e.g. no live cluster), it must be documented as SKIPPED
  with the specific reason — not silently omitted

## Severity Definitions

| Severity | Definition |
|----------|-----------|
| CRITICAL | Exploitable remotely with no authentication; leads to cluster compromise, data exfiltration, or persistent code execution |
| HIGH | Exploitable with limited access; significant data exposure or privilege escalation |
| MEDIUM | Exploitable under specific conditions; partial data exposure or limited privilege gain |
| LOW | Hardening gap with no direct exploitability; defence-in-depth improvement |
| INFO | Observation with no security impact; best-practice recommendation |
