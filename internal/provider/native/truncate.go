package native

// Truncation limits for cluster message fields.
//
// These are per-field limits chosen to match the actual information density of
// each field and the upstream constraints imposed by the Kubernetes API /
// kubelet, so the LLM receives as much diagnostic context as possible without
// unbounded growth.
const (
	// maxTerminatedMessage matches the kubelet's own hard cap on the
	// /dev/termination-log file (pkg/kubelet/container/runtime.go). Application
	// crash output (Go panics, Java stack traces) routinely fills this limit.
	maxTerminatedMessage = 4096

	// maxSchedulerMessage covers clusters up to ~50 nodes. The scheduler lists
	// every node's failure reason in PodCondition.Message (Unschedulable), which
	// grows ~30 bytes per node. 2048 comfortably covers most production clusters.
	maxSchedulerMessage = 2048

	// maxCSIEventMessage covers the full Kubernetes Event.Message limit (1024
	// bytes enforced by the API server). CSI provisioner errors often put the
	// root cause toward the end of the error chain; truncating at 500 discards it.
	maxCSIEventMessage = 2048

	// maxWaitingMessage covers ImagePullBackOff / ErrImagePull messages, which
	// can include registry URLs, auth error chains, and image digests. In practice
	// these rarely exceed 500 bytes but 1024 provides a safe margin.
	maxWaitingMessage = 1024

	// maxConditionMessage is used for condition messages where the upstream
	// controller sets a short fixed string (deployment, statefulset, job, node).
	// These never approach 500 bytes in practice; the limit is a safeguard only.
	maxConditionMessage = 500
)

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
