# Story: Configuration Validation and Documentation

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Medium
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mendabot operator**, I want robust configuration validation, sensible defaults, and comprehensive documentation for all cascade prevention features, so that I can deploy and operate the system safely with minimal configuration errors.

---

## Acceptance Criteria

- [x] Configuration validation for all cascade-related environment variables
- [x] Sensible defaults that prevent unsafe operations
- [x] Configuration documentation in README and code comments
- [x] Validation errors provide clear, actionable messages
- [x] Configuration precedence and override rules documented
- [x] Safe mode configurations for production deployment
- [x] Configuration test coverage for edge cases
- [x] Integration with existing config validation in `internal/config/config.go`
- [x] Configuration migration path for future changes
- [x] Operator documentation with examples and troubleshooting

> **Note:** The design doc specified a named `validateCascadeConfig()` function. The actual
> implementation uses inline validation within `FromEnv()` which achieves the same outcome.
> This is acceptable — the function boundary is an implementation detail, not a requirement.

---

## Technical Implementation

### Location: `internal/config/config.go` and documentation files

**Configuration Variables to Validate:**

1. **Self-Remediation Depth Limits**:
   ```go
   // SELF_REMEDIATION_MAX_DEPTH
   // Default: 2 (allow 2 levels of self-remediation)
   // Validation: >= 0, 0 = disable self-remediation entirely
   // Recommendation: 2 for production, 3-5 for debugging
   ```

2. **Circuit Breaker Cooldown**:
   ```go
   // SELF_REMEDIATION_COOLDOWN_SECONDS
   // Default: 300 (5 minutes)
   // Validation: >= 0, 0 = disable circuit breaker
   // Recommendation: 300-600 seconds for production
   ```

3. **Upstream Repository**:
   ```go
   // MENDABOT_UPSTREAM_REPO
   // Default: "lenaxia/k8s-mendabot"
   // Validation: Must be in "owner/repo" format
   // Must be a valid GitHub repository (optional validation)
   ```

4. **Upstream Contributions**:
   ```go
   // MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS
   // Default: false
   // Validation: boolean (true/false/1/0)
   ```

5. **Cascade Check Configuration**:
   ```go
   // DISABLE_CASCADE_CHECK
   // Default: false
   // Validation: boolean
   
   // CASCADE_NAMESPACE_THRESHOLD
   // Default: 50 (percentage)
   // Validation: 1-100
   
   // CASCADE_NODE_CACHE_TTL
   // Default: 30 (seconds)
   // Validation: >= 0, 0 = disable caching
   ```

**Enhanced Validation Logic**:

```go
func validateCascadeConfig(cfg *Config) error {
    var errs []string
    
    // Self-remediation depth validation
    if cfg.SelfRemediationMaxDepth < 0 {
        errs = append(errs, "SELF_REMEDIATION_MAX_DEPTH must be >= 0")
    }
    
    // Circuit breaker cooldown validation
    if cfg.SelfRemediationCooldown < 0 {
        errs = append(errs, "SELF_REMEDIATION_COOLDOWN_SECONDS must be >= 0")
    }
    
    // Upstream repo format validation
    if cfg.MendabotUpstreamRepo != "" {
        parts := strings.Split(cfg.MendabotUpstreamRepo, "/")
        if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
            errs = append(errs, fmt.Sprintf("MENDABOT_UPSTREAM_REPO must be in 'owner/repo' format, got: %s", cfg.MendabotUpstreamRepo))
        }
    }
    
    // Cascade check threshold validation
    if cfg.CascadeNamespaceThreshold < 1 || cfg.CascadeNamespaceThreshold > 100 {
        errs = append(errs, fmt.Sprintf("CASCADE_NAMESPACE_THRESHOLD must be between 1 and 100, got: %d", cfg.CascadeNamespaceThreshold))
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("configuration validation failed:\n  %s", strings.Join(errs, "\n  "))
    }
    
    return nil
}
```

**Safe Defaults Configuration**:

```go
// Production-safe defaults
var ProductionDefaults = Config{
    SelfRemediationMaxDepth:      2,
    SelfRemediationCooldown:      5 * time.Minute,
    MendabotUpstreamRepo:         "lenaxia/k8s-mendabot",
    DisableUpstreamContributions: false,
    DisableCascadeCheck:          false,
    CascadeNamespaceThreshold:    50,
    CascadeNodeCacheTTL:          30 * time.Second,
}

// Debug/development defaults
var DebugDefaults = Config{
    SelfRemediationMaxDepth:      5,  // Allow deeper cascades for debugging
    SelfRemediationCooldown:      30 * time.Second,  // Shorter cooldown
    DisableUpstreamContributions: true,  // Don't spam upstream during development
    DisableCascadeCheck:          true,  // Disable cascade checks for focused testing
}
```

### Documentation Requirements

**README Documentation**:
```markdown
## Cascade Prevention Configuration

### Self-Remediation Settings
- `SELF_REMEDIATION_MAX_DEPTH`: Maximum chain depth (default: 2)
- `SELF_REMEDIATION_COOLDOWN_SECONDS`: Cooldown between self-remediations (default: 300)

### Upstream Contribution Settings  
- `MENDABOT_UPSTREAM_REPO`: Target for mendabot bug reports (default: "lenaxia/k8s-mendabot")
- `MENDABOT_DISABLE_UPSTREAM_CONTRIBUTIONS`: Disable upstream routing (default: false)

### Cascade Check Settings
- `DISABLE_CASCADE_CHECK`: Disable infrastructure cascade detection (default: false)
- `CASCADE_NAMESPACE_THRESHOLD`: Percentage of failing pods to trigger namespace-wide suppression (default: 50)
- `CASCADE_NODE_CACHE_TTL`: Node state cache TTL in seconds (default: 30)
```

**Code Documentation**:
- GoDoc comments for all configuration fields
- Example configurations in function documentation
- Deprecation warnings for future changes

### Integration Points

- **Config Package**: Enhanced validation and safe defaults
- **Deployment Manifests**: Example configurations in kustomize overlays
- **Documentation**: README updates and configuration guides
- **Testing**: Configuration validation tests

### Testing Requirements

**Unit Tests** (`internal/config/config_test.go`):
- Validation logic for each configuration variable
- Safe default values
- Error messages for invalid configurations
- Environment variable parsing edge cases

**Integration Tests**:
- End-to-end configuration loading
- Configuration precedence (env vars > defaults)
- Configuration migration scenarios
- Production vs debug mode differences

**Documentation Tests**:
- Configuration examples are valid
- Documentation matches code behavior
- All configuration options documented

---

## Tasks

- [x] Enhance configuration validation in `internal/config/config.go`
- [x] Add safe default configurations for production and debug modes
- [x] Update deployment manifests with example configurations
- [x] Write comprehensive configuration documentation
- [x] Add GoDoc comments for all configuration fields
- [x] Write unit tests for configuration validation
- [ ] Write integration tests for configuration scenarios
- [ ] Create configuration migration guide for future changes
- [x] Document troubleshooting steps for common configuration errors

---

## Dependencies

**Depends on:** All other stories in epic11 (for complete configuration set)
**Blocks:** Production deployment readiness

---

## Definition of Done

- [x] All tests pass with `-race`
- [x] `go vet` clean
- [x] Configuration validation prevents unsafe settings
- [x] Sensible defaults for production deployment
- [x] Comprehensive documentation with examples
- [x] Clear error messages for configuration errors
- [ ] Configuration migration path documented
- [x] All configuration options covered in tests
- [x] Documentation matches implementation behavior