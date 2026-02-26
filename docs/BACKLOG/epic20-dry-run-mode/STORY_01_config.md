# Story: Config — DRY_RUN env var

**Epic:** [epic20-dry-run-mode](README.md)
**Priority:** High
**Status:** Not Started
**Estimated Effort:** 0.5 hours

---

## User Story

As a **cluster operator**, I want to set `DRY_RUN=true` on the watcher Deployment and have
the entire system respect it, so that I can evaluate mendabot in shadow mode without creating
any pull requests.

---

## Background

`internal/config/config.go` holds all runtime configuration. Every subsequent story in this
epic reads `cfg.DryRun` from the `config.Config` struct. This story adds that field and its
parsing logic — nothing else.

The existing pattern in `FromEnv` for boolean-like env vars does not exist yet; the closest
analogue is the string-enum pattern used for `INJECTION_DETECTION_ACTION`. For `DRY_RUN` we
accept `"true"` / `"1"` (enabled) and `"false"` / `"0"` / `""` (disabled). Any other
non-empty value is a startup error, consistent with how invalid values for other fields are
handled.

---

## Exact Code Location

| File | Line range (as of reading) | What changes |
|------|---------------------------|--------------|
| `internal/config/config.go` | struct `Config` (lines 13–30) | add `DryRun bool` field |
| `internal/config/config.go` | `FromEnv` body (lines 34–162) | add parsing block after `AgentWatchNamespaces` |
| `internal/config/config_test.go` | end of file | add five new test functions |

---

## Acceptance Criteria

- [ ] `config.Config` gains a `DryRun bool` field with the comment
  `// DRY_RUN — default false; set "true" or "1" to enable dry-run mode`
- [ ] `config.FromEnv` reads `DRY_RUN`; accepted values and their results:
  - `""` (unset) → `false` (default)
  - `"false"` → `false`
  - `"0"` → `false`
  - `"true"` → `true`
  - `"1"` → `true`
  - any other non-empty string → `return Config{}, fmt.Errorf("DRY_RUN must be 'true', 'false', '1', or '0', got %q", val)`
- [ ] `config_test.go` contains five new test functions (see Test Cases below)
- [ ] `go test -race ./internal/config/...` passes

---

## Implementation

Add the field to the struct immediately after `AgentWatchNamespaces`:

```go
// DRY_RUN — default false; set "true" or "1" to enable dry-run mode
DryRun bool
```

Add the parsing block at the end of `FromEnv`, just before `return cfg, nil`:

```go
dryRunStr := os.Getenv("DRY_RUN")
switch dryRunStr {
case "", "false", "0":
    cfg.DryRun = false
case "true", "1":
    cfg.DryRun = true
default:
    return Config{}, fmt.Errorf("DRY_RUN must be 'true', 'false', '1', or '0', got %q", dryRunStr)
}
```

---

## Test Cases

All test functions use the existing `setRequiredEnv(t)` helper already present in
`config_test.go` (line 250).

| Test Name | `DRY_RUN` value | Expected outcome |
|-----------|-----------------|-----------------|
| `TestFromEnv_DryRunDefault` | unset | `cfg.DryRun == false` |
| `TestFromEnv_DryRunFalse` | `"false"` | `cfg.DryRun == false` |
| `TestFromEnv_DryRunZero` | `"0"` | `cfg.DryRun == false` |
| `TestFromEnv_DryRunTrue` | `"true"` | `cfg.DryRun == true` |
| `TestFromEnv_DryRunOne` | `"1"` | `cfg.DryRun == true` |
| `TestFromEnv_DryRunInvalid` | `"yes"` | error containing `"DRY_RUN"` |

Example test skeleton:

```go
func TestFromEnv_DryRunDefault(t *testing.T) {
    setRequiredEnv(t)
    os.Unsetenv("DRY_RUN")

    cfg, err := config.FromEnv()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.DryRun != false {
        t.Errorf("DryRun default: got %v, want false", cfg.DryRun)
    }
}

func TestFromEnv_DryRunTrue(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("DRY_RUN", "true")

    cfg, err := config.FromEnv()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !cfg.DryRun {
        t.Error("DryRun: got false, want true")
    }
}

func TestFromEnv_DryRunOne(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("DRY_RUN", "1")

    cfg, err := config.FromEnv()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !cfg.DryRun {
        t.Error("DryRun '1': got false, want true")
    }
}

func TestFromEnv_DryRunFalse(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("DRY_RUN", "false")

    cfg, err := config.FromEnv()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.DryRun {
        t.Error("DryRun 'false': got true, want false")
    }
}

func TestFromEnv_DryRunZero(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("DRY_RUN", "0")

    cfg, err := config.FromEnv()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.DryRun {
        t.Error("DryRun '0': got true, want false")
    }
}

func TestFromEnv_DryRunInvalid(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("DRY_RUN", "yes")

    _, err := config.FromEnv()
    if err == nil {
        t.Fatal("expected error for DRY_RUN=yes, got nil")
    }
    if !contains(err.Error(), "DRY_RUN") {
        t.Errorf("error should mention DRY_RUN, got: %v", err)
    }
}
```

---

## Tasks

- [ ] Write all six test functions in `internal/config/config_test.go` (TDD — verify they
  fail before adding the field)
- [ ] Add `DryRun bool` to `config.Config` struct
- [ ] Add parsing block to `config.FromEnv`
- [ ] Run `go test -race ./internal/config/...` — all pass
- [ ] Run `go build ./...` — clean

---

## Dependencies

**Depends on:** epic00-foundation (config package exists)
**Blocks:** STORY_02 (jobbuilder), STORY_04 (reconciler)

---

## Definition of Done

- [ ] All six new config tests pass with `-race`
- [ ] Full test suite passes: `go test -timeout 120s -race ./...`
- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
- [ ] `DRY_RUN` documented in `charts/mendabot/templates/deployment-watcher.yaml` env block
  (as a commented-out optional variable with its default `false`)
