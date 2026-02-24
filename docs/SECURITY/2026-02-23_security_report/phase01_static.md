# Phase 1: Static Code Analysis

**Date run:** 2026-02-23
**Reviewer:** OpenCode (automated review)

---

## 1.1 govulncheck

**Command:**
```bash
govulncheck ./...
```

**Output:**
```
=== Symbol Results ===

Vulnerability #1: GO-2026-4341
    Memory exhaustion in query parameter parsing in net/url
  Standard library
    Found in: net/url@go1.25.5
    Fixed in: net/url@go1.25.6

Vulnerability #2: GO-2026-4340
    Handshake messages may be processed at the incorrect encryption level in crypto/tls
  Standard library
    Found in: crypto/tls@go1.25.5
    Fixed in: crypto/tls@go1.25.6

Vulnerability #3: GO-2026-4337
    Unexpected session resumption in crypto/tls
  Standard library
    Found in: crypto/tls@go1.25.5
    Fixed in: crypto/tls@go1.25.7

Your code is affected by 3 vulnerabilities from the Go standard library.
This scan also found 0 vulnerabilities in packages you import and 7
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
```

Full output in `raw/govulncheck.txt`.

**Findings:** 3 standard library CVEs → 2026-02-23-001

---

## 1.2 gosec

**Command:**
```bash
gosec -fmt json -out raw/gosec.json ./...
```

**Summary of issues found:**
```
Issues [High: 0, Medium: 0, Low: 1]
```

**Issues reviewed:**

| Rule | File | Line | Severity | Disposition |
|------|------|------|----------|-------------|
| G104 (errors unhandled) | internal/metrics/metrics.go | 93 | LOW | Not a security finding. Unhandled error in Prometheus metrics registration is a correctness issue only. No user-controlled data is involved. Recorded as INFO. |

**Suppressed `#nosec` annotations reviewed:**

| File | Line | Rule | Rationale still valid? |
|------|------|------|------------------------|
| (none) | — | — | — |

**Findings:** 1 INFO → 2026-02-23-002

---

## 1.3 go vet

**Command:**
```bash
go vet ./...
```

**Output:**
```
no issues found
```

**Findings:** none

---

## 1.4 staticcheck

**Command:**
```bash
staticcheck ./...
```

**Output:**
```
SKIPPED — staticcheck binary not installed in this environment.
```

**Findings:** SKIPPED — recommend installing for next review

---

## 1.5 Dependency audit

**go mod verify:**
```
all modules verified
```

**Outdated dependencies (`go list -u -m all | grep '['`):**
```
SKIPPED — command exceeded timeout. go mod verify confirms checksums are intact.
```

**Replace directives in go.mod:**
```
none
```

**Pre-release or pseudo-version dependencies:**
```
github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc (indirect, pseudo-version)
```

`davecgh/go-spew` is pinned to a specific commit SHA. It is an indirect transitive dependency via controller-runtime. No direct dependencies use pre-release versions.

**Findings:** none

---

## 1.6 Secret scanning

**git history scan result:**
```
no matches
```

**Working tree scan result:**
```
./docker/scripts/agent-entrypoint.sh: --token="$(cat $SA_TOKEN_FILE)" \
```

This is a runtime file read — not a hardcoded credential. The SA token is read from the filesystem at execution time, not embedded in the script.

Secret placeholder YAML files (`secret-github-app-placeholder.yaml`, `secret-llm-placeholder.yaml`) contain `REPLACE_ME` strings only — no real values committed.

**Findings:** none

---

## Phase 1 Summary

**Total findings:** 2
**Findings added to findings.md:** 2026-02-23-001, 2026-02-23-002
