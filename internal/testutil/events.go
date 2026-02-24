package testutil

import "k8s.io/client-go/tools/record"

// DrainEvents non-blockingly drains all buffered events from a fake recorder
// channel and returns them as a slice of strings. Each string has the format
// "<Type> <Reason> <message>" as emitted by record.FakeRecorder.
func DrainEvents(rec *record.FakeRecorder) []string {
	var out []string
	for {
		select {
		case e := <-rec.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}
