package metrics

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// circuitBreakerActivationsTotal counts circuit breaker trips
	circuitBreakerActivationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mendabot_circuit_breaker_activations_total",
			Help: "Total number of circuit breaker activations (trips)",
		},
		[]string{"provider", "namespace"},
	)

	// circuitBreakerCooldownSeconds tracks remaining cooldown time
	circuitBreakerCooldownSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mendabot_circuit_breaker_cooldown_seconds",
			Help: "Remaining cooldown time for circuit breaker in seconds",
		},
		[]string{"provider", "namespace"},
	)

	// chainDepthDistribution tracks distribution of chain depths
	chainDepthDistribution = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mendabot_chain_depth_distribution",
			Help:    "Distribution of cascade chain depths",
			Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		[]string{"provider", "namespace"},
	)

	// maxDepthExceededTotal counts max depth violations
	maxDepthExceededTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mendabot_max_depth_exceeded_total",
			Help: "Total number of times maximum chain depth was exceeded",
		},
		[]string{"provider", "namespace", "depth"},
	)

	// selfRemediationAttemptsTotal counts self-remediation attempts
	selfRemediationAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mendabot_self_remediation_attempts_total",
			Help: "Total number of self-remediation attempts",
		},
		[]string{"provider", "namespace", "success"},
	)

	// selfRemediationSuccessRate tracks success rate of self-remediations
	selfRemediationSuccessRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mendabot_self_remediation_success_rate",
			Help: "Success rate of self-remediation attempts (0.0 to 1.0)",
		},
		[]string{"provider", "namespace"},
	)

	// cascadeSuppressionsTotal counts cascade suppression events
	cascadeSuppressionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mendabot_cascade_suppressions_total",
			Help: "Total number of cascade suppression events",
		},
		[]string{"provider", "namespace", "suppression_type"},
	)

	// cascadeSuppressionReasons counts suppression reasons
	cascadeSuppressionReasons = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mendabot_cascade_suppression_reasons",
			Help: "Count of cascade suppression reasons",
		},
		[]string{"provider", "namespace", "reason"},
	)

	// successCounts and attemptCounts track self-remediation success rates
	successCounts   = make(map[string]map[string]int) // provider -> namespace -> count
	attemptCounts   = make(map[string]map[string]int) // provider -> namespace -> count
	successRateLock sync.RWMutex
)

func init() {
	// Register all metrics with controller-runtime metrics registry
	RegisterMetrics(metrics.Registry)
}

// RegisterMetrics registers all metrics with the given registry
func RegisterMetrics(registry prometheus.Registerer) error {
	metrics := []prometheus.Collector{
		circuitBreakerActivationsTotal,
		circuitBreakerCooldownSeconds,
		chainDepthDistribution,
		maxDepthExceededTotal,
		selfRemediationAttemptsTotal,
		selfRemediationSuccessRate,
		cascadeSuppressionsTotal,
		cascadeSuppressionReasons,
	}

	for _, metric := range metrics {
		if err := registry.Register(metric); err != nil {
			return err
		}
	}

	return nil
}

// ResetMetrics resets all metrics (for testing only)
func ResetMetrics() {
	circuitBreakerActivationsTotal.Reset()
	circuitBreakerCooldownSeconds.Reset()
	chainDepthDistribution.Reset()
	maxDepthExceededTotal.Reset()
	selfRemediationAttemptsTotal.Reset()
	selfRemediationSuccessRate.Reset()
	cascadeSuppressionsTotal.Reset()
	cascadeSuppressionReasons.Reset()

	successRateLock.Lock()
	defer successRateLock.Unlock()
	successCounts = make(map[string]map[string]int)
	attemptCounts = make(map[string]map[string]int)
}

// RecordCircuitBreakerActivation records a circuit breaker trip
func RecordCircuitBreakerActivation(provider, namespace string) {
	circuitBreakerActivationsTotal.WithLabelValues(provider, namespace).Inc()
}

// SetCircuitBreakerCooldown sets the remaining cooldown time
func SetCircuitBreakerCooldown(provider, namespace string, seconds float64) {
	circuitBreakerCooldownSeconds.WithLabelValues(provider, namespace).Set(seconds)
}

// ClearCircuitBreakerCooldown clears the cooldown timer
func ClearCircuitBreakerCooldown(provider, namespace string) {
	circuitBreakerCooldownSeconds.WithLabelValues(provider, namespace).Set(0)
}

// RecordChainDepth records a chain depth observation
func RecordChainDepth(provider, namespace string, depth int) {
	chainDepthDistribution.WithLabelValues(provider, namespace).Observe(float64(depth))
}

// RecordMaxDepthExceeded records a max depth violation
func RecordMaxDepthExceeded(provider, namespace string, depth int) {
	maxDepthExceededTotal.WithLabelValues(provider, namespace, strconv.Itoa(depth)).Inc()
}

// RecordSelfRemediationAttempt records a self-remediation attempt
func RecordSelfRemediationAttempt(provider, namespace string, success bool) {
	successStr := "false"
	if success {
		successStr = "true"
	}
	selfRemediationAttemptsTotal.WithLabelValues(provider, namespace, successStr).Inc()

	// Update internal counters for success rate calculation
	successRateLock.Lock()
	defer successRateLock.Unlock()

	if _, ok := attemptCounts[provider]; !ok {
		attemptCounts[provider] = make(map[string]int)
		successCounts[provider] = make(map[string]int)
	}

	attemptCounts[provider][namespace]++
	if success {
		successCounts[provider][namespace]++
	}
}

// UpdateSelfRemediationSuccessRate updates the success rate gauge
func UpdateSelfRemediationSuccessRate(provider, namespace string) {
	successRateLock.RLock()
	defer successRateLock.RUnlock()

	attempts := attemptCounts[provider][namespace]
	if attempts == 0 {
		selfRemediationSuccessRate.WithLabelValues(provider, namespace).Set(0)
		return
	}

	successes := successCounts[provider][namespace]
	rate := float64(successes) / float64(attempts)
	selfRemediationSuccessRate.WithLabelValues(provider, namespace).Set(rate)
}

// RecordCascadeSuppression records a cascade suppression event
func RecordCascadeSuppression(provider, namespace, suppressionType string) {
	cascadeSuppressionsTotal.WithLabelValues(provider, namespace, suppressionType).Inc()
}

// RecordCascadeSuppressionReason records a suppression reason
func RecordCascadeSuppressionReason(provider, namespace, reason, description string) {
	cascadeSuppressionReasons.WithLabelValues(provider, namespace, reason).Inc()
}

// SelfRemediationAttemptsTotal exposes the counter for test inspection.
func SelfRemediationAttemptsTotal() *prometheus.CounterVec {
	return selfRemediationAttemptsTotal
}

// MaxDepthExceededTotal exposes the counter for test inspection.
func MaxDepthExceededTotal() *prometheus.CounterVec {
	return maxDepthExceededTotal
}

// CascadeSuppressionsTotal exposes the counter for test inspection.
func CascadeSuppressionsTotal() *prometheus.CounterVec {
	return cascadeSuppressionsTotal
}
