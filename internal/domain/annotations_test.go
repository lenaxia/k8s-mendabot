package domain

import (
	"testing"
	"time"
)

func TestShouldSkip(t *testing.T) {
	anyTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		annotations map[string]string
		now         time.Time
		want        bool
	}{
		{
			name:        "SkipWhenDisabled",
			annotations: map[string]string{AnnotationEnabled: "false"},
			now:         anyTime,
			want:        true,
		},
		{
			name:        "SkipWhenSkipUntilInFuture",
			annotations: map[string]string{AnnotationSkipUntil: "2099-12-31"},
			now:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			want:        true,
		},
		{
			name:        "NoSkipWhenSkipUntilInPast",
			annotations: map[string]string{AnnotationSkipUntil: "2020-01-01"},
			now:         time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
			want:        false,
		},
		{
			name:        "NoSkipWhenSkipUntilMalformed",
			annotations: map[string]string{AnnotationSkipUntil: "not-a-date"},
			now:         anyTime,
			want:        false,
		},
		{
			name:        "NoSkipWhenNoAnnotations",
			annotations: map[string]string{},
			now:         anyTime,
			want:        false,
		},
		{
			name:        "NoSkipWhenNilAnnotations",
			annotations: nil,
			now:         anyTime,
			want:        false,
		},
		{
			name:        "SkipOnTheDateItself",
			annotations: map[string]string{AnnotationSkipUntil: "2025-06-01"},
			now:         time.Date(2025, 6, 1, 23, 59, 59, 0, time.UTC),
			want:        true,
		},
		{
			name:        "NoSkipDayAfter",
			annotations: map[string]string{AnnotationSkipUntil: "2025-06-01"},
			now:         time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldSkip(tt.annotations, tt.now)
			if got != tt.want {
				t.Errorf("ShouldSkip(%v, %v) = %v, want %v", tt.annotations, tt.now, got, tt.want)
			}
		})
	}
}
