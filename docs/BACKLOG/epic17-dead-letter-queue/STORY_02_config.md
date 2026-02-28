# Story 02: Config — MAX_INVESTIGATION_RETRIES Env Var

**Epic:** [epic17-dead-letter-queue](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 1.5 hours

---

## User Story

As a **mechanic operator**, I want to control the maximum number of investigation retries
via the `MAX_INVESTIGATION_RETRIES` environment variable so I can tune the retry cap to
match my infrastructure's reliability characteristics without rebuilding the image.

---

## Background

`internal/config/config.go` currently has no retry-related field. The `FromEnv` function
follows a consistent pattern for each variable:

1. Read the env var string.
2. If empty, apply a hard-coded default.
3. Otherwise `strconv.Atoi`, validate range, assign.

The new field must follow that exact pattern. The existing `MaxConcurrentJobs` parsing
block (lines 66–78) is the closest analogue and should be used as a copy-paste template.

The config value is consumed in two places:
- `SourceProviderReconciler.Reconcile` (STORY_04) — when creating a new `RemediationJob`,
  sets `rjob.Spec.MaxRetries = r.Cfg.MaxInvestigationRetries`.
- Operator startup / `main.go` — no change needed; `FromEnv` is already called there.

---

## Acceptance Criteria

- [x] `config.Config` has field `MaxInvestigationRetries int32`
- [x] `FromEnv` reads `MAX_INVESTIGATION_RETRIES`; default is `3`
- [x] Non-integer value returns a descriptive error
- [x] Value `<= 0` returns a descriptive error
- [x] All existing `config_test.go` tests still pass
- [x] New table-driven tests cover: unset (default 3), explicit value, zero → error, negative → error, non-integer → error

---

## Technical Implementation

### `internal/config/config.go`

#### 1. New field in `Config` struct (after `AgentWatchNamespaces` at line 29)

```go
// MaxInvestigationRetries is the maximum number of times a RemediationJob's
// owned batch/v1 Job may fail before the RemediationJob is permanently
// tombstoned. Populated from MAX_INVESTIGATION_RETRIES env var; default 3.
MaxInvestigationRetries int32 // MAX_INVESTIGATION_RETRIES — default 3
```

#### 2. Parsing block in `FromEnv` (insert after the `AgentWatchNamespaces` block,
before the final `return cfg, nil` at line 161)

```go
retriesStr := os.Getenv("MAX_INVESTIGATION_RETRIES")
if retriesStr == "" {
    cfg.MaxInvestigationRetries = 3
} else {
    n, err := strconv.Atoi(retriesStr)
    if err != nil {
        return Config{}, fmt.Errorf("MAX_INVESTIGATION_RETRIES must be an integer: %w", err)
    }
    if n <= 0 {
        return Config{}, fmt.Errorf("MAX_INVESTIGATION_RETRIES must be a positive integer, got %d", n)
    }
    cfg.MaxInvestigationRetries = int32(n)
}
```

Note: `strconv.Atoi` returns `int`; we cast to `int32`. The maximum valid `int32`
(2,147,483,647) is an absurd retry count but not a misconfiguration — no upper-bound
validation is needed.

---

## Test Cases

File: `internal/config/config_test.go` — add to the existing test file.

```go
// TestFromEnv_MaxInvestigationRetries_Default verifies unset → default 3.
func TestFromEnv_MaxInvestigationRetries_Default(t *testing.T) {
    setRequiredEnv(t) // helper already used in this file to set required vars
    t.Setenv("MAX_INVESTIGATION_RETRIES", "")

    cfg, err := config.FromEnv()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.MaxInvestigationRetries != 3 {
        t.Errorf("MaxInvestigationRetries default: got %d, want 3", cfg.MaxInvestigationRetries)
    }
}

// TestFromEnv_MaxInvestigationRetries table-driven cases.
func TestFromEnv_MaxInvestigationRetries(t *testing.T) {
    tests := []struct {
        name      string
        envValue  string
        wantValue int32
        wantErr   bool
    }{
        {
            name:      "unset uses default 3",
            envValue:  "",
            wantValue: 3,
        },
        {
            name:      "explicit value 1",
            envValue:  "1",
            wantValue: 1,
        },
        {
            name:      "explicit value 5",
            envValue:  "5",
            wantValue: 5,
        },
        {
            name:      "explicit value 10",
            envValue:  "10",
            wantValue: 10,
        },
        {
            name:    "zero is invalid",
            envValue: "0",
            wantErr: true,
        },
        {
            name:    "negative is invalid",
            envValue: "-1",
            wantErr: true,
        },
        {
            name:    "non-integer is invalid",
            envValue: "three",
            wantErr: true,
        },
        {
            name:    "float is invalid",
            envValue: "3.5",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            setRequiredEnv(t)
            if tt.envValue == "" {
                t.Setenv("MAX_INVESTIGATION_RETRIES", "")
            } else {
                t.Setenv("MAX_INVESTIGATION_RETRIES", tt.envValue)
            }

            cfg, err := config.FromEnv()
            if tt.wantErr {
                if err == nil {
                    t.Errorf("expected error for MAX_INVESTIGATION_RETRIES=%q, got nil", tt.envValue)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if cfg.MaxInvestigationRetries != tt.wantValue {
                t.Errorf("MaxInvestigationRetries = %d, want %d", cfg.MaxInvestigationRetries, tt.wantValue)
            }
        })
    }
}
```

Note: `setRequiredEnv(t)` is a local helper that sets all five required variables. If one
does not already exist in `config_test.go`, add it:

```go
func setRequiredEnv(t *testing.T) {
    t.Helper()
    t.Setenv("GITOPS_REPO", "org/repo")
    t.Setenv("GITOPS_MANIFEST_ROOT", "kubernetes/")
    t.Setenv("AGENT_IMAGE", "ghcr.io/lenaxia/mechanic-agent:latest")
    t.Setenv("AGENT_NAMESPACE", "mechanic")
    t.Setenv("AGENT_SA", "mechanic-agent")
}
```

If `config_test.go` already inlines the required vars in each test, copy that
existing pattern instead of introducing `setRequiredEnv`.

---

## Tasks

- [x] Write table-driven tests in `internal/config/config_test.go` (TDD — run first, must fail)
- [x] Add `MaxInvestigationRetries int32` field to `Config` struct
- [x] Add `MAX_INVESTIGATION_RETRIES` parsing block in `FromEnv` before the final `return`
- [x] Run: `go test -timeout 30s -race ./internal/config/...` — must pass
- [x] Run: `go vet ./internal/config/...` — must be clean

---

## Dependencies

**Depends on:** STORY_01 (struct types must exist before this story can be
meaningfully tested end-to-end; config.go itself has no import of v1alpha1, so
it can be implemented in parallel)
**Blocks:** STORY_04 (SourceProviderReconciler reads `cfg.MaxInvestigationRetries`
when creating a `RemediationJob`)

---

## Definition of Done

- [x] `config.Config.MaxInvestigationRetries int32` field present
- [x] `FromEnv` parses `MAX_INVESTIGATION_RETRIES` with default `3`
- [x] Zero and negative values produce a descriptive error from `FromEnv`
- [x] Non-integer value produces a descriptive error from `FromEnv`
- [x] `go test -timeout 30s -race ./internal/config/...` green
- [x] `go vet ./internal/config/...` clean
