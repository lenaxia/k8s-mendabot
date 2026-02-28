# Cascade Prevention Metrics

This document describes the Prometheus metrics exposed by the Mechanic cascade prevention system.

## Overview

The cascade prevention metrics provide operational visibility into:
- Circuit breaker activations and cooldowns
- Chain depth distribution for self-remediations
- Self-remediation success rates
- Cascade suppression events and reasons

## Available Metrics

### Circuit Breaker Metrics

#### `mechanic_circuit_breaker_activations_total`
- **Type**: Counter
- **Labels**: `provider`, `namespace`
- **Description**: Total number of circuit breaker activations (trips)
- **When incremented**: When a self-remediation is blocked due to circuit breaker cooldown

#### `mechanic_circuit_breaker_cooldown_seconds`
- **Type**: Gauge
- **Labels**: `provider`, `namespace`
- **Description**: Remaining cooldown time for circuit breaker in seconds
- **Values**: 0 when no cooldown, positive when cooldown active

### Chain Depth Metrics

#### `mechanic_chain_depth_distribution`
- **Type**: Histogram
- **Labels**: `provider`, `namespace`
- **Description**: Distribution of cascade chain depths
- **Buckets**: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
- **When observed**: When a self-remediation with chain depth > 2 is processed

#### `mechanic_max_depth_exceeded_total`
- **Type**: Counter
- **Labels**: `provider`, `namespace`, `depth`
- **Description**: Total number of times maximum chain depth was exceeded
- **When incremented**: When chain depth > 3 (configurable threshold)

### Self-Remediation Metrics

#### `mechanic_self_remediation_attempts_total`
- **Type**: Counter
- **Labels**: `provider`, `namespace`, `success`
- **Description**: Total number of self-remediation attempts
- **Success values**: `true` for successful attempts, `false` for failed attempts

#### `mechanic_self_remediation_success_rate`
- **Type**: Gauge
- **Labels**: `provider`, `namespace`
- **Description**: Success rate of self-remediation attempts (0.0 to 1.0)
- **When updated**: After each self-remediation attempt completion

### Cascade Suppression Metrics

#### `mechanic_cascade_suppressions_total`
- **Type**: Counter
- **Labels**: `provider`, `namespace`, `suppression_type`
- **Description**: Total number of cascade suppression events
- **Suppression types**: `circuit_breaker`, `max_depth`, `stabilisation_window`

#### `mechanic_cascade_suppression_reasons`
- **Type**: Counter
- **Labels**: `provider`, `namespace`, `reason`
- **Description**: Count of cascade suppression reasons
- **Reason examples**: `cooldown_active`, `chain_too_deep`, `window_active`

## Example Prometheus Queries

### Circuit Breaker Monitoring
```promql
# Circuit breaker activation rate (per minute)
rate(mechanic_circuit_breaker_activations_total[5m])

# Active circuit breaker cooldowns
mechanic_circuit_breaker_cooldown_seconds > 0

# Circuit breaker activations by provider
sum by (provider) (rate(mechanic_circuit_breaker_activations_total[5m]))
```

### Chain Depth Analysis
```promql
# Average chain depth
avg(mechanic_chain_depth_distribution_bucket)

# Chain depth distribution percentiles
histogram_quantile(0.95, sum(rate(mechanic_chain_depth_distribution_bucket[5m])) by (le))

# Max depth violations
sum(rate(mechanic_max_depth_exceeded_total[5m])) by (depth)
```

### Self-Remediation Success Tracking
```promql
# Current success rate by provider
mechanic_self_remediation_success_rate

# Success rate trend (30-minute moving average)
avg_over_time(mechanic_self_remediation_success_rate[30m])

# Total attempts by outcome
sum by (success) (mechanic_self_remediation_attempts_total)
```

### Cascade Suppression Analysis
```promql
# Suppression rate by type
sum by (suppression_type) (rate(mechanic_cascade_suppressions_total[5m]))

# Top suppression reasons
topk(5, sum by (reason) (rate(mechanic_cascade_suppression_reasons[5m])))

# Suppression ratio (suppressions / total self-remediation attempts)
sum(rate(mechanic_cascade_suppressions_total[5m])) / 
sum(rate(mechanic_self_remediation_attempts_total[5m]))
```

## Alerting Examples

### High Circuit Breaker Activation Rate
```yaml
alert: HighCircuitBreakerActivationRate
expr: rate(mechanic_circuit_breaker_activations_total[5m]) > 0.1
for: 5m
labels:
  severity: warning
annotations:
  summary: "High circuit breaker activation rate"
  description: "Circuit breaker is activating frequently, indicating potential cascade issues"
```

### Low Self-Remediation Success Rate
```yaml
alert: LowSelfRemediationSuccessRate
expr: mechanic_self_remediation_success_rate < 0.5
for: 10m
labels:
  severity: critical
annotations:
  summary: "Low self-remediation success rate"
  description: "Self-remediation success rate is below 50%"
```

### Deep Cascade Chains
```yaml
alert: DeepCascadeChains
expr: histogram_quantile(0.95, sum(rate(mechanic_chain_depth_distribution_bucket[5m])) by (le)) > 5
for: 5m
labels:
  severity: warning
annotations:
  summary: "Deep cascade chains detected"
  description: "95th percentile of chain depth exceeds 5"
```

## Implementation Details

### Thread Safety
All metrics updates are thread-safe using:
- Prometheus atomic operations for counters and gauges
- `sync.RWMutex` for internal success rate calculations
- Controller-runtime's single-worker guarantee per reconciler

### Performance Considerations
- Metrics use vectorized operations (CounterVec, GaugeVec, etc.)
- Label cardinality is limited to prevent metric explosion
- Histogram buckets are optimized for typical chain depths (1-10)

### Integration Points
1. **Provider Reconciler** (`internal/provider/provider.go`):
   - Records circuit breaker activations and cooldowns
   - Tracks chain depth distribution
   - Records cascade suppression events

2. **RemediationJob Controller** (`internal/controller/remediationjob_controller.go`):
   - Records self-remediation success/failure
   - Updates success rate gauges

3. **Metrics Package** (`internal/metrics/metrics.go`):
   - Centralized metric definitions
   - Thread-safe operations
   - Registry integration