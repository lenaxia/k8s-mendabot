package native

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// Compile-time assertion: podProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*podProvider)(nil)

// waitingReasons is the set of container waiting reasons that produce a Finding.
var waitingReasons = map[string]struct{}{
	"CrashLoopBackOff":           {},
	"ImagePullBackOff":           {},
	"ErrImagePull":               {},
	"CreateContainerConfigError": {},
	"InvalidImageName":           {},
	"RunContainerError":          {},
	"CreateContainerError":       {},
}

type podProvider struct {
	client client.Client
}

// NewPodProvider constructs a podProvider. Panics if c is nil.
func NewPodProvider(c client.Client) domain.SourceProvider {
	if c == nil {
		panic("NewPodProvider: client must not be nil")
	}
	return &podProvider{client: c}
}

// ProviderName returns the stable identifier for this provider.
func (p *podProvider) ProviderName() string { return "native" }

// ObjectType returns the runtime.Object type this provider watches.
func (p *podProvider) ObjectType() client.Object { return &corev1.Pod{} }

// ExtractFinding converts a watched Pod into a Finding.
// Returns (nil, nil) if the pod is healthy (no failure conditions detected).
// Returns (nil, err) if obj is not a *corev1.Pod.
func (p *podProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	if domain.ShouldSkip(obj.GetAnnotations(), time.Now()) {
		return nil, nil
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("podProvider: expected *corev1.Pod, got %T", obj)
	}

	type errorEntry struct {
		Text string `json:"text"`
	}

	var errors []errorEntry

	// Check container statuses and init container statuses for failures.
	allStatuses := make([]corev1.ContainerStatus, 0, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	allStatuses = append(allStatuses, pod.Status.ContainerStatuses...)
	allStatuses = append(allStatuses, pod.Status.InitContainerStatuses...)
	for _, cs := range allStatuses {
		// Waiting state: check for known failure reasons.
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if _, isFailure := waitingReasons[reason]; isFailure {
				if reason == "CrashLoopBackOff" {
					text := buildCrashLoopText(cs)
					errors = append(errors, errorEntry{Text: text})
				} else {
					text := buildWaitingText(cs)
					errors = append(errors, errorEntry{Text: text})
				}
			}
			// Container is in a waiting state — do not also check terminated state below.
			continue
		}

		// Terminated state (not being restarted — Waiting is nil): non-zero exit code.
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			msg := cs.State.Terminated.Message
			if msg != "" {
				msg = ": " + truncate(domain.StripDelimiters(domain.RedactSecrets(msg)), maxTerminatedMessage)
			}
			text := fmt.Sprintf("container %s: terminated with exit code %d%s",
				cs.Name, cs.State.Terminated.ExitCode, msg)
			errors = append(errors, errorEntry{Text: text})
		}
	}

	// Unschedulable pending pod.
	if pod.Status.Phase == corev1.PodPending {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled &&
				cond.Status == corev1.ConditionFalse &&
				cond.Reason == "Unschedulable" {
				text := fmt.Sprintf("pod %s: %s", cond.Reason, truncate(domain.StripDelimiters(domain.RedactSecrets(cond.Message)), maxSchedulerMessage))
				errors = append(errors, errorEntry{Text: text})
				break
			}
		}
	}

	if len(errors) == 0 {
		return nil, nil
	}

	errorsJSON, err := json.Marshal(errors)
	if err != nil {
		return nil, fmt.Errorf("podProvider: serialising errors: %w", err)
	}

	parent := getParent(context.Background(), p.client, pod.ObjectMeta, "Pod")

	return &domain.Finding{
		Kind:         "Pod",
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		ParentObject: parent,
		Errors:       string(errorsJSON),
		Severity:     computePodSeverity(pod),
	}, nil
}

// computePodSeverity determines the highest severity across all container failure conditions.
func computePodSeverity(pod *corev1.Pod) domain.Severity {
	current := domain.SeverityMedium

	allStatuses := make([]corev1.ContainerStatus, 0, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	allStatuses = append(allStatuses, pod.Status.ContainerStatuses...)
	allStatuses = append(allStatuses, pod.Status.InitContainerStatuses...)
	for _, cs := range allStatuses {
		if cs.State.Waiting != nil {
			switch cs.State.Waiting.Reason {
			case "CrashLoopBackOff":
				if cs.RestartCount > 5 {
					return domain.SeverityCritical
				}
				if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					if domain.SeverityLevel(domain.SeverityHigh) > domain.SeverityLevel(current) {
						current = domain.SeverityHigh
					}
					continue
				}
				if domain.SeverityLevel(domain.SeverityHigh) > domain.SeverityLevel(current) {
					current = domain.SeverityHigh
				}
			case "ImagePullBackOff", "ErrImagePull":
				if domain.SeverityLevel(domain.SeverityHigh) > domain.SeverityLevel(current) {
					current = domain.SeverityHigh
				}
			}
			continue
		}

		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			if cs.State.Terminated.Reason == "OOMKilled" {
				if domain.SeverityLevel(domain.SeverityHigh) > domain.SeverityLevel(current) {
					current = domain.SeverityHigh
				}
			}
		}
	}

	// Unschedulable condition → high.
	if pod.Status.Phase == corev1.PodPending {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled &&
				cond.Status == corev1.ConditionFalse &&
				cond.Reason == "Unschedulable" {
				if domain.SeverityLevel(domain.SeverityHigh) > domain.SeverityLevel(current) {
					current = domain.SeverityHigh
				}
			}
		}
	}

	return current
}

// buildCrashLoopText constructs the error message for a CrashLoopBackOff container.
// It includes the last termination reason (e.g. OOMKilled) if available.
func buildCrashLoopText(cs corev1.ContainerStatus) string {
	lastReason := ""
	if cs.LastTerminationState.Terminated != nil {
		lastReason = cs.LastTerminationState.Terminated.Reason
	}
	if lastReason != "" {
		return fmt.Sprintf("container %s: CrashLoopBackOff (last exit: %s)", cs.Name, lastReason)
	}
	return fmt.Sprintf("container %s: CrashLoopBackOff", cs.Name)
}

// buildWaitingText constructs the error message for a container in a non-CrashLoopBackOff
// waiting failure state, including the Waiting.Message when non-empty.
func buildWaitingText(cs corev1.ContainerStatus) string {
	reason := cs.State.Waiting.Reason
	msg := cs.State.Waiting.Message
	if msg != "" {
		msg = truncate(domain.StripDelimiters(domain.RedactSecrets(msg)), maxWaitingMessage)
		return fmt.Sprintf("container %s: %s: %s", cs.Name, reason, msg)
	}
	return fmt.Sprintf("container %s: %s", cs.Name, reason)
}
