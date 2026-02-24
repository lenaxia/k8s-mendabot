package domain

import "testing"

func TestSeverityLevel(t *testing.T) {
	tests := []struct {
		name  string
		input Severity
		want  int
	}{
		{"SeverityLevelCritical", SeverityCritical, 4},
		{"SeverityLevelHigh", SeverityHigh, 3},
		{"SeverityLevelMedium", SeverityMedium, 2},
		{"SeverityLevelLow", SeverityLow, 1},
		{"SeverityLevelUnknown", Severity("bogus"), 0},
		{"SeverityLevelEmpty", Severity(""), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SeverityLevel(tt.input)
			if got != tt.want {
				t.Errorf("SeverityLevel(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestMeetsSeverityThreshold(t *testing.T) {
	tests := []struct {
		name string
		f    Severity
		min  Severity
		want bool
	}{
		{"MeetsThreshold_CriticalMeetsCritical", SeverityCritical, SeverityCritical, true},
		{"MeetsThreshold_CriticalMeetsHigh", SeverityCritical, SeverityHigh, true},
		{"MeetsThreshold_LowDoesNotMeetHigh", SeverityLow, SeverityHigh, false},
		{"MeetsThreshold_EmptyDoesNotMeetHigh", Severity(""), SeverityHigh, false},
		{"MeetsThreshold_EmptyMeetsLow", Severity(""), SeverityLow, true},
		{"MeetsThreshold_LowMeetsLow", SeverityLow, SeverityLow, true},
		{"MeetsThreshold_UnknownMinFailsClosed", SeverityCritical, Severity("bogus"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MeetsSeverityThreshold(tt.f, tt.min)
			if got != tt.want {
				t.Errorf("MeetsSeverityThreshold(%q, %q) = %v, want %v", tt.f, tt.min, got, tt.want)
			}
		})
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue Severity
		wantOK    bool
	}{
		{"ParseSeverityValid", "critical", SeverityCritical, true},
		{"ParseSeverityHigh", "high", SeverityHigh, true},
		{"ParseSeverityMedium", "medium", SeverityMedium, true},
		{"ParseSeverityLow", "low", SeverityLow, true},
		{"ParseSeverityUnknown", "urgent", SeverityLow, false},
		{"ParseSeverityEmpty", "", SeverityLow, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseSeverity(tt.input)
			if got != tt.wantValue || ok != tt.wantOK {
				t.Errorf("ParseSeverity(%q) = (%q, %v), want (%q, %v)", tt.input, got, ok, tt.wantValue, tt.wantOK)
			}
		})
	}
}
