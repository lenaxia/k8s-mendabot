# Phase 1: Static Code Analysis

**Date run:** 2026-02-24
**Reviewer:** automated (orchestrator + live cluster)
**Go version (dev):** go1.25.5
**Go version (Dockerfiles/deployed images):** go1.25.7

---

## 1.1 govulncheck

**Command:**
```bash
govulncheck ./...
```

**Output (Symbol Results — code paths reachable):**
```
Vulnerability #1: GO-2026-4341  net/url — Memory exhaustion in query parameter parsing
  Found in: net/url@go1.25.5    Fixed in: net/url@go1.25.6
Vulnerability #2: GO-2026-4340  crypto/tls — Handshake messages at incorrect encryption level
  Found in: crypto/tls@go1.25.5 Fixed in: crypto/tls@go1.25.6
Vulnerability #3: GO-2026-4337  crypto/tls — Unexpected session resumption
  Found in: crypto/tls@go1.25.5 Fixed in: crypto/tls@go1.25.7
```

**Module Results (present in dependency tree, code paths not directly reachable):**
```
GO-2026-4441  golang.org/x/net v0.30.0  Fixed: v0.45.0
GO-2026-4440  golang.org/x/net v0.30.0  Fixed: v0.45.0
GO-2026-4342  stdlib go1.25.5            Fixed: go1.25.6
GO-2025-3595  golang.org/x/net v0.30.0  Fixed: v0.38.0
GO-2025-3503  golang.org/x/net v0.30.0  Fixed: v0.36.0
GO-2024-3333  golang.org/x/net v0.30.0  Fixed: v0.33.0
```

**Analysis:**
- The 3 symbol-reachable CVEs are in the **local dev environment** Go 1.25.5 stdlib.
  The deployed watcher and agent images are built from `golang:1.25.7-bookworm@sha256:...`
  and are clean. No deployed binary exposure.
- `golang.org/x/net v0.30.0` is an indirect dependency with 4 unpatched CVEs.
  govulncheck reports the code does not call the vulnerable functions, but the module
  is significantly behind (v0.30.0 vs v0.45.0). Upgrade warranted.

**Findings:** 2026-02-24-P-002 (LOW — local toolchain); 2026-02-24-P-003 (MEDIUM — golang.org/x/net)

---

## 1.2 gosec

**Command:**
```bash
gosec -fmt json -out docs/SECURITY/2026-02-24_pentest_report/raw/gosec.json ./...
```

**Summary:** Files: 30 | Lines: 3674 | Issues: 4

| Rule | File | Line | Sev | Conf | Disposition |
|------|------|------|-----|------|-------------|
| G115 | `internal/config/config.go` | 222 | HIGH | MED | Carry-over 2026-02-24-005 — open |
| G115 | `cmd/watcher/main.go` | 105 | HIGH | MED | Carry-over 2026-02-24-005 — open |
| G101 | `internal/readiness/sink/github.go` | 16 | HIGH | LOW | Carry-over 2026-02-24-007 — false positive, open |
| G109 | `internal/config/config.go` | 222 | HIGH | MED | Carry-over 2026-02-24-005 — open |

No new gosec findings compared to the 2026-02-24 report.

---

## 1.3 go vet

**Command:** `go vet ./...`

**Output:** (no output — zero issues)

---

## 1.4 staticcheck

**Command:** `staticcheck ./...`

**Output:**
```
internal/provider/native/job_test.go:33:6: func ptr is unused (U1000)
```

`ptr[T any]` defined at line 33 is a generic helper not called in any test. Dead test code. No security impact.

**Findings:** 2026-02-24-P-001 (INFO)

---

## 1.5 Dependency audit

**go mod verify:** `all modules verified`

**Replace directives in go.mod:** none

**Notable outdated dependencies (selection):**
- `github.com/golang-jwt/jwt/v4 v4.5.0` → v4.5.2 (security fixes)
- `github.com/gorilla/websocket v1.5.0` → v1.5.3
- `golang.org/x/net v0.30.0` → v0.45.0 (6 CVEs in module tree)

---

## 1.6 Secret scanning

**git history scan:** No literal credentials found in tracked `.go`, `.yaml`, `.sh` files.

**Working tree scan:** All Secret YAML files use `<PLACEHOLDER>` values. No hardcoded credentials.

`githubAppSecretName = "github-app"` at `internal/readiness/sink/github.go:16` is a Kubernetes object name, not a credential (existing G101 false positive, 2026-02-24-007).

---

## Phase 1 Summary

**Total new findings:** 3
**Carry-over findings confirmed present:** 4 (005, 007 as gosec; 003, 004 not in scope of Phase 1)
**Findings added to findings.md:** 2026-02-24-P-001, 2026-02-24-P-002, 2026-02-24-P-003
