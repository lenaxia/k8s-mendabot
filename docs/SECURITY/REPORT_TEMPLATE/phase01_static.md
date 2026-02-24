# Phase 1: Static Code Analysis

**Date run:**
**Reviewer:**

---

## 1.1 govulncheck

**Command:**
```bash
govulncheck ./...
```

**Output:**
```
<!-- paste full output, or "No vulnerabilities found." -->
```

**Findings:** (none / list findings → add each to findings.md)

---

## 1.2 gosec

**Command:**
```bash
gosec -fmt json -out raw/gosec.json ./...
```

**Summary of issues found:**
```
<!-- paste summary line from gosec, e.g. "Issues [High: 0, Medium: 2, Low: 1]" -->
```

**Issues reviewed:**

| Rule | File | Line | Severity | Disposition |
|------|------|------|----------|-------------|
| | | | | |

**Suppressed `#nosec` annotations reviewed:**

| File | Line | Rule | Rationale still valid? |
|------|------|------|------------------------|
| | | | |

**Findings:** (none / list findings → add each to findings.md)

---

## 1.3 go vet

**Command:**
```bash
go vet ./...
```

**Output:**
```
<!-- paste output, or "no issues found" -->
```

**Findings:** (none / list findings → add each to findings.md)

---

## 1.4 staticcheck

**Command:**
```bash
staticcheck ./...
```

**Output:**
```
<!-- paste output, or "no issues found" -->
```

**Findings:** (none / list findings → add each to findings.md)

---

## 1.5 Dependency audit

**go mod verify:**
```
<!-- paste output -->
```

**Outdated dependencies (`go list -u -m all | grep '['`):**
```
<!-- paste output -->
```

**Replace directives in go.mod:**
```
<!-- paste any replace directives, or "none" -->
```

**Pre-release or pseudo-version dependencies:**
```
<!-- list any, or "none" -->
```

**Findings:** (none / list findings → add each to findings.md)

---

## 1.6 Secret scanning

**git history scan result:**
```
<!-- paste any matches, or "no matches" -->
```

**Working tree scan result:**
```
<!-- paste any matches, or "no matches" -->
```

**Findings:** (none / list findings → add each to findings.md)

---

## Phase 1 Summary

**Total findings:** 0
**Findings added to findings.md:** (list IDs)
