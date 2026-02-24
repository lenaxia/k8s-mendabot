# Epic 11: Self-Remediation Cascade Prevention

## Purpose

Implement a comprehensive system to prevent infinite cascades where mendabot analyzes its own failures. This includes detection, chain depth tracking, circuit breaking, and upstream contribution routing for mendabot bugs.

## Status: Deferred — moved to `feature/epic11-13-deferred`

## Dependencies

- epic01-controller complete (SourceProviderReconciler)
- epic02-jobbuilder complete (JobBuilder)
- epic04-deploy complete (manifests for agent namespace)

## Blocks

- Future reliability improvements (FT-R series)
- Enhanced monitoring and observability (FT-U series)

## Success Criteria

- [x] Self-remediation detection identifies mendabot job failures
- [x] Chain depth tracking prevents infinite recursion (max depth configurable)
- [x] Circuit breaker with persistent state prevents rapid cascades
 - [ ] Upstream contribution routing for mendabot bugs (depth ≥ 2) - REMOVED
- [ ] Infrastructure failure cascade prevention (node/pod correlation) - STORY_05
- [x] Thread-safe implementation with concurrent reconciliation support
- [x] Configurable cooldown periods and depth limits
- [ ] Comprehensive test coverage including integration tests - STORY_06
- [ ] Configuration validation and documentation - STORY_07

## Stories

| Story | File | Status | Priority | Effort |
|-------|------|--------|----------|--------|
 | Self-remediation detection in JobProvider | [STORY_01_self_remediation_detection.md](STORY_01_self_remediation_detection.md) | Complete | Critical | 2h |
 | Chain depth tracking and max depth enforcement | [STORY_02_chain_depth_tracking.md](STORY_02_chain_depth_tracking.md) | Complete | Critical | 3h |
 | Persistent circuit breaker with ConfigMap state | [STORY_03_circuit_breaker.md](STORY_03_circuit_breaker.md) | Complete | Critical | 4h |
 | Infrastructure failure cascade prevention | [STORY_05_infrastructure_cascade.md](STORY_05_infrastructure_cascade.md) | Not Started | Medium | 6h |
 | Monitoring and observability for cascade events | [STORY_06_monitoring.md](STORY_06_monitoring.md) | Not Started | Medium | 4h |
 | Configuration validation and documentation | [STORY_07_configuration.md](STORY_07_configuration.md) | Not Started | Medium | 3h |

## Technical Overview

The self-remediation cascade prevention system consists of several integrated components:

### 1. Self-Remediation Detection (`internal/provider/native/job.go`)
- Detects mendabot agent jobs via label `app.kubernetes.io/managed-by: mendabot-watcher`
- Marks findings with `IsSelfRemediation: true`
- Reads chain depth from owner RemediationJob for atomic updates

### 2. Chain Depth Tracking
- `ChainDepth` field in `domain.Finding` and `RemediationJobSpec`
- Incremented on each self-remediation level
- Max depth enforced via `SELF_REMEDIATION_MAX_DEPTH` config
- Prevents infinite recursion by stopping at configurable limit

### 3. Circuit Breaker (`internal/circuitbreaker/`)
- Persistent state stored in ConfigMap `mendabot-circuit-breaker`
- Thread-safe operations with mutex protection
- Configurable cooldown via `SELF_REMEDIATION_COOLDOWN_SECONDS`
- Prevents rapid cascades of self-remediations

 ### 4. Self-Remediation Cascade Prevention
 - Self-remediations at depth > 2 trigger circuit breaker
 - All PRs created against user's configured GitOps repository
 - No upstream routing to avoid GitHub App permission complexity
 - Focus on preventing infinite cascades rather than bug reporting

### 5. Integration Points
- **SourceProviderReconciler**: Applies circuit breaker to self-remediations
- **JobProvider**: Detects mendabot jobs and computes chain depth
- **JobBuilder**: Injects chain depth and self-remediation flags into agent jobs
- **Config**: Environment variables for all tunable parameters

## Definition of Done

- [x] All unit tests pass with race detector
- [x] Integration tests simulate cascade scenarios
- [x] Circuit breaker persistence survives controller restarts
- [x] Concurrent reconciliation handled correctly
- [x] Configuration documented in README and code
- [x] No data races in concurrent access patterns
- [x] Backward compatibility maintained for existing annotations

## Implementation Notes

 ### Already Implemented
1. **Self-remediation detection**: Complete in `job.go:76-97`
2. **Chain depth tracking**: Complete with atomic owner reference reading
3. **Circuit breaker**: Complete with ConfigMap persistence
4. **Max depth enforcement**: Complete via config validation

### Remaining Work
1. **Infrastructure cascade prevention**: Detect node/pod correlation failures
2. **Enhanced monitoring**: Metrics and events for cascade detection
3. **Configuration validation**: Additional safety checks and defaults

 ### Configuration Reference
```bash
# Required for cascade prevention
SELF_REMEDIATION_MAX_DEPTH=2                    # Maximum chain depth (0 = disable)
SELF_REMEDIATION_COOLDOWN_SECONDS=300           # 5 minutes between self-remediations
```

### Operational Considerations
- Circuit breaker state survives controller restarts via ConfigMap
- Chain depth is atomic via RemediationJob owner references
- Self-remediations are logged with warning for depth > 2
- All configurations have safe defaults for production use