# Story 04: Config, Jobbuilder, and Deploy Wiring

**Epic:** [epic29-agent-hardening](README.md)
**Priority:** Critical
**Status:** Not Started

---

## User Story

As a **mendabot developer**, I want the new `agent.hardenKubectl` and
`agent.extraRedactPatterns` Helm values to be wired end-to-end through the config
struct, watcher deployment, jobbuilder, and provider layer, so that the settings
configured by an operator in `values.yaml` reliably reach the kubectl wrapper sentinel
file and the `redact` binary inside every agent Job container.

---

## Background

This story is the integration glue for STORY_01 (sentinel) and STORY_03 (custom patterns).
It touches all the non-shell, non-domain layers: Go config parsing, jobbuilder Job spec
construction, Helm chart templates, and the provider layer's adoption of the new
`Redactor` struct.

The changes follow established patterns already used for `DryRun`:
- Config field + env var parsing in `internal/config/config.go`
- Conditional env var injection in `charts/mendabot/templates/deployment-watcher.yaml`
- Conditional sentinel write and env var injection in `internal/jobbuilder/job.go`
- `Config` struct extended in `internal/jobbuilder/job.go`

The `ExtraRedactPatterns` provider-layer wiring is new: the watcher must initialise a
`domain.Redactor` at startup and pass it to the six native providers so they use custom
patterns when building `Finding.Errors` and `Finding.Details`.

---

## Acceptance Criteria

### Config (`internal/config/config.go`)

- [ ] `Config` struct has `HardenAgentKubectl bool` field
- [ ] `Config` struct has `ExtraRedactPatterns []string` field
- [ ] `HARDEN_AGENT_KUBECTL` env var is parsed with `true`/`false`/`1`/`0` semantics
      (same as `DRY_RUN`); any other value is a startup error
- [ ] `EXTRA_REDACT_PATTERNS` env var is parsed as comma-separated strings; empty string
      → empty slice (no error); whitespace-only tokens are filtered out
- [ ] Invalid regex patterns in `EXTRA_REDACT_PATTERNS` cause a watcher startup error
      (validated via `domain.New` at config parse time)
- [ ] `internal/config/config_test.go` covers all four parsing cases

### Jobbuilder (`internal/jobbuilder/job.go`)

- [ ] `jobbuilder.Config` struct has `HardenAgentKubectl bool` field
- [ ] `jobbuilder.Config` struct has `ExtraRedactPatterns []string` field
- [ ] The `mechanic-cfg` emptyDir volume is added to the Job spec when
      `cfg.DryRun || cfg.HardenAgentKubectl` (previously only when `cfg.DryRun`)
- [ ] The `dry-run-gate` init container is added when `cfg.DryRun || cfg.HardenAgentKubectl`
- [ ] The init container command writes only the sentinels that are needed:
      - `DryRun` only: writes `dry-run`
      - `HardenAgentKubectl` only: writes `harden-kubectl`
      - Both: writes both (chained with `&&`)
- [ ] `HARDEN_KUBECTL=true` is appended to the main container's env when
      `cfg.HardenAgentKubectl` is true
- [ ] `EXTRA_REDACT_PATTERNS=<comma-joined>` is appended to the main container's env
      when `len(cfg.ExtraRedactPatterns) > 0`
- [ ] When neither `DryRun` nor `HardenAgentKubectl` is set, the Job spec is identical
      to the pre-epic29 spec (no regressions for existing deployments)
- [ ] `internal/jobbuilder/job_test.go` covers all combinations of flag states

### Helm chart (`charts/mendabot/values.yaml`)

- [ ] `agent.hardenKubectl: false` added under the `agent:` key
- [ ] `agent.extraRedactPatterns: []` added under the `agent:` key
- [ ] Inline comments explain both fields

### Helm chart (`charts/mendabot/templates/deployment-watcher.yaml`)

- [ ] `HARDEN_AGENT_KUBECTL: "true"` is conditionally emitted when
      `.Values.agent.hardenKubectl` is truthy (same pattern as `DRY_RUN`)
- [ ] `EXTRA_REDACT_PATTERNS: "<comma-joined>"` is conditionally emitted when
      `.Values.agent.extraRedactPatterns` is non-empty (joined with `,`)

### Provider layer (`internal/provider/native/*.go`)

- [ ] All six providers (`pod.go`, `deployment.go`, `statefulset.go`, `pvc.go`,
      `node.go`, `job.go`) receive a `*domain.Redactor` at construction time
- [ ] Each provider uses `r.redactor.Redact(text)` instead of `domain.RedactSecrets(text)`
      at its call sites
- [ ] The watcher's `cmd/watcher/main.go` (or wherever providers are instantiated)
      calls `domain.New(cfg.ExtraRedactPatterns)` and passes the resulting `*Redactor`
      to each provider constructor
- [ ] `domain.RedactSecrets` is preserved as a shim — no call sites outside the provider
      layer need updating

---

## Technical Implementation

### `internal/config/config.go` additions

```go
// HardenAgentKubectl — when true, the kubectl wrapper in agent Jobs blocks
// get/describe secret(s), get all, exec, and port-forward in addition to all
// write subcommands. Propagated to agent Jobs via HARDEN_KUBECTL env var and
// /mechanic-cfg/harden-kubectl sentinel file (read-only mount, chmod 444).
HardenAgentKubectl bool

// ExtraRedactPatterns — additional RE2 regex patterns applied by both the
// watcher's finding redaction and the agent's redact binary. Comma-separated
// in the EXTRA_REDACT_PATTERNS env var. Invalid patterns cause startup failure.
ExtraRedactPatterns []string
```

