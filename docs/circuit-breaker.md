# Persistent Circuit Breaker Implementation

## Overview

The circuit breaker prevents rapid cascades of self-remediations by enforcing a cooldown period between consecutive self-remediation attempts. The implementation is persistent (survives controller restarts), thread-safe, and shared across all provider reconcilers.

## Architecture

### Components

1. **`CircuitBreaker` struct** (`internal/circuitbreaker/circuitbreaker.go`):
   - Manages circuit breaker state with thread-safe operations
   - Persists state to Kubernetes ConfigMap
   - Provides `ShouldAllow()` method for cooldown checks

2. **ConfigMap-based persistence**:
   - Name: `mechanic-circuit-breaker`
   - Namespace: Same as agent namespace
   - Data:
     - `last-self-remediation`: RFC3339 timestamp of last self-remediation
     - `agent-namespace`: Namespace where ConfigMap resides

3. **Provider integration** (`internal/provider/provider.go`):
   - Each `SourceProviderReconciler` has a `circuitBreaker` field
   - Circuit breaker initialized on first self-remediation
   - All providers share same circuit breaker state via ConfigMap

## Configuration

### Environment Variables

- `SELF_REMEDIATION_COOLDOWN_SECONDS`: Cooldown period in seconds (default: 300, 5 minutes)
- Set to `0` to disable circuit breaker entirely

### Defaults

- Default cooldown: 5 minutes
- Configurable via environment variable
- Zero cooldown disables circuit breaker

## Thread Safety

- Uses `sync.RWMutex` for concurrent access protection
- Safe for multiple reconciler goroutines
- Controller-runtime defaults to single worker per controller, but circuit breaker handles concurrent access anyway

## Persistence

- State stored in ConfigMap `mechanic-circuit-breaker`
- Survives controller pod restarts
- Shared across multiple controller instances (if scaled)
- ConfigMap created on first self-remediation if not exists

## Usage in Provider Reconciler

```go
if finding.IsSelfRemediation {
    // Initialize circuit breaker if not already initialized
    if r.circuitBreaker == nil {
        r.circuitBreaker = circuitbreaker.New(r.Client, r.Cfg.AgentNamespace, r.Cfg.SelfRemediationCooldown)
    }
    
    allowed, remaining, err := r.circuitBreaker.ShouldAllow(ctx)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("circuit breaker error: %w", err)
    }
    
    if !allowed {
        r.Log.Info("circuit breaker: skipping self-remediation due to cooldown",
            zap.String("fingerprint", fp[:12]),
            zap.Duration("remaining", remaining),
            zap.Int("chainDepth", finding.ChainDepth),
        )
        return ctrl.Result{RequeueAfter: remaining}, nil
    }
    
    // Proceed with self-remediation...
}
```

## Testing

### Unit Tests

- `internal/circuitbreaker/circuitbreaker_test.go`: Tests circuit breaker logic
- `internal/provider/circuitbreaker_test.go`: Tests provider integration
- `internal/provider/circuitbreaker_integration_test.go`: Integration tests for persistence and multi-provider scenarios

### Test Coverage

1. **Basic functionality**: Cooldown enforcement, zero cooldown (disabled)
2. **Persistence**: State survives across reconciler instances (simulates controller restart)
3. **Multi-provider**: All provider reconcilers share same circuit breaker state
4. **Configurability**: Cooldown period configurable via environment variable

## Failure Modes

### ConfigMap Operations Fail

- `ShouldAllow()` returns error if ConfigMap operations fail
- Provider reconciler returns error, triggering retry
- In-memory state preserved for current instance

### Zero Cooldown

- Circuit breaker disabled when cooldown = 0
- `ShouldAllow()` always returns `true`
- Useful for testing or disabling circuit breaker

### Invalid Timestamp in ConfigMap

- If ConfigMap contains invalid timestamp, treated as zero time (never remediated)
- Logs error but continues operation
- Next successful self-remediation overwrites invalid timestamp

## Monitoring

- Logs when circuit breaker blocks self-remediation
- Logs when ConfigMap is created
- Logs deep cascade warnings (chain depth > 2)
- ConfigMap can be inspected for last self-remediation time

## Manual Intervention

### Reset Circuit Breaker

```bash
# Delete ConfigMap to reset circuit breaker
kubectl delete configmap mechanic-circuit-breaker -n <agent-namespace>
```

### Inspect State

```bash
# Check last self-remediation time
kubectl get configmap mechanic-circuit-breaker -n <agent-namespace> -o yaml
```

## Design Decisions

### Why ConfigMap instead of...

1. **In-memory only**: Wouldn't survive controller restarts
2. **Custom Resource**: Overkill for simple timestamp storage
3. **RemediationJob annotation**: Wouldn't work across different findings
4. **Leader election with shared state**: More complex, ConfigMap is simpler

### Why Shared Circuit Breaker

- Prevents cascades across all provider types
- Single cooldown period for entire system
- Simpler than per-provider circuit breakers

### Why 5-minute Default

- Balances between preventing cascades and allowing legitimate remediations
- Configurable for different environments
- Based on typical remediation job runtime