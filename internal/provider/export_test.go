package provider

import "time"

// SetFirstSeenForTest sets a firstSeen entry with a specific timestamp for testing.
func (r *SourceProviderReconciler) SetFirstSeenForTest(fp string, t time.Time) {
	r.initFirstSeen()
	r.firstSeen.SetForTest(fp, t)
}

// FirstSeen returns a snapshot of the firstSeen map for assertions in tests.
func (r *SourceProviderReconciler) FirstSeen() map[string]time.Time {
	r.initFirstSeen()
	return r.firstSeen.Copy()
}