Parsing (add to `FromEnv()`):

```go
// HARDEN_AGENT_KUBECTL
hardenStr := os.Getenv("HARDEN_AGENT_KUBECTL")
switch hardenStr {
case "", "false", "0":
    cfg.HardenAgentKubectl = false
case "true", "1":
    cfg.HardenAgentKubectl = true
default:
    return Config{}, fmt.Errorf("HARDEN_AGENT_KUBECTL must be 'true', 'false', '1', or '0', got %q", hardenStr)
}

// EXTRA_REDACT_PATTERNS
if raw := os.Getenv("EXTRA_REDACT_PATTERNS"); raw != "" {
    for _, p := range strings.Split(raw, ",") {
        p = strings.TrimSpace(p)
        if p != "" {
            cfg.ExtraRedactPatterns = append(cfg.ExtraRedactPatterns, p)
        }
    }
}
// Validate early: fail at startup if any pattern is invalid.
if _, err := domain.New(cfg.ExtraRedactPatterns); err != nil {
    return Config{}, fmt.Errorf("EXTRA_REDACT_PATTERNS: %w", err)
}
```

### `internal/jobbuilder/job.go` — init container command builder

```go
// buildGateCommand returns the shell command for the dry-run-gate init container.
// Only writes the sentinels that are actually needed.
func buildGateCommand(dryRun, hardenKubectl bool) string {
    var cmds []string
    if dryRun {
        cmds = append(cmds, "echo -n 'true' > /mechanic-cfg/dry-run && chmod 444 /mechanic-cfg/dry-run")
    }
    if hardenKubectl {
        cmds = append(cmds, "echo -n 'true' > /mechanic-cfg/harden-kubectl && chmod 444 /mechanic-cfg/harden-kubectl")
    }
    return strings.Join(cmds, " && ")
}
```

The init container is created when `len(cmds) > 0`.

### `charts/mendabot/values.yaml` diff

```yaml
agent:
  image:
    repository: ghcr.io/lenaxia/mendabot-agent
    tag: ""
  # When true: blocks kubectl get/describe secret(s), get all, exec, and port-forward
  # in agent Jobs. Enforced via read-only sentinel file (cannot be bypassed from
  # within the container). Default: false.
  hardenKubectl: false
  # Additional RE2 regex patterns applied by both the watcher's finding redaction
  # and the agent redact binary inside every Job. Invalid patterns cause watcher
  # startup failure. Default: [] (no extra patterns).
  # Example:
  #   extraRedactPatterns:
  #     - 'CORP-[0-9]{8}'
  #     - 'INT-[A-Z]+-[0-9]+'
  extraRedactPatterns: []
```

### `charts/mendabot/templates/deployment-watcher.yaml` additions

```yaml
{{- if .Values.agent.hardenKubectl }}
- name: HARDEN_AGENT_KUBECTL
  value: "true"
{{- end }}
{{- if .Values.agent.extraRedactPatterns }}
- name: EXTRA_REDACT_PATTERNS
  value: {{ .Values.agent.extraRedactPatterns | join "," | quote }}
{{- end }}
```

### Provider constructor update (example: `pod.go`)

Current signature (representative):
```go
type PodProvider struct {
    client client.Client
    // ...
}

func NewPodProvider(c client.Client, ...) *PodProvider
```

New signature:
```go
type PodProvider struct {
    client   client.Client
    redactor *domain.Redactor
    // ...
}

func NewPodProvider(c client.Client, redactor *domain.Redactor, ...) *PodProvider
```

All six providers follow the same pattern. In `cmd/watcher/main.go`:

```go
redactor, err := domain.New(cfg.ExtraRedactPatterns)
if err != nil {
    // Should not happen — config.FromEnv validates patterns at startup.
    // Treat as fatal.
    setupLog.Error(err, "failed to initialise redactor")
    os.Exit(1)
}

podProvider    := native.NewPodProvider(mgr.GetClient(), redactor, ...)
deployProvider := native.NewDeploymentProvider(mgr.GetClient(), redactor, ...)
// ... etc.
```

---

## Definition of Done

- [ ] `internal/config/config.go` has both new fields with correct parsing
- [ ] `internal/config/config_test.go` covers `HardenAgentKubectl` and
      `ExtraRedactPatterns` parsing (valid, invalid, empty)
- [ ] `internal/jobbuilder/job.go` `Config` struct has both new fields
- [ ] `internal/jobbuilder/job_test.go` covers all init-container command combinations:
      - Neither flag: no `mechanic-cfg` volume, no init container
      - DryRun only: volume present, only `dry-run` sentinel written
      - HardenKubectl only: volume present, only `harden-kubectl` sentinel written
      - Both: volume present, both sentinels written
      - `HARDEN_KUBECTL=true` in env when flag is set
      - `EXTRA_REDACT_PATTERNS` in env when patterns are set
- [ ] `charts/mendabot/values.yaml` has both new `agent.*` fields with comments
- [ ] `charts/mendabot/templates/deployment-watcher.yaml` emits both new env vars
      conditionally
- [ ] All six native providers receive `*domain.Redactor` and use `redactor.Redact`
- [ ] `cmd/watcher/main.go` initialises a `*domain.Redactor` from config and passes it
      to all providers
- [ ] `go test -timeout 30s -race ./...` passes
- [ ] `go build ./...` succeeds
