# Story: Monitoring and Observability for Cascade Events

**Epic:** [epic11-self-remediation-cascade](README.md)
**Priority:** Medium
**Status:** In Progress
**Estimated Effort:** 4 hours

---

## User Story

As a **mendabot operator**, I want comprehensive monitoring and observability for cascade prevention events, so that I can track system health, detect anomalies, and understand cascade prevention effectiveness.

---

## Acceptance Criteria

- [x] Prometheus metrics for cascade prevention events
- [x] Structured audit logs for cascade decisions
- [ ] Kubernetes Events for significant cascade events
- [x] Circuit breaker state exposed as metrics
- [x] Chain depth distribution metrics
- [x] Self-remediation success/failure rates
- [x] Suppression reason tracking (circuit breaker, max depth, infrastructure cascade)
- [x] Integration with existing controller-runtime metrics
- [ ] Grafana dashboard template for cascade monitoring
- [ ] Alert rules for cascade anomalies (e.g., deep cascades, circuit breaker stuck open)
- [x] Unit tests for metrics collection
- [ ] Integration tests for observability features

---

## Technical Implementation

### Location: `internal/metrics/` (new package) and updates to existing packages

**Metrics Design:**

1. **Circuit Breaker Metrics**:
   ```go
   mendabot_circuit_breaker_state{namespace="<agent-namespace>"} 1 // 1 = open, 0 = closed
   mendabot_circuit_breaker_last_self_remediation_timestamp{namespace="<agent-namespace>"}
   mendabot_circuit_breaker_cooldown_seconds{namespace="<agent-namespace>"}
   ```

2. **Chain Depth Metrics**:
   ```go
   mendabot_chain_depth_distribution{namespace="<agent-namespace>", depth="0|1|2|3+"}
   mendabot_self_remediation_depth_total{namespace="<agent-namespace>", depth="<n>"}
   ```

3. **Cascade Prevention Metrics**:
   ```go
   mendabot_findings_suppressed_total{namespace="<agent-namespace>", reason="circuit_breaker|max_depth|infrastructure_cascade|node_failure|node_pressure|namespace_wide"}
   mendabot_cascade_checks_total{namespace="<agent-namespace>", result="suppressed|allowed"}
   mendabot_infrastructure_cascade_detected_total{namespace="<agent-namespace>", type="node_failure|node_pressure|namespace_wide"}
   ```

4. **Self-Remediation Metrics**:
   ```go
   mendabot_self_remediation_total{namespace="<agent-namespace>", outcome="success|failure|suppressed"}
   mendabot_upstream_contributions_total{namespace="<agent-namespace>", repo="<target-repo>"}
   ```

### Audit Logging

**Structured Log Events** (zap fields):
```go
// Circuit breaker events
logger.Info("circuit breaker: skipping self-remediation due to cooldown",
    zap.String("fingerprint", fp[:12]),
    zap.Duration("remaining", remaining),
    zap.Int("chainDepth", finding.ChainDepth),
    zap.Time("lastSelfRemediation", lastTime),
)

// Cascade suppression events
logger.Info("cascade check: suppressing finding",
    zap.String("reason", reason),
    zap.String("kind", finding.Kind),
    zap.String("namespace", finding.Namespace),
    zap.String("parentObject", finding.ParentObject),
    zap.String("infrastructureResource", infraResource), // e.g., node name
)

// Deep cascade warnings
logger.Warn("deep cascade detected in self-remediation",
    zap.String("fingerprint", fp[:12]),
    zap.Int("chainDepth", finding.ChainDepth),
    zap.String("findingName", finding.Name),
)
```

### Kubernetes Events

**Event Types to Emit**:
- `CircuitBreakerOpened`: When circuit breaker prevents self-remediation
- `DeepCascadeDetected`: When chain depth > 2
- `InfrastructureCascadeSuppressed`: When pod finding suppressed due to node failure
- `UpstreamContributionRouted`: When self-remediation routed to upstream repo

### Integration Points

- **CircuitBreaker**: Expose state metrics and emit events
- **SourceProviderReconciler**: Log cascade decisions and emit events
- **JobProvider**: Track chain depth distribution
- **CascadeChecker**: Track suppression reasons and infrastructure correlations
- **Config**: Metrics namespace and label configuration

### Configuration

**Environment Variables**:
```bash
# Metrics configuration
METRICS_ENABLED=true
METRICS_NAMESPACE="mendabot"
METRICS_PORT="8080"

# Logging configuration
LOG_LEVEL="info"
LOG_FORMAT="json"  # or "console"
AUDIT_LOG_ENABLED=true

# Events configuration
EVENTS_ENABLED=true
```

### Testing Requirements

**Unit Tests** (`internal/metrics/metrics_test.go`):
- Metrics registration and collection
- Label correctness
- Counter increment logic
- Gauge value updates

**Integration Tests** (`internal/metrics/integration_test.go`):
- Metrics endpoint availability
- Metric values under different scenarios
- Audit log format and content
- Event emission correctness

**Performance Tests**:
- Metrics collection overhead
- Log volume impact
- Memory usage with high event rates

---

## Tasks

- [x] Design metrics schema and labels
- [x] Implement metrics collection package
- [x] Add metrics to CircuitBreaker
- [x] Add metrics to SourceProviderReconciler
- [x] Add metrics to CascadeChecker
- [x] Implement structured audit logging
- [ ] Add Kubernetes Events emission
- [ ] Create Grafana dashboard template
- [ ] Write Prometheus alert rules
- [x] Write unit tests for metrics
- [ ] Write integration tests for observability
- [ ] Document monitoring setup and dashboards

---

## Dependencies

**Depends on:** STORY_03_circuit_breaker, STORY_05_infrastructure_cascade
**Blocks:** Operational readiness for production deployment

---

## Definition of Done

- [x] All tests pass with `-race`
- [x] `go vet` clean
- [x] Metrics available on `/metrics` endpoint
- [x] Audit logs contain structured cascade events
- [ ] Kubernetes Events emitted for significant cascade events
- [ ] Grafana dashboard template provided
- [ ] Alert rules documented
- [ ] Performance impact measured and acceptable
- [ ] Documentation covers monitoring setup and interpretation