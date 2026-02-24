package testutil

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
)

// RecordedEvent captures a single Kubernetes Event emission including the target object.
type RecordedEvent struct {
	Object  runtime.Object
	EType   string
	Reason  string
	Message string
}

// ObjectRecorder is a test-only record.EventRecorder that captures the target object
// alongside the event type, reason, and message. This allows tests to assert not just
// that an event was emitted, but which object it was emitted on.
type ObjectRecorder struct {
	mu     sync.Mutex
	Events []RecordedEvent
}

// Event implements record.EventRecorder.
func (r *ObjectRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Events = append(r.Events, RecordedEvent{
		Object:  object,
		EType:   eventtype,
		Reason:  reason,
		Message: message,
	})
}

// Eventf implements record.EventRecorder.
func (r *ObjectRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

// AnnotatedEventf implements record.EventRecorder.
func (r *ObjectRecorder) AnnotatedEventf(object runtime.Object, _ map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

// FindByReason returns all events with the given reason.
func (r *ObjectRecorder) FindByReason(reason string) []RecordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []RecordedEvent
	for _, e := range r.Events {
		if e.Reason == reason {
			out = append(out, e)
		}
	}
	return out
}
