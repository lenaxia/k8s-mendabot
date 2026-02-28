# Story 04: Config — MIN_SEVERITY Env Var and Reconciler Filter

**Epic:** [epic24-severity-tiers](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **cluster operator**, I want to set `MIN_SEVERITY=high` on the watcher Deployment so
that mechanic only investigates high and critical findings, reducing noise from medium and
low severity events.

---

## Background

`config.Config` is populated in `internal/config/config.go` via `FromEnv()`. The
`SourceProviderReconciler` in `internal/provider/provider.go` calls `ExtractFinding` and
then decides whether to create a `RemediationJob`. The severity filter belongs at the
reconciler level — providers are severity-unaware with respect to filtering (they always
return the highest applicable severity; the filter is a deployment-time policy).

---

## Design

### internal/config/config.go

Add `MinSeverity domain.Severity` to `Config`:

```go
type Config struct {
    // ... existing fields ...

    // MinSeverity is the minimum severity level for which a RemediationJob is created.
    // Findings below this threshold are silently dropped.
    // Default: domain.SeverityLow (all findings pass).
    MinSeverity domain.Severity
}
```

In `FromEnv()`:

```go
rawMinSeverity := os.Getenv("MIN_SEVERITY")
if rawMinSeverity != "" {
    if sv, ok := domain.ParseSeverity(rawMinSeverity); ok {
        cfg.MinSeverity = sv
    } else {
        return Config{}, fmt.Errorf("invalid MIN_SEVERITY value %q: must be one of critical, high, medium, low", rawMinSeverity)
    }
} else {
    cfg.MinSeverity = domain.SeverityLow
}
```

`FromEnv()` returns an error for invalid values — fail fast at startup, not silently at
reconcile time.

### internal/provider/provider.go

The actual reconcile flow in `provider.go` is (in order):

```
1.  Fetch object (r.Get)
2.  ExtractFinding
3.  nil check (return if no finding)
4.  Injection detection
5.  Namespace filter (WatchNamespaces / ExcludeNamespaces)
6.  Namespace annotation gate (ShouldSkip on Namespace object)  ← provider.go ~line 218
    ↑ INSERT SEVERITY FILTER HERE (step 6.5)
7.  Fingerprint computation                                       ← provider.go ~line 221
8.  Stabilisation window check                                    ← provider.go ~line 241
9.  Deduplication list query
10. Readiness gate
11. RemediationJob creation
```

The severity filter belongs at **step 6.5**: after the namespace annotation gate returns
(line ~219), before `domain.FindingFingerprint` is called (line ~221). This is the
earliest point where severity is known and all suppression gates have already run.

Inserting it after the stabilisation window (step 8) is **incorrect** — fingerprinting
must happen before the stabilisation window because the window is keyed by fingerprint.

Add the filter immediately before the `domain.FindingFingerprint` call:

```go
if !domain.MeetsSeverityThreshold(finding.Severity, r.Cfg.MinSeverity) {
    if r.Log != nil {
        r.Log.Info("finding suppressed",
            zap.Bool("audit", true),
            zap.String("event", "finding.suppressed.min_severity"),
            zap.String("provider", r.Provider.ProviderName()),
            zap.String("kind", finding.Kind),
            zap.String("namespace", finding.Namespace),
            zap.String("severity", string(finding.Severity)),
            zap.String("minSeverity", string(r.Cfg.MinSeverity)),
        )
    }
    return ctrl.Result{}, nil
}

fp, err := domain.FindingFingerprint(finding)
```

### RemediationJobSpec population

In the `RemediationJob` creation block (step 11), copy severity to spec:

```go
rjob := &v1alpha1.RemediationJob{
    Spec: v1alpha1.RemediationJobSpec{
        // ... existing fields ...
        Severity: string(finding.Severity),
    },
}
```

---

## Acceptance Criteria

- [ ] `config.Config.MinSeverity` field present; defaults to `domain.SeverityLow`
- [ ] `MIN_SEVERITY` env var parsed by `FromEnv()` using `domain.ParseSeverity`
- [ ] `FromEnv()` returns an error for unrecognised `MIN_SEVERITY` values
- [ ] `SourceProviderReconciler` drops findings below `MinSeverity` before fingerprinting
- [ ] `RemediationJobSpec.Severity` is set from `finding.Severity` when a `RemediationJob` is created
- [ ] Config tests cover: default (no env var), each valid value, invalid value
- [ ] Reconciler tests cover: finding passes filter, finding dropped by filter

---

## Tasks

- [ ] Write config tests for `MIN_SEVERITY` in `internal/config/config_test.go` (TDD — must fail first); follow the `setRequiredEnv` helper pattern already used in the file
- [ ] Update `internal/config/config.go` — add `import "github.com/lenaxia/k8s-mechanic/internal/domain"` (new import; not circular — domain does not import config)
- [ ] Add `MinSeverity domain.Severity` field to `Config` struct
- [ ] Add `MIN_SEVERITY` parsing block to `FromEnv()` using `domain.ParseSeverity`; default to `domain.SeverityLow` when env var is absent
- [ ] Write reconciler test cases for severity filter in `internal/provider/provider_test.go`: (a) finding with severity above threshold passes; (b) finding with severity below threshold is dropped with no RemediationJob created
- [ ] Update `internal/provider/provider.go` — add severity filter at step 6.5 (after namespace annotation gate, before `domain.FindingFingerprint`) using the audit log format shown above
- [ ] Update the `RemediationJob` creation block in `provider.go` to set `Severity: string(finding.Severity)`
- [ ] Run `go test -race -timeout 30s ./internal/config/...` — must pass
- [ ] Run `go test -race -timeout 30s ./internal/provider/...` — must pass
- [ ] Run `go build ./...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (domain.Severity and helpers), STORY_02 (RemediationJobSpec.Severity), STORY_03 (providers set Severity)
**Blocks:** STORY_05 (JobBuilder reads severity from RemediationJob spec)

---

## Definition of Done

- [ ] `MIN_SEVERITY` env var parsed and validated at startup
- [ ] Reconciler drops below-threshold findings
- [ ] `RemediationJobSpec.Severity` populated on all newly-created RemediationJobs
- [ ] All tests pass with `-race`
- [ ] `go vet ./...` clean
