package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// failWriter is an io.Writer that always returns an error, used to test the
// write-failure path in run().
type failWriter struct{ err error }

func (f failWriter) Write(_ []byte) (int, error) { return 0, f.err }

func TestRun(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantContains    []string
		wantNotContains []string
		// wantExact is checked only when wantExactSet is true, so that
		// wantExact: "" on the "Empty input" case is actually asserted
		// rather than silently skipped by a != "" guard.
		wantExact    string
		wantExactSet bool
	}{
		{
			name:            "password in kubectl output",
			input:           "password: hunter2",
			wantContains:    []string{"[REDACTED]"},
			wantNotContains: []string{"hunter2"},
		},
		{
			name:            "Bearer token in header",
			input:           "Authorization: Bearer ghp_abc123AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantContains:    []string{"[REDACTED]"},
			wantNotContains: []string{"ghp_"},
		},
		{
			name:         "GH token standalone",
			input:        "ghs_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantExact:    "[REDACTED-GH-TOKEN]",
			wantExactSet: true,
		},
		{
			name:         "GH actions token standalone",
			input:        "gha_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantExact:    "[REDACTED-GH-TOKEN]",
			wantExactSet: true,
		},
		{
			name:         "PEM key block multi-line",
			input:        "-----BEGIN RSA PRIVATE KEY-----\nMIIEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n-----END RSA PRIVATE KEY-----",
			wantExact:    "[REDACTED-PEM-KEY]",
			wantExactSet: true,
		},
		{
			name:         "Base64 value 40 chars boundary",
			input:        "data: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantContains: []string{"[REDACTED-BASE64]"},
		},
		{
			name:         "Base64 secret value mixed charset",
			input:        "data: dGhpc2lzYXNlY3JldHZhbHVlMTIzNDU2Nzg5MGFiY2Q=",
			wantContains: []string{"[REDACTED-BASE64]"},
		},
		{
			name:         "Base64 value 39 chars not redacted",
			input:        "data: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantExact:    "data: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantExactSet: true,
		},
		{
			name:         "No secrets passthrough",
			input:        "pod app: CrashLoopBackOff",
			wantExact:    "pod app: CrashLoopBackOff",
			wantExactSet: true,
		},
		{
			name:         "Empty input",
			input:        "",
			wantExact:    "",
			wantExactSet: true,
		},
		{
			name:            "Large input over 50KB",
			input:           strings.Repeat("CrashLoopBackOff\n", 3500),
			wantExact:       strings.Repeat("CrashLoopBackOff\n", 3500),
			wantExactSet:    true,
			wantNotContains: []string{"[REDACTED]"},
		},
		{
			name:            "Multiple patterns same line",
			input:           "password=secret token=mytoken",
			wantContains:    []string{"password=[REDACTED]", "token=[REDACTED]"},
			wantNotContains: []string{"secret", "mytoken"},
		},
		{
			name:            "URL credentials",
			input:           "postgres://user:pass@db:5432/mydb",
			wantContains:    []string{"postgres://[REDACTED]@db:5432/mydb"},
			wantNotContains: []string{"user:pass"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			var w bytes.Buffer

			err := run(r, &w)
			if err != nil {
				t.Fatalf("run() returned unexpected error: %v", err)
			}

			got := w.String()

			if tt.wantExactSet {
				if got != tt.wantExact {
					t.Errorf("run() output = %q, want exact %q", got, tt.wantExact)
				}
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("run() output = %q, want it to contain %q", got, want)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("run() output = %q, want it NOT to contain %q", got, notWant)
				}
			}
		})
	}
}

func TestRunWriteFailure(t *testing.T) {
	// Verify that a write error (e.g. broken pipe) is propagated by run().
	sentinel := errors.New("write error: broken pipe")
	err := run(strings.NewReader("password=hunter2"), failWriter{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Errorf("run() error = %v, want %v", err, sentinel)
	}
}

func TestRunExtraPatterns(t *testing.T) {
	t.Run("EXTRA_REDACT_PATTERNS applied", func(t *testing.T) {
		t.Setenv("EXTRA_REDACT_PATTERNS", `CORP-[0-9]{8}`)
		var w bytes.Buffer
		err := run(strings.NewReader("id: CORP-12345678 and token: abc123"), &w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := w.String()
		if !strings.Contains(out, "[REDACTED-CUSTOM]") {
			t.Errorf("output %q should contain [REDACTED-CUSTOM]", out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Errorf("output %q should contain [REDACTED] from token pattern", out)
		}
	})

	t.Run("invalid EXTRA_REDACT_PATTERNS skipped no crash", func(t *testing.T) {
		t.Setenv("EXTRA_REDACT_PATTERNS", `[invalid,CORP-[0-9]{8}`)
		var w bytes.Buffer
		err := run(strings.NewReader("CORP-12345678"), &w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := w.String()
		if !strings.Contains(out, "[REDACTED-CUSTOM]") {
			t.Errorf("output %q should contain [REDACTED-CUSTOM]", out)
		}
	})
}
